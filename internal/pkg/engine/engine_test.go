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
		uxFactory := &ux.UxEvalT{}

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
}

func TestRuntimeT_AddRules(t *testing.T) {
	t.Run("adds rules successfully", func(t *testing.T) {
		runtime := New(100, &ux.UxEvalT{})
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
		runtime := New(100, &ux.UxEvalT{})
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
}

func TestRuntimeT_GetCre(t *testing.T) {
	t.Run("returns rule when found", func(t *testing.T) {
		runtime := New(100, &ux.UxEvalT{})
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
		runtime := New(100, &ux.UxEvalT{})

		cre, err := runtime.getCre("non-existent")
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
		runtime := New(100, &ux.UxEvalT{})
		err := runtime.Close()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestRuntimeT_Run(t *testing.T) {
	t.Run("handles empty sources", func(t *testing.T) {
		runtime := New(100, &ux.UxEvalT{})
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
		runtime := New(100, &ux.UxEvalT{})
		report := ux.NewReport(nil)

		err := runtime.Run(context.Background(), nil, []*LogData{}, report)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
