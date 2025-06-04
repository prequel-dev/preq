package runbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"
)

type slackConfig struct {
	WebhookURL      string `yaml:"webhook_url"`
	MessageTemplate string `yaml:"message_template"`
}

type slackAction struct {
	cfg   slackConfig
	tmpl  *template.Template
	httpc *http.Client
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
