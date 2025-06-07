package runbook

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"text/template"
)

type stubAction struct{ called bool }

func (s *stubAction) Execute(ctx context.Context, m map[string]any) error {
	s.called = true
	return nil
}

func TestFilteredAction(t *testing.T) {
	a := &stubAction{}
	f := &filteredAction{pattern: regexp.MustCompile("CRE-1"), inner: a}

	// no match
	if err := f.Execute(context.Background(), map[string]any{"cre": map[string]any{"id": "CRE-2"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.called {
		t.Fatalf("action should not run")
	}

	// match
	if err := f.Execute(context.Background(), map[string]any{"cre": map[string]any{"id": "CRE-1"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !a.called {
		t.Fatalf("action should run")
	}
}

func TestFuncMapAndExecuteTemplate(t *testing.T) {
	data := map[string]any{"cre": map[string]any{"ID": "CRE-9"}, "desc": "- message"}
	funcs := funcMap()
	tmpl, err := template.New("t").Funcs(funcs).Parse("{{field .cre \"ID\"}} {{stripdash .desc}}")
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}
	var out string
	if err := executeTemplate(&out, tmpl, data); err != nil {
		t.Fatalf("executeTemplate: %v", err)
	}
	if out != "CRE-9 message" {
		t.Fatalf("got %q", out)
	}
}

func TestExtractCreId(t *testing.T) {
	ev := map[string]any{"cre": map[string]any{"id": "A"}}
	if id := extractCreId(ev); id != "A" {
		t.Fatalf("map: expected A got %s", id)
	}
	type cre struct{ ID string }
	ev["cre"] = &cre{ID: "B"}
	if id := extractCreId(ev); id != "B" {
		t.Fatalf("struct: expected B got %s", id)
	}
	delete(ev, "cre")
	ev["id"] = "C"
	if id := extractCreId(ev); id != "C" {
		t.Fatalf("top-level: expected C got %s", id)
	}
}

func TestNewExecAction(t *testing.T) {
	_, err := newExecAction(execConfig{})
	if err == nil {
		t.Fatalf("expected error for missing path")
	}
	script := filepath.Join(t.TempDir(), "script.sh")
	os.WriteFile(script, []byte("#!/bin/sh\ncat >/dev/null"), 0755)
	a, err := newExecAction(execConfig{Path: script})
	if err != nil {
		t.Fatalf("newExecAction: %v", err)
	}
	if err := a.Execute(context.Background(), map[string]any{"id": "x"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestNewSlackAction(t *testing.T) {
	_, err := newSlackAction(slackConfig{})
	if err == nil {
		t.Fatalf("expected error for missing fields")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte("CRE-5")) {
			t.Errorf("missing id")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	a, err := newSlackAction(slackConfig{WebhookURL: srv.URL, MessageTemplate: "{{field .cre \"ID\"}}"})
	if err != nil {
		t.Fatalf("newSlackAction: %v", err)
	}
	err = a.Execute(context.Background(), map[string]any{"cre": map[string]any{"ID": "CRE-5"}})
	if err != nil {
		t.Fatalf("execute slack: %v", err)
	}
}

func TestBuildActions(t *testing.T) {
	script := filepath.Join(t.TempDir(), "run.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 0"), 0755)
	cfg := "actions:\n- type: exec\n  exec:\n    path: " + script + "\n"
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	os.WriteFile(path, []byte(cfg), 0644)
	acts, err := buildActions(path)
	if err != nil {
		t.Fatalf("buildActions: %v", err)
	}
	if len(acts) != 1 {
		t.Fatalf("expected 1 action got %d", len(acts))
	}
}
