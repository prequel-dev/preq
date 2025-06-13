package engine

import (
	"context"
	"testing"

	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/prequel-compiler/pkg/compiler"
	"github.com/prequel-dev/prequel-compiler/pkg/parser"
)

func TestNew(t *testing.T) {
	t.Run("creates new runtime with correct initial values", func(t *testing.T) {
		stop := int64(100)
		uxFactory := ux.NewUxEval()

		runtime := New(stop, uxFactory)

		if runtime == nil {
			t.Fatal("Expected runtime to not be nil")
		}
		if runtime.Stop != stop {
			t.Errorf("Expected Stop to be %d, got %d", stop, runtime.Stop)
		}
		if runtime.Ux != uxFactory {
			t.Error("Expected Ux to match provided factory")
		}
		if runtime.Rules == nil {
			t.Error("Expected Rules map to be initialized")
		}
		if len(runtime.Rules) != 0 {
			t.Error("Expected Rules map to be empty")
		}
	})

	t.Run("handles nil ux factory", func(t *testing.T) {
		runtime := New(100, nil)
		if runtime == nil {
			t.Fatal("Expected runtime to not be nil")
		}
		if runtime.Ux != nil {
			t.Error("Expected Ux to be nil")
		}
	})

	t.Run("handles zero stop value", func(t *testing.T) {
		runtime := New(0, ux.NewUxEval())
		if runtime == nil {
			t.Fatal("Expected runtime to not be nil")
		}
		if runtime.Stop != 0 {
			t.Errorf("Expected Stop to be 0, got %d", runtime.Stop)
		}
	})
}

func TestRuntimeT_AddRules(t *testing.T) {
	t.Run("adds rules successfully", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		rules := &parser.RulesT{
			Rules: []parser.ParseRuleT{
				{
					Metadata: parser.ParseRuleMetadataT{
						Id:   "test-rule-1",
						Hash: "hash1",
					},
					Cre: parser.ParseCreT{
						Id: "cre1",
					},
				},
			},
		}

		err := runtime.AddRules(rules)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(runtime.Rules) != 1 {
			t.Errorf("Expected 1 rule, got %d", len(runtime.Rules))
		}
		if runtime.Rules["hash1"].Id != rules.Rules[0].Cre.Id {
			t.Error("Expected rule ID to match")
		}
	})

	t.Run("handles duplicate rules", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		rules := &parser.RulesT{
			Rules: []parser.ParseRuleT{
				{
					Metadata: parser.ParseRuleMetadataT{
						Id:   "test-rule-1",
						Hash: "hash1",
					},
					Cre: parser.ParseCreT{
						Id: "cre1",
					},
				},
			},
		}

		// Add first rule
		err := runtime.AddRules(rules)
		if err != nil {
			t.Errorf("Expected no error on first add, got %v", err)
		}

		// Try to add duplicate rule
		err = runtime.AddRules(rules)
		if err != ErrDuplicateRule {
			t.Errorf("Expected ErrDuplicateRule, got %v", err)
		}
	})

	t.Run("handles multiple rules", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		rules := &parser.RulesT{
			Rules: []parser.ParseRuleT{
				{
					Metadata: parser.ParseRuleMetadataT{
						Id:   "test-rule-1",
						Hash: "hash1",
					},
					Cre: parser.ParseCreT{
						Id: "cre1",
					},
				},
				{
					Metadata: parser.ParseRuleMetadataT{
						Id:   "test-rule-2",
						Hash: "hash2",
					},
					Cre: parser.ParseCreT{
						Id: "cre2",
					},
				},
			},
		}

		err := runtime.AddRules(rules)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(runtime.Rules) != 2 {
			t.Errorf("Expected 2 rules, got %d", len(runtime.Rules))
		}
	})

	t.Run("handles empty rules slice", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		rules := &parser.RulesT{
			Rules: []parser.ParseRuleT{},
		}

		err := runtime.AddRules(rules)
		if err != nil {
			t.Errorf("Expected no error for empty rules, got %v", err)
		}
		if len(runtime.Rules) != 0 {
			t.Error("Expected no rules to be added")
		}
	})
}

func TestRuntimeT_GetCre(t *testing.T) {
	t.Run("returns rule when found", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		expectedCre := parser.ParseCreT{Id: "test-cre"}
		runtime.Rules["test-hash"] = expectedCre

		cre, err := runtime.getCre("test-hash")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if cre.Id != expectedCre.Id {
			t.Error("Expected CRE ID to match")
		}
	})

	t.Run("returns error when rule not found", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())

		cre, err := runtime.getCre("non-existent")
		if err != ErrRuleNotFound {
			t.Errorf("Expected ErrRuleNotFound, got %v", err)
		}
		if cre.Id != "" {
			t.Error("Expected empty CRE ID when rule not found")
		}
	})

	t.Run("handles empty hash", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())

		cre, err := runtime.getCre("")
		if err != ErrRuleNotFound {
			t.Errorf("Expected ErrRuleNotFound, got %v", err)
		}
		if cre.Id != "" {
			t.Error("Expected empty CRE ID when rule not found")
		}
	})
}

func TestRuntimeT_Close(t *testing.T) {
	t.Run("closes runtime without error", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		err := runtime.Close()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("handles multiple close calls", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		err := runtime.Close()
		if err != nil {
			t.Errorf("Expected no error on first close, got %v", err)
		}
		err = runtime.Close()
		if err != nil {
			t.Errorf("Expected no error on second close, got %v", err)
		}
	})
}

func TestRuntimeT_Run(t *testing.T) {
	t.Run("handles empty sources", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		matchers := &RuleMatchersT{
			match:    make(map[string]any),
			cb:       make(map[string]compiler.CallbackT),
			eventSrc: make(map[string]parser.ParseEventT),
		}
		report := ux.NewReport(nil)

		err := runtime.Run(context.Background(), matchers, []*LogData{}, report)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("handles nil matchers", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		report := ux.NewReport(nil)

		err := runtime.Run(context.Background(), nil, []*LogData{}, report)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("handles nil report", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		matchers := &RuleMatchersT{
			match:    make(map[string]any),
			cb:       make(map[string]compiler.CallbackT),
			eventSrc: make(map[string]parser.ParseEventT),
		}

		err := runtime.Run(context.Background(), matchers, []*LogData{}, nil)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("handles nil context", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		matchers := &RuleMatchersT{
			match:    make(map[string]any),
			cb:       make(map[string]compiler.CallbackT),
			eventSrc: make(map[string]parser.ParseEventT),
		}
		report := ux.NewReport(nil)

		err := runtime.Run(nil, matchers, []*LogData{}, report)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("handles nil sources", func(t *testing.T) {
		runtime := New(100, ux.NewUxEval())
		matchers := &RuleMatchersT{
			match:    make(map[string]any),
			cb:       make(map[string]compiler.CallbackT),
			eventSrc: make(map[string]parser.ParseEventT),
		}
		report := ux.NewReport(nil)

		err := runtime.Run(context.Background(), matchers, nil, report)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
