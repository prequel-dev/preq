package rules

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/Masterminds/semver"
	"github.com/avast/retry-go/v4"
	"github.com/cqroot/prompt"
	"github.com/cqroot/prompt/choose"
	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/preq/internal/pkg/verz"
	"github.com/prequel-dev/preq/pkg/prefix"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	udpBaseAddr              = "api-beta.prequel.dev"
	prequelRulesSuffix       = ".gz"
	prequelRulesSha256Suffix = ".sha2"
	prequelRulesSigSuffix    = ".sig"
	tmpDirPrefix             = "preq-update"
	defaultLocalCheckDur     = 24 * time.Hour * 2 // 2 days
)

const (
	retries          = uint(1)
	rulesFilenameFmt = prefix.PrequelPublicRulesPrefix + "%s" + prequelRulesSuffix
	fastCheckTimeout = 150 * time.Millisecond
	slowCheckTimeout = 300 * time.Millisecond
	noRulesTimeout   = 5 * time.Second
	downloadTimeout  = 30 * time.Second
)

var (
	ErrInvalidResponse   = errors.New("invalid response")
	ErrNoRulesRelease    = errors.New("no rules release found")
	ErrNoVersion         = errors.New("no version found")
	ErrTimeout           = errors.New("timeout")
	ErrHashMismatch      = errors.New("hash mismatch")
	ErrInvalidKey        = errors.New("invalid public key")
	ErrInvalidUpdate     = errors.New("invalid update")
	ErrNoRules           = errors.New("no rules")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrUpdateExeFailed   = errors.New("exe update failed")
	ErrUpdateRulesFailed = errors.New("rules update failed")
)

// Set with `-ldflags "-X github.com/prequel-dev/preq/internal/pkg/rules.krewPluginEnabled=true"`
var (
	krewPluginEnabled string
)

type PackageUrls struct {
	DataUrl  string `json:"du,omitempty"`
	DataSize int64  `json:"ds,omitempty"`
	Hash     string `json:"h,omitempty"`
	HashUrl  string `json:"hu,omitempty"`
	HashSize int64  `json:"hs,omitempty"`
	SigUrl   string `json:"su,omitempty"`
	SigSize  int64  `json:"ss,omitempty"`
	Os       string `json:"os,omitempty"`
	Arch     string `json:"arch,omitempty"`
}

type RuleUpdateResponse struct {
	LatestRuleVersion string         `json:"rv,omitempty"`
	LatestRuleHash    string         `json:"rh,omitempty"`
	LatestExeVersion  string         `json:"ex,omitempty"`
	RuleUrls          *PackageUrls   `json:"ru,omitempty"`
	ExeUrls           []*PackageUrls `json:"eu,omitempty"`
}

const (
	fastReq = "UPDATE"
	maxResp = 1500 // typical MTU size
)

func GetRules(ctx context.Context, conf *config.Config, configDir, cmdLineRules, token, ruleUpdateFile, baseAddr string, tlsPort, udpPort int) ([]utils.RulePathT, error) {
	var (
		syncRulesPath string
		rulePaths     = make([]utils.RulePathT, 0)
		err           error
	)

	// Sync rules
	if syncRulesPath, err = syncUpdates(ctx, conf, configDir, token, ruleUpdateFile, baseAddr, tlsPort, udpPort); err != nil {
		// Continue on error. If we cannot download any rules at all on first run, a user will have to provide them on the command line or config
		log.Error().Err(err).Msg("Failed to sync updates. Continue...")
	}

	if syncRulesPath != "" && !conf.Rules.Disabled {
		rulePaths = append(rulePaths, utils.RulePathT{
			Path: syncRulesPath,
			Type: utils.RuleTypeCre,
		})
	}

	if cmdLineRules != "" {
		rulePaths = append(rulePaths, utils.RulePathT{
			Path: cmdLineRules,
			Type: utils.RuleTypeUser,
		})
	}

	for _, path := range conf.Rules.Paths {
		rulePaths = append(rulePaths, utils.RulePathT{
			Path: path,
			Type: utils.RuleTypeUser,
		})
	}

	if len(rulePaths) == 0 {
		return nil, ErrNoRules
	}

	return rulePaths, nil
}

func syncUpdates(ctx context.Context, conf *config.Config, configDir, token, updateFile, baseAddr string, tlsPort, udpPort int) (string, error) {

	var (
		tinyResp         *RuleUpdateResponse
		fullResp         *RuleUpdateResponse
		localCheckUpdate bool
		currRulesVer     *semver.Version
		currRulesPath    string
		apiUrl           = fmt.Sprintf("https://%s:%d", baseAddr, tlsPort)
		dur              = defaultLocalCheckDur
		timeout          = slowCheckTimeout
		err              error
	)

	if currRulesVer, currRulesPath, err = GetCurrentRulesVersion(configDir); err != nil {
		log.Error().Err(err).Msg("Failed to get current rules version")
		currRulesVer = semver.MustParse("0.0.0")
	}

	log.Info().Str("path", currRulesPath).Str("version", currRulesVer.String()).Msg("Current rules version")

	if conf.UpdateFrequency != nil {
		dur = *conf.UpdateFrequency
	}

	// Always check local state first in case time is up and we should just do a full check in
	if localCheckUpdate, err = localStateShouldUpdate(updateFile, dur); err != nil {
		return currRulesPath, err
	}

	// If we don't have any rules installed, then bump the timeout and force a full checkin
	if currRulesPath == "" {
		timeout = noRulesTimeout
		localCheckUpdate = true
		log.Warn().
			Str("timeout", timeout.String()).
			Msg("No rules installed yet. Increasing timeout and forcing a full checkin")
	}

	// If we don't need to do a full check in, do a fast one (~30ms)
	if !localCheckUpdate {
		if tinyResp, err = fastUpdateSync(ctx, fmt.Sprintf("%s:%d", udpBaseAddr, udpPort), fastCheckTimeout); err != nil {
			return currRulesPath, err
		}
	} else {
		// Otherwise, do a full check in (~130ms). Uses slowCheckTimeout
		if fullResp, err = checkin(ctx, apiUrl, token, currRulesVer, timeout); err != nil {
			return currRulesPath, err
		}
	}

	// We might have a tiny or full response. If we need to do one or more updates, then just do one full update checkin for a full response
	if shouldUpdateExe(tinyResp) || shouldUpdateRules(currRulesVer, tinyResp) {
		// Ok, we need to do one or more updates. But we don't know if we have a full response.
		if fullResp == nil {
			// Otherwise, do a full check in (~130ms). Uses slowCheckTimeout
			if fullResp, err = checkin(ctx, apiUrl, token, currRulesVer, timeout); err != nil {
				return currRulesPath, err
			}
		}
	}

	if fullResp == nil || fullResp.LatestExeVersion == "" || fullResp.LatestRuleVersion == "" {
		return currRulesPath, nil
	}

	// If we had a tiny response earlier, we have a full one now. If we had a full one earlier, we still have it.
	if shouldUpdateExe(fullResp) && !isKrewPluginEnabled() {
		if err = requestExeUpdate(ctx, fullResp, apiUrl, token, slowCheckTimeout, downloadTimeout, conf.AcceptUpdates); err != nil {
			return "", ErrUpdateExeFailed
		}
	}

	if shouldUpdateRules(currRulesVer, fullResp) {
		if currRulesPath, err = requestRuleUpdate(ctx, fullResp, apiUrl, token, configDir, slowCheckTimeout, downloadTimeout, conf.AcceptUpdates); err != nil {
			return "", err
		}
	}

	return currRulesPath, nil
}

func isKrewPluginEnabled() bool {
	log.Debug().Bool("enabled", len(krewPluginEnabled) > 0).Msg("Krew plugin")
	return len(krewPluginEnabled) > 0
}

func fastUpdateSync(_ context.Context, addr string, timeout time.Duration) (*RuleUpdateResponse, error) {
	var (
		resp       = &RuleUpdateResponse{}
		start      = time.Now()
		serverAddr *net.UDPAddr
		conn       *net.UDPConn
		err        error
	)

	if serverAddr, err = net.ResolveUDPAddr("udp", addr); err != nil {
		return nil, err
	}

	if conn, err = net.DialUDP("udp", nil, serverAddr); err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err = conn.Write([]byte(fastReq)); err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(timeout))

	buffer := make([]byte, maxResp)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(buffer[:n], resp); err != nil {
		return nil, err
	}

	log.Info().Interface("resp", resp).Str("rtt", time.Since(start).String()).Msg("Fast update sync")

	return resp, nil
}

func shouldUpdateExe(r *RuleUpdateResponse) bool {

	var (
		currVer *semver.Version
		newVer  *semver.Version
		err     error
	)

	if r == nil || r.LatestExeVersion == "" {
		return false
	}

	if newVer, err = semver.NewVersion(r.LatestExeVersion); err != nil {
		return false
	}

	if currVer, err = semver.NewVersion(verz.Semver()); err != nil {
		return false
	}

	return newVer.GreaterThan(currVer)
}

func requestExeUpdate(ctx context.Context, fullResp *RuleUpdateResponse, apiUrl, token string, slowCheckTimeout, downloadTimeout time.Duration, acceptUpdates bool) error {

	var (
		downloadLink = fmt.Sprintf(ux.DownloadPreqLinkFmt, fullResp.LatestExeVersion)
		choice       string
		err          error
	)

	// No support for auto-update exe on Windows yet. We'll need to rundll32 a separate updater so we can overwrite the exe
	if runtime.GOOS == "windows" {
		txt := fmt.Sprintf(ux.DownloadPreqAvailableFmt, fullResp.LatestExeVersion, downloadLink)
		fmt.Fprint(os.Stdout, txt)
		return nil
	}

	if !acceptUpdates {
		txt := fmt.Sprintf(ux.DownloadPreqAvailablePromptFmt, fullResp.LatestExeVersion, downloadLink)
		choice, err = prompt.New().Ask(txt).Choose(
			[]string{"Yes", "No"},
			choose.WithTheme(choose.ThemeArrow),
		)
		if err != nil {
			return err
		}
	} else {
		choice = "Yes"
	}

	if choice != "Yes" {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// If the returned path might be a symlink, use filepath.EvalSymlinks
	// to resolve it to the real path.
	currPath, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	log.Debug().
		Str("path", currPath).
		Msg("Current exe path")

	tempDir, err := os.MkdirTemp(filepath.Dir(currPath), tmpDirPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	pw := ux.NewProgressWriter(3)
	go pw.Render()

	var (
		dataUrl        string
		dataSize       int64
		hashUrl        string
		hashSize       int64
		sigUrl         string
		sigSize        int64
		exeHash        string
		newExePath     string
		newExeHashPath string
		newExeSigPath  string
		eb, hb, sb     []byte
	)

	for _, urls := range fullResp.ExeUrls {

		log.Info().
			Str("os", urls.Os).
			Str("arch", urls.Arch).
			Str("data_url", urls.DataUrl).
			Str("hash_url", urls.HashUrl).
			Str("sig_url", urls.SigUrl).
			Str("this_os", runtime.GOOS).
			Str("this_arch", runtime.GOARCH).
			Msg("Checking exe urls")

		if urls.Os == runtime.GOOS && urls.Arch == runtime.GOARCH {
			dataUrl = urls.DataUrl
			dataSize = urls.DataSize
			hashUrl = urls.HashUrl
			hashSize = urls.HashSize
			sigUrl = urls.SigUrl
			sigSize = urls.SigSize
			exeHash = urls.Hash
			break
		}
	}

	eb, err = downloadPackage(ctx, apiUrl, dataUrl, token, dataSize, pw, slowCheckTimeout, downloadTimeout)
	if err != nil {
		return err
	}

	newExePath = filepath.Join(tempDir, "preq")
	if err = os.WriteFile(newExePath, eb, 0755); err != nil {
		return err
	}

	log.Debug().
		Str("path", newExePath).
		Msg("Temp updated exe path")

	hb, err = downloadPackage(ctx, apiUrl, hashUrl, token, hashSize, pw, slowCheckTimeout, downloadTimeout)
	if err != nil {
		return err
	}

	newExeHashPath = filepath.Join(tempDir, "preq.sha2")
	if err = os.WriteFile(newExeHashPath, hb, 0644); err != nil {
		return err
	}

	log.Debug().
		Str("path", newExeHashPath).
		Msg("Temp updated exe hash path")

	sb, err = downloadPackage(ctx, apiUrl, sigUrl, token, sigSize, pw, slowCheckTimeout, downloadTimeout)
	if err != nil {
		return err
	}

	newExeSigPath = filepath.Join(tempDir, "preq.sig")
	if err = os.WriteFile(newExeSigPath, sb, 0644); err != nil {
		return err
	}

	log.Debug().
		Str("path", newExeSigPath).
		Msg("Temp updated exe sig path")

	block, _ := pem.Decode(publicRulesKeyPEM)
	if block == nil {
		return ErrInvalidKey
	}

	pubKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}

	pubKey, ok := pubKeyInterface.(*ecdsa.PublicKey)
	if !ok {
		return ErrInvalidKey
	}

	hash := sha256.New()
	hash.Write(eb)
	hashed := hash.Sum(nil)

	valid := ecdsa.VerifyASN1(pubKey, hashed, sb)
	if !valid {
		return ErrInvalidSignature
	}

	ebHash := utils.Sha256Sum(eb)
	if ebHash != exeHash {
		return ErrHashMismatch
	}

	fmt.Println("ECDSA signature and sha256 hash verified")

	if err = utils.CopyFile(newExePath, currPath); err != nil {
		return err
	}

	log.Debug().
		Str("src_path", newExePath).
		Str("dst_path", currPath).
		Msg("Updated exe path")

	return nil
}

func requestRuleUpdate(ctx context.Context, fullResp *RuleUpdateResponse, apiUrl, token string, configDir string, slowCheckTimeout, downloadTimeout time.Duration, acceptUpdates bool) (string, error) {

	var (
		downloadLink = fmt.Sprintf(ux.DownloadCreLinkFmt, fullResp.LatestRuleVersion)
		choice       string
		err          error
	)

	txt := fmt.Sprintf(ux.DownloadCreAvailablePromptFmt, fullResp.LatestRuleVersion, downloadLink)

	if !acceptUpdates {
		choice, err = prompt.New().Ask(txt).Choose(
			[]string{"Yes", "No"},
			choose.WithTheme(choose.ThemeArrow),
		)
		if err != nil {
			return "", err
		}
	} else {
		choice = "Yes"
	}

	if choice != "Yes" {
		return "", nil
	}

	log.Debug().
		Str("path", configDir).
		Msg("Config dir path")

	tempDir, err := os.MkdirTemp(configDir, "cre-rule-update")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	pw := ux.NewProgressWriter(3)
	go pw.Render()

	var (
		newRulePath     string
		newRuleHashPath string
		newRuleSigPath  string
		rb, hb, sb      []byte
	)

	rb, err = downloadPackage(ctx, apiUrl, fullResp.RuleUrls.DataUrl, token, fullResp.RuleUrls.DataSize, pw, slowCheckTimeout, downloadTimeout)
	if err != nil {
		return "", err
	}

	newRulePath = filepath.Join(tempDir, "cre-rules")
	if err = os.WriteFile(newRulePath, rb, 0644); err != nil {
		return "", err
	}

	log.Debug().
		Str("path", newRulePath).
		Msg("Temp updated rule path")

	hb, err = downloadPackage(ctx, apiUrl, fullResp.RuleUrls.HashUrl, token, fullResp.RuleUrls.HashSize, pw, slowCheckTimeout, downloadTimeout)
	if err != nil {
		return "", err
	}

	newRuleHashPath = filepath.Join(tempDir, "cre-rules.sha256")
	if err = os.WriteFile(newRuleHashPath, hb, 0644); err != nil {
		return "", err
	}

	log.Debug().
		Str("path", newRuleHashPath).
		Msg("Temp updated rule hash path")

	sb, err = downloadPackage(ctx, apiUrl, fullResp.RuleUrls.SigUrl, token, fullResp.RuleUrls.SigSize, pw, slowCheckTimeout, downloadTimeout)
	if err != nil {
		return "", err
	}

	newRuleSigPath = filepath.Join(tempDir, "cre-rules.sig")
	if err = os.WriteFile(newRuleSigPath, sb, 0644); err != nil {
		return "", err
	}

	log.Debug().
		Str("path", newRuleSigPath).
		Msg("Temp updated rule sig path")

	block, _ := pem.Decode(publicRulesKeyPEM)
	if block == nil {
		return "", ErrInvalidKey
	}

	pubKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", err
	}

	pubKey, ok := pubKeyInterface.(*ecdsa.PublicKey)
	if !ok {
		return "", ErrInvalidKey
	}

	hash := sha256.New()
	hash.Write(rb)
	hashed := hash.Sum(nil)

	valid := ecdsa.VerifyASN1(pubKey, hashed, sb)
	if !valid {
		return "", ErrInvalidSignature
	}

	ebHash := utils.Sha256Sum(rb)
	if ebHash != fullResp.LatestRuleHash {
		log.Error().Str("expected", fullResp.LatestRuleHash).Str("actual", ebHash).Msg("Hash mismatch")
		return "", ErrHashMismatch
	}

	fmt.Println("ECDSA signature and sha256 hash verified")

	baseRulesName, err := utils.UrlBase(fullResp.RuleUrls.DataUrl)
	if err != nil {
		return "", err
	}

	updatedRulesPath := filepath.Join(configDir, baseRulesName)
	if err = utils.CopyFile(newRulePath, updatedRulesPath); err != nil {
		return "", err
	}

	log.Debug().
		Str("src_path", newRulePath).
		Str("dst_path", updatedRulesPath).
		Msg("Updated rule path")

	baseHashName, err := utils.UrlBase(fullResp.RuleUrls.HashUrl)
	if err != nil {
		return "", err
	}

	if err = utils.CopyFile(newRuleHashPath, filepath.Join(configDir, baseHashName)); err != nil {
		return "", err
	}

	log.Debug().
		Str("src_path", newRuleHashPath).
		Str("dst_path", filepath.Join(configDir, baseHashName)).
		Msg("Updated rule hash path")

	return updatedRulesPath, nil
}

func shouldUpdateRules(currVer *semver.Version, r *RuleUpdateResponse) bool {

	var (
		newVer *semver.Version
		err    error
	)

	if r == nil || r.LatestRuleVersion == "" {
		return false
	}

	if newVer, err = semver.NewVersion(r.LatestRuleVersion); err != nil {
		return false
	}

	return newVer.GreaterThan(currVer)
}

func GetCurrentRulesVersion(configDir string) (*semver.Version, string, error) {

	var (
		currVer  *semver.Version
		currPath string
		err      error
	)

	if currVer, currPath, err = _getCurrentRulesVersion(configDir); err != nil {
		return nil, "", err
	}

	return currVer, currPath, nil
}

func postUrl(ctx context.Context, url string, token string, body []byte, timeout time.Duration) ([]byte, error) {

	var (
		httpRequest *http.Request
		client      = &http.Client{
			Timeout: timeout,
		}
		err error
	)

	if httpRequest, err = http.NewRequest("POST", url, bytes.NewBuffer(body)); err != nil {
		return nil, err
	}

	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	return retry.DoWithData(
		func() ([]byte, error) {

			resp, err := client.Do(httpRequest)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return nil, err
			}
			defer resp.Body.Close()

			rb, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error().Err(err).Msg("Fail read body")
				return nil, err
			}

			return rb, nil
		},
		retry.Attempts(retries),
		retry.Context(ctx),
	)
}

type RulesWhoAmI struct {
	Os          string `json:"os"`
	Version     string `json:"version"`
	GitHash     string `json:"git_hash"`
	RuleVersion string `json:"rule_version"`
	Timezone    string `json:"timezone"`
}

func checkin(ctx context.Context, apiUrl, token string, currRulesVer *semver.Version, timeout time.Duration) (*RuleUpdateResponse, error) {

	var (
		u = fmt.Sprintf("%s/v1/rules/update", apiUrl)
		w = &RulesWhoAmI{
			Os:          utils.GetOSInfo(),
			Version:     verz.Semver(),
			GitHash:     verz.Githash,
			RuleVersion: currRulesVer.String(),
		}
		start            = time.Now()
		tzName, tzOffset = start.Zone()
		data             []byte
		resp             []byte
		r                RuleUpdateResponse
		err              error
	)

	w.Timezone = fmt.Sprintf("%s/%d", tzName, tzOffset)

	if data, err = json.Marshal(w); err != nil {
		log.Error().Err(err).Msg("Fail json.Marshal")
		return nil, err
	}

	if resp, err = postUrl(ctx, u, token, data, timeout); err != nil {
		log.Error().Err(err).Msg("Fail postUrl")
		return nil, err
	}

	if err = json.Unmarshal(resp, &r); err != nil {
		log.Error().Str("resp", string(resp)).Err(err).Msg("Fail json.Unmarshal")
		return nil, err
	}

	log.Debug().Interface("resp", r).Str("rtt", time.Since(start).String()).Msg("Checkin")

	return &r, nil
}

func localStateShouldUpdate(updateFile string, dur time.Duration) (bool, error) {

	var (
		info       os.FileInfo
		f          *os.File
		createErr  error
		chtimesErr error
		now        = time.Now()
		modTime    time.Time
		err        error
	)

	info, err = os.Stat(updateFile)

	if os.IsNotExist(err) {
		f, createErr = os.Create(updateFile)
		if createErr != nil {
			return false, createErr
		}
		f.Close()

		if chtimesErr := os.Chtimes(updateFile, now, now); chtimesErr != nil {
			return false, chtimesErr
		}

		log.Debug().
			Str("path", updateFile).
			Dur("dur", dur).
			Msg("Local state time to update")

		return true, nil

	} else if err != nil {
		return false, err
	} else {
		modTime = info.ModTime()
		if time.Since(modTime) >= dur {
			if chtimesErr = os.Chtimes(updateFile, now, now); chtimesErr != nil {
				return false, chtimesErr
			}
			log.Debug().
				Str("path", updateFile).
				Dur("dur", dur).
				Dur("since", time.Since(modTime)).
				Msg("Local state time to update")

			return true, nil
		} else {
			log.Debug().Msg("Local state not time to update")
			return false, nil
		}
	}
}

/*
section: version
content:
  - version: 0.3.0
*/

func getRulesVersion(path string) (*semver.Version, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Decompress the gzip file
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	decoder := yaml.NewDecoder(gz)

	var docs []map[string]interface{}
	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err == io.EOF {
			// No more documents
			break
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}

	// Search for document with section == "version"
	var versionStr string
	for _, doc := range docs {
		if doc["section"] == "version" {
			// Extract the `content` field, which should be a slice of interface{}
			if content, ok := doc["content"].([]interface{}); ok {
				// Each element in `content` is itself a map[string]interface{}
				for _, c := range content {
					if entry, ok := c.(map[string]interface{}); ok {
						if v, ok := entry["version"].(string); ok {
							versionStr = v
							break
						}
					}
				}
			}
		}
		if versionStr != "" {
			break
		}
	}

	if versionStr == "" {
		return nil, ErrNoVersion
	}

	// Parse the version string using Masterminds/semver
	ver, err := semver.NewVersion(versionStr)
	if err != nil {
		return nil, err
	}

	return ver, nil
}

func _getCurrentRulesVersion(configDir string) (*semver.Version, string, error) {

	var (
		pattern  = filepath.Join(configDir, fmt.Sprintf(rulesFilenameFmt, "*"))
		packages []string
		err      error
		curr     *semver.Version
		currPath string
	)

	if packages, err = filepath.Glob(pattern); err != nil {
		return nil, "", err
	}

	if len(packages) == 0 {
		return nil, "", ErrNoRulesRelease
	}

	sort.Strings(packages)

	for _, p := range packages {

		var ver *semver.Version

		if ver, err = getRulesVersion(p); err != nil {
			log.Error().Err(err).Msg("Failed to get rules version")
			return nil, "", err
		}

		if curr == nil || ver.Compare(curr) > 0 {
			curr = ver
			currPath = p
		}
	}

	return curr, currPath, nil
}
