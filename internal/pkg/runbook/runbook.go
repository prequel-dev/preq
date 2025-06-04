package runbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"text/template"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

/* -------------------------------------------------------------------------
   Trigger/Action proof‑of‑concept (YAML‑driven)

   Build & run:
     go run ./... config.yaml event.json

   Example config.yaml
   -------------------
   actions:
     # 1) Slack notification
     - type: slack
       slack:
         webhook_url: https://hooks.slack.com/services/T000/B000/XXX
         message_template: |
           *Build failed!* 🚨\nReason: {{ .Reason }}\nCommit: {{ .Commit }}

     # 2) Jira Automation webhook
     - type: jira
       jira:
         webhook_url: https://your-domain.atlassian.net/gateway/api/automation/.../WEBHOOK_ID
         secret: abc123
         summary_template: Build failed on {{ .Branch }}
         description_template: |
           Commit: {{ .Commit }}\nReason: {{ .Reason }}

     # 3) Run local helper script
     - type: exec
       exec:
         path: /usr/local/bin/notify.sh
         args:
           - "--branch={{ .Branch }}"
           - "--reason={{ .Reason }}"
--------------------------------------------------------------------------*/

// ------------------------------------------------------------------------
// Action interface & factory
// ------------------------------------------------------------------------

type Action interface {
	Execute(ctx context.Context, cre map[string]any) error
}

type configFile struct {
	Actions []actionConfig `yaml:"actions"`
}

type actionConfig struct {
	Type  string `yaml:"type"`
	Regex string `yaml:"regex,omitempty"`

	Slack *slackConfig `yaml:"slack,omitempty"`
	Jira  *jiraConfig  `yaml:"jira,omitempty"`
	Exec  *execConfig  `yaml:"exec,omitempty"`
}

func extractCREID(ev map[string]any) string {
	if cre, ok := ev["cre"]; ok {
		// map variant
		if m, ok := cre.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				return id
			}
			if id, ok := m["ID"].(string); ok {
				return id
			}
		}
		// struct variant
		v := reflect.ValueOf(cre)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.IsValid() && v.Kind() == reflect.Struct {
			f := v.FieldByName("ID")
			if f.IsValid() && f.Kind() == reflect.String {
				return f.String()
			}
		}
	}
	// fallback to top-level id
	if id, ok := ev["id"].(string); ok {
		return id
	}
	return ""
}

// ----- decorator that runs the action only when CRE ID matches ---------------
type filteredAction struct {
	pattern *regexp.Regexp
	inner   Action
}

func (f *filteredAction) Execute(ctx context.Context, ev map[string]any) error {
	if f.pattern == nil { // no filter → always run
		return f.inner.Execute(ctx, ev)
	}
	if id := extractCREID(ev); id != "" && f.pattern.MatchString(id) {
		return f.inner.Execute(ctx, ev) // match → run
	}
	return nil // no match → silently skip
}

func buildActions(cfgPath string) ([]Action, error) {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var file configFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, err
	}

	actions := make([]Action, 0, len(file.Actions))
	for i, c := range file.Actions {
		var a Action
		switch c.Type {
		case "slack":
			if c.Slack == nil {
				return nil, fmt.Errorf("missing slack section for action #%d", i)
			}
			a, err = newSlackAction(*c.Slack)
		case "jira":
			if c.Jira == nil {
				return nil, fmt.Errorf("missing jira section for action #%d", i)
			}
			a, err = newJiraAction(*c.Jira)
		case "exec":
			if c.Exec == nil {
				return nil, fmt.Errorf("missing exec section for action #%d", i)
			}
			a, err = newExecAction(*c.Exec)
		default:
			err = fmt.Errorf("unknown action type %q (index %d)", c.Type, i)
		}
		if err != nil {
			return nil, err
		}

		if c.Regex != "" {
			re, err := regexp.Compile(c.Regex)
			if err != nil {
				return nil, fmt.Errorf("invalid cre_id_regex for action #%d: %w", i, err)
			}
			a = &filteredAction{pattern: re, inner: a}
		}
		actions = append(actions, a)
	}
	return actions, nil
}

// ------------------------------------------------------------------------
// Slack Action
// ------------------------------------------------------------------------

type slackConfig struct {
	WebhookURL      string `yaml:"webhook_url"`
	MessageTemplate string `yaml:"message_template"`
}

type slackAction struct {
	cfg   slackConfig
	tmpl  *template.Template
	httpc *http.Client
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		// field works with map[string]any OR struct / *struct
		"field": func(obj any, name string) any {
			if obj == nil {
				log.Error().Msg("field: obj is nil")
				return nil
			}
			// map
			if m, ok := obj.(map[string]any); ok {
				log.Info().Msgf("field: obj is map[string]any, name: %s", name)
				return m[name]
			}
			// struct via reflection
			v := reflect.ValueOf(obj)
			if v.Kind() == reflect.Pointer {
				log.Info().Msg("field: obj is pointer")
				v = v.Elem()
			}
			if v.IsValid() && v.Kind() == reflect.Struct {
				log.Info().Msgf("field: obj is struct, name: %s", name)
				f := v.FieldByName(name)
				if f.IsValid() {
					log.Info().Msgf("field: obj is struct, name: %s, value: %v", name, f.Interface())
					return f.Interface()
				}
			}
			log.Error().Msgf("field: unknown type: %T", obj)
			return nil // unknown
		},
	}
}

func newSlackAction(cfg slackConfig) (Action, error) {
	if cfg.WebhookURL == "" {
		return nil, errors.New("slack.webhook_url is required")
	}
	if cfg.MessageTemplate == "" {
		return nil, errors.New("slack.message_template is required")
	}
	t, err := template.New("slack").Funcs(funcMap()).Parse(cfg.MessageTemplate)
	if err != nil {
		return nil, err
	}

	return &slackAction{
		cfg:  cfg,
		tmpl: t,
		httpc: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (s *slackAction) Execute(ctx context.Context, cre map[string]any) error {
	var msg string
	if err := executeTemplate(&msg, s.tmpl, cre); err != nil {
		return err
	}
	payload := struct {
		Text string `json:"text"`
	}{Text: msg}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.WebhookURL,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("slack post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack post failed: %s – %s", resp.Status, respBody)
	}
	return nil
}

// ------------------------------------------------------------------------
// Jira Action (Automation webhook flavour)
// ------------------------------------------------------------------------

type jiraConfig struct {
	WebhookURL          string `yaml:"webhook_url"`
	Secret              string `yaml:"secret"`     // optional
	SecretEnv           string `yaml:"secret_env"` // optional
	SummaryTemplate     string `yaml:"summary_template"`
	DescriptionTemplate string `yaml:"description_template"`
	ProjectKey          string `yaml:"project_key"` // e.g. "PREQ"
}

type jiraAction struct {
	cfg         jiraConfig
	summaryTmpl *template.Template
	descTmpl    *template.Template
	httpc       *http.Client
}

func newJiraAction(cfg jiraConfig) (Action, error) {
	if cfg.WebhookURL == "" {
		return nil, errors.New("jira.webhook_url is required")
	}
	if cfg.SummaryTemplate == "" {
		return nil, errors.New("jira.summary_template is required")
	}
	if cfg.ProjectKey == "" {
		return nil, errors.New("jira.project_key is required when using REST API mode")
	}
	st, err := template.New("jira-summary").Funcs(funcMap()).Parse(cfg.SummaryTemplate)
	if err != nil {
		return nil, err
	}
	dt, err := template.New("jira-desc").Funcs(funcMap()).Parse(cfg.DescriptionTemplate)
	if err != nil {
		return nil, err
	}

	if cfg.Secret == "" && cfg.SecretEnv != "" {
		cfg.Secret = os.Getenv(cfg.SecretEnv)
	}
	// optional: hard‑fail if both were empty
	if cfg.Secret == "" {
		return nil, errors.New("jira secret missing; set either 'secret' or 'secret_env'")
	}

	return &jiraAction{
		cfg:         cfg,
		summaryTmpl: st,
		descTmpl:    dt,
		httpc: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (j *jiraAction) Execute(ctx context.Context, cre map[string]any) error {
	var summary, desc string
	if err := executeTemplate(&summary, j.summaryTmpl, cre); err != nil {
		return err
	}
	if err := executeTemplate(&desc, j.descTmpl, cre); err != nil {
		return err
	}
	payload := map[string]any{
		"project":     map[string]any{"key": j.cfg.ProjectKey},
		"summary":     summary,
		"description": adfParagraph(desc),
		"issuetype":   map[string]any{"name": "Bug"},
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, j.cfg.WebhookURL,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if j.cfg.Secret != "" {
		req.Header.Set("X-Automation-Webhook-Token", j.cfg.Secret)
	}
	resp, err := j.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("jira post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira post failed: %s – %s", resp.Status, respBody)
	}
	return nil
}

// ------------------------------------------------------------------------
// Exec Action
// ------------------------------------------------------------------------

type execConfig struct {
	Path string   `yaml:"path"`
	Args []string `yaml:"args"`
}

type execAction struct {
	cfg execConfig
}

func newExecAction(cfg execConfig) (Action, error) {
	if cfg.Path == "" {
		return nil, errors.New("exec.path is required")
	}
	return &execAction{cfg: cfg}, nil
}

func (e *execAction) Execute(ctx context.Context, cre map[string]any) error {
	// Template‑render each arg
	args := make([]string, len(e.cfg.Args))
	for i, a := range e.cfg.Args {
		tmpl, err := template.New("arg").Funcs(funcMap()).Parse(a)
		if err != nil {
			return err
		}
		if err := executeTemplate(&args[i], tmpl, cre); err != nil {
			return err
		}
	}

	raw, err := json.Marshal(cre)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, e.cfg.Path, args...)
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ------------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------------

func executeTemplate(out *string, tmpl *template.Template, data any) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	*out = buf.String()
	return nil
}

// ------------------------------------------------------------------------
// Main
// ------------------------------------------------------------------------

func Runbook(ctx context.Context, cfgPath string, report ux.ReportDocT) error {

	actions, err := buildActions(cfgPath)
	if err != nil {
		return err
	}

	for _, a := range actions {
		for _, cre := range report {
			if err := a.Execute(ctx, cre); err != nil {
				return err
			}
		}
	}

	return nil
}

func adfParagraph(txt string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": txt,
					},
				},
			},
		},
	}
}
