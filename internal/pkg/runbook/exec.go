package runbook

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

type execConfig struct {
	Path    string   `yaml:"path"`
	Expr    string   `yaml:"expr"`
	Runtime string   `yaml:"runtime"` // optional, default to "sh -s"
	Args    []string `yaml:"args"`
}

type execAction struct {
	cfg execConfig
}

func newExecAction(cfg execConfig) (Action, error) {
	if cfg.Path == "" && cfg.Expr == "" {
		return nil, errors.New("either exec.path or exec.expr is required")
	}
	if cfg.Path != "" && cfg.Expr != "" {
		return nil, errors.New("exec.path and exec.expr are mutually exclusive")
	}
	return &execAction{cfg: cfg}, nil
}

func (e *execAction) Execute(ctx context.Context, cre map[string]any) error {
	// Template substitution for args
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

	var cmd *exec.Cmd

	switch {
	// External command with args
	case e.cfg.Path != "":
		cmd = exec.CommandContext(ctx, e.cfg.Path, args...)

	// expr + runtime piped via stdin
	case e.cfg.Expr != "":
  	// Expand template variables
  	expr, err := renderTemplate(e.cfg.Expr, cre)
 		if err != nil {
			return err
		}

		runtime := e.cfg.Runtime
		if runtime == "" {
			runtime = "sh -s"
		}

		parts := splitRuntime(runtime)
		cmd = exec.CommandContext(ctx, parts[0], append(parts[1:], args...)...)
    cmd.Stdin = strings.NewReader(expr)
	}

	// Common output wiring
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func renderTemplate(input string, data map[string]any) (string, error) {
	tmpl, err := template.New("inline").Funcs(funcMap()).Parse(input)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}

func splitRuntime(runtime string) []string {
	return strings.Fields(runtime) // basic split
}
