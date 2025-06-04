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
	"text/template"
	"time"
)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.cfg.WebhookURL,
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("jira post: %w", err)
	}
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
