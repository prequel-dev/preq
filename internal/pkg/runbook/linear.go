package runbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"text/template"
)

type linearConfig struct {
	SecretEnv           string `yaml:"secret_env"`
	Secret              string `yaml:"secret"`
	TitleTemplate       string `yaml:"title_template"`
	DescriptionTemplate string `yaml:"description_template"`
	TeamID              string `yaml:"team_id"`
}

type linearAction struct {
	token      string
	teamID     string
	titleTmpl  *template.Template
	descTmpl   *template.Template
}

func newLinearAction(cfg linearConfig) (Action, error) {
	if cfg.TeamID == "" {
		return nil, errors.New("linear.team_id is required")
	}
	if cfg.TitleTemplate == "" {
		return nil, errors.New("linear.title_template is required")
	}
	if cfg.DescriptionTemplate == "" {
		return nil, errors.New("linear.description_template is required")
	}

	st, err := template.New("linear-title").Funcs(funcMap()).Parse(cfg.TitleTemplate)
	if err != nil {
		return nil, fmt.Errorf("linear title template error: %w", err)
	}
	dt, err := template.New("linear-desc").Funcs(funcMap()).Parse(cfg.DescriptionTemplate)
	if err != nil {
		return nil, fmt.Errorf("linear description template error: %w", err)
	}

	if cfg.Secret == "" && cfg.SecretEnv != "" {
		cfg.Secret = os.Getenv(cfg.SecretEnv)
	}
	if cfg.Secret == "" {
		return nil, errors.New("linear secret missing; set either 'secret' or 'secret_env'")
	}

	return &linearAction{
		token:     cfg.Secret,
		teamID:    cfg.TeamID,
		titleTmpl: st,
		descTmpl:  dt,
	}, nil
}

func (a *linearAction) Execute(ctx context.Context, cre map[string]any) error {
	var title, desc string
  if err := executeTemplate(&title, a.titleTmpl, cre); err != nil {
		return fmt.Errorf("linear: title: %w", err)
	}
  if err := executeTemplate(&desc, a.descTmpl, cre); err != nil {
		return fmt.Errorf("linear: description: %w", err)
	}

	query := `mutation IssueCreate($input: IssueCreateInput!) {
		issueCreate(input: $input) {
			success
			issue { id url title }
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"title":       title,
			"description": desc,
			"teamId":      a.teamID,
		},
	}

	payload := map[string]any{
		"query":     query,
		"variables": vars,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("linear: encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("linear: request: %w", err)
	}
	req.Header.Set("Authorization", a.token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("linear: post: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		return fmt.Errorf("linear: HTTP %d", res.StatusCode)
	}

	var out struct {
		Data struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return fmt.Errorf("linear: decode: %w", err)
	}
	if len(out.Errors) > 0 {
		return errors.New(out.Errors[0].Message)
	}
	if !out.Data.IssueCreate.Success {
		return errors.New("linear: issue creation failed")
	}

	log := out.Data.IssueCreate.Issue.URL
	fmt.Fprintf(os.Stderr, "linear: created issue → %s\n", log)
	return nil
}
