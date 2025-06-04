package runbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"text/template"
)

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
