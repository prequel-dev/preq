package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/rs/zerolog/log"
)

const (
	AUTH_HDR_AMZ_SHA256 = "X-Amz-Content-Sha256"
	AUTH_HDR_AMZ_DATE   = "X-Amz-Date"
	AUTH_HDR_AMZ_TOKEN  = "X-Amz-Security-Token"
	AUTH_HDR_AMZ_AUTH   = "Authorization"
)

var (
	ErrPrefixMatch         = errors.New("url does not match prefix")
	ErrInvalidDownloadPath = errors.New("invalid download path")
)

const numAttempts = 3

type RulesDownload struct {
	RulesPackage string `json:"rules_package"`
}

type RulesDownloadAuth struct {
	Token  string `json:"token"`
	Auth   string `json:"authorization"`
	Sha256 string `json:"sha256"`
	Date   string `json:"date"`
}

func rulesDownloadAuthRequest(ctx context.Context, retries int, baseUrl, rulesDownloadUrl, token string, timeout time.Duration) (*RulesDownloadAuth, error) {
	parsedURL, err := url.Parse(rulesDownloadUrl)
	if err != nil {
		return nil, err
	}

	if len(parsedURL.Path) == 0 {
		log.Error().Str("url", rulesDownloadUrl).Msg("Invalid download path")
		return nil, ErrInvalidDownloadPath
	}

	rulesDownloadPath := parsedURL.Path[1:]

	reqData := &RulesDownload{
		RulesPackage: rulesDownloadPath,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"POST",
		baseUrl+"/v1/rules/rules_auth",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{
		Timeout: timeout,
	}

	return retry.DoWithData(
		func() (*RulesDownloadAuth, error) {
			resp, err := client.Do(req)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return nil, err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error().Err(err).Msg("Fail read body")
				return nil, err
			}

			var responseData RulesDownloadAuth
			err = json.Unmarshal(body, &responseData)
			if err != nil {
				return nil, err
			}

			return &responseData, nil
		},
		retry.Attempts(uint(retries)),
		retry.Context(ctx),
	)
}

func downloadPackage(ctx context.Context, apiUrl, packageUrl, token string, totalSize int64, pw progress.Writer, slowCheckTimeout, downloadTimeout time.Duration) ([]byte, error) {
	var (
		authHdrs *RulesDownloadAuth
		err      error
	)

	if authHdrs, err = rulesDownloadAuthRequest(ctx, numAttempts, apiUrl, packageUrl, token, slowCheckTimeout); err != nil {
		log.Error().Err(err).Msg("Fail RulesDownloadAuthRequest")
		return nil, err
	}

	return _downloadPackage(ctx, packageUrl, totalSize, authHdrs, pw, downloadTimeout)
}

func _downloadPackage(ctx context.Context, url string, totalSize int64, authHdrs *RulesDownloadAuth, pw progress.Writer, downloadTimeout time.Duration) ([]byte, error) {

	var (
		httpRequest *http.Request
		client      = &http.Client{
			Timeout: downloadTimeout,
		}
		err error
	)

	if httpRequest, err = http.NewRequest("GET", url, nil); err != nil {
		return nil, err
	}

	httpRequest.Header.Set(AUTH_HDR_AMZ_SHA256, authHdrs.Sha256)
	httpRequest.Header.Set(AUTH_HDR_AMZ_DATE, authHdrs.Date)
	httpRequest.Header.Set(AUTH_HDR_AMZ_TOKEN, authHdrs.Token)
	httpRequest.Header.Set(AUTH_HDR_AMZ_AUTH, authHdrs.Auth)

	return retry.DoWithData(
		func() ([]byte, error) {

			resp, err := client.Do(httpRequest)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return nil, err
			}
			defer resp.Body.Close()

			tracker := ux.NewDownloadTracker(totalSize)

			pw.AppendTracker(&tracker)

			var buf bytes.Buffer

			chunkSize := 1 * 1024
			tmp := make([]byte, chunkSize)

			for {
				n, readErr := resp.Body.Read(tmp)
				if n > 0 {
					buf.Write(tmp[:n])
					tracker.Increment(int64(n))
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					fmt.Printf("Read error: %v\n", readErr)
					pw.Stop()
					return nil, fmt.Errorf("read error: %w", readErr)
				}
			}

			tracker.MarkAsDone()
			pw.Stop()

			return buf.Bytes(), nil
		},
		retry.Attempts(retries),
		retry.Context(ctx),
	)
}
