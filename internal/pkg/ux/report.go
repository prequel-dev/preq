package ux

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/prequel-dev/prequel-compiler/pkg/matchz"
	"github.com/prequel-dev/prequel-compiler/pkg/parser"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/rs/zerolog/log"
)

const (
	sevCritical   = "critical"
	sevHigh       = "high"
	sevMedium     = "medium"
	sevLow        = "low"
	colorCritical = text.FgHiRed
	colorHigh     = text.FgHiYellow
	colorMedium   = text.FgHiMagenta
	colorLow      = text.FgHiGreen
	reportFmt     = "preq-report-%d.json"
)

var (
	retries = uint(3)
	delay   = time.Second * 5
)

type ReportT struct {
	mux     sync.Mutex
	CreHits map[string][]time.Time
	Hits    map[string]map[time.Time]matchz.HitsT
	Rules   map[string]parser.ParseRuleT
	Pw      progress.Writer
}

func NewReport(pw progress.Writer) *ReportT {
	return &ReportT{
		CreHits: make(map[string][]time.Time),                // cre -> timestamps for each detection
		Hits:    make(map[string]map[time.Time]matchz.HitsT), // cre -> timestamp -> matchz.HitsT
		Rules:   make(map[string]parser.ParseRuleT),          // cre -> parser.ParseRuleT
		Pw:      pw,
	}
}

func (r *ReportT) AddCreHit(cre *parser.ParseCreT, hit time.Time, m matchz.HitsT) bool {
	r.mux.Lock()
	defer r.mux.Unlock()

	var newDetection bool

	if _, ok := r.CreHits[cre.Id]; !ok {
		newDetection = true
	}

	r.CreHits[cre.Id] = append(r.CreHits[cre.Id], hit)

	if _, ok := r.Hits[cre.Id]; !ok {
		r.Hits[cre.Id] = make(map[time.Time]matchz.HitsT)
	}

	r.Hits[cre.Id][hit] = m

	return newDetection
}

func (r *ReportT) AddRules(rules *parser.RulesT) {
	r.mux.Lock()
	defer r.mux.Unlock()

	var ok bool
	for _, rule := range rules.Rules {
		if _, ok = r.Rules[rule.Cre.Id]; !ok {
			r.Rules[rule.Cre.Id] = rule
		} else {
			log.Warn().Str("creId", rule.Cre.Id).Msg("CRE already exists")
		}
	}
}

func (r *ReportT) GetCre(creId string) parser.ParseRuleT {
	r.mux.Lock()
	defer r.mux.Unlock()
	return r.Rules[creId]
}

func getColorizedCount(c int, timestamp time.Time) string {
	count := text.Colors{text.FgBlue, text.Bold}.Sprintf("[%d hits ", c)
	count += text.Colors{text.FgMagenta, text.Bold}.Sprintf("@ ")
	count += text.Colors{text.FgBlue, text.Bold}.Sprintf("%s]", timestamp.Format(time.RFC3339Nano))
	return count
}

func getColorizedCre(creId string, colors text.Colors) string {
	return colors.Sprintf("%-20s", creId)
}

func (r *ReportT) DisplayCREs() error {
	r.mux.Lock()
	defer r.mux.Unlock()

	var (
		rules = make([]parser.ParseRuleT, 0)
	)

	for _, rule := range r.Rules {
		rules = append(rules, rule)
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Cre.Severity > rules[j].Cre.Severity
	})

	for _, rule := range rules {

		var (
			creHits = r.CreHits[rule.Cre.Id]
		)

		if len(creHits) == 0 {
			continue
		}

		var (
			color    text.Color
			severity string
		)

		switch rule.Cre.Severity {
		case parser.SeverityCritical:
			severity = sevCritical
			color = colorCritical
		case parser.SeverityHigh:
			severity = sevHigh
			color = colorHigh
		case parser.SeverityMedium:
			severity = sevMedium
			color = colorMedium
		case parser.SeverityLow:
			severity = sevLow
			color = colorLow
		}

		const sevWidth = max(len(sevCritical), len(sevHigh), len(sevMedium), len(sevLow))

		var (
			count = getColorizedCount(len(creHits), creHits[0])
			cre   = getColorizedCre(rule.Cre.Id, text.Colors{color, text.Bold})
			tmpl  = fmt.Sprintf("%%%ds", sevWidth)
			sevS  = text.Colors{color}.Sprintf(tmpl, severity)
		)

		r.Pw.Log(fmt.Sprintf("%s %s %s", cre, sevS, count))
	}
	return nil
}

func (r *ReportT) Write(path string) (string, error) {
	r.mux.Lock()
	defer r.mux.Unlock()

	var (
		reportName string
		o          any
		data       []byte
		err        error
	)

	if path == "" {
		reportName = fmt.Sprintf(reportFmt, time.Now().Unix())
	} else {
		reportName = path
	}

	if o, err = r.createReport(); err != nil {
		return "", err
	}

	data, err = json.MarshalIndent(o, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal report")
		return "", err
	}

	if err = os.WriteFile(reportName, data, 0644); err != nil {
		return "", err
	}

	return reportName, nil
}

func (r *ReportT) PrintReport() error {
	r.mux.Lock()
	defer r.mux.Unlock()

	var (
		o    any
		data []byte
		err  error
	)

	if o, err = r.createReport(); err != nil {
		return err
	}

	data, err = json.MarshalIndent(o, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal report")
		return err
	}

	fmt.Fprintln(os.Stdout, string(data))

	return nil
}

func (r *ReportT) Size() int {
	r.mux.Lock()
	defer r.mux.Unlock()
	return len(r.CreHits)
}

func (r *ReportT) CreateReport() (any, error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	return r.createReport()
}

func (r *ReportT) createReport() (any, error) {
	var (
		out = make([]map[string]any, 0)
	)

	// timestamp, CRE, rule id and hash, hit data
	for id, creHits := range r.CreHits {

		var o = make(map[string]any)
		o["timestamp"] = creHits[0].Format(time.RFC3339Nano)
		o["id"] = id
		o["cre"] = r.Rules[id].Cre
		o["rule_id"] = r.Rules[id].Metadata.Id
		o["rule_hash"] = r.Rules[id].Metadata.Hash

		type entryT struct {
			Timestamp time.Time `json:"timestamp"`
			Entry     string    `json:"entry"`
		}
		matchHits := make([]entryT, 0)
		for _, hit := range creHits {

			for _, e := range r.Hits[id][hit].Entries {
				matchHits = append(matchHits, entryT{
					Timestamp: time.Unix(0, e.Timestamp),
					Entry:     string(e.Entry),
				})
			}
		}

		o["hits"] = matchHits
		out = append(out, o)
	}

	return out, nil
}

func (r *ReportT) PostSlackDetection(ctx context.Context, url string, notificationContext string) error {
	return r.postSlackDetection(ctx, url, notificationContext)
}

func (r *ReportT) postSlackDetection(ctx context.Context, url, notificationContext string) error {

	var (
		notification string
		msg          = make(map[string]any)
		jsonData     []byte
		err          error
	)

	notification = fmt.Sprintf("preq detection [%s]: ", notificationContext)

	for creId := range r.CreHits {
		notification += fmt.Sprintf("%s (%d), ", creId, r.Rules[creId].Cre.Severity)
	}

	// remove the last comma
	notification = notification[:len(notification)-2]
	msg["text"] = notification

	jsonData, err = json.Marshal(msg)
	if err != nil {
		return err
	}

	httpRequest, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	httpRequest.Header.Set("Accept", "application/json")

	client := &http.Client{}

	return retry.Do(
		func() error {

			resp, err := client.Do(httpRequest)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return err
			}
			defer resp.Body.Close()

			return nil
		},
		retry.Attempts(retries),
		retry.Delay(delay),
		retry.Context(ctx),
		retry.OnRetry(func(u uint, err error) {
			log.Error().Err(err).Uint("retry", u).Msg("Retry token poll error")
		}),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}
