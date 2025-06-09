package ux

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNewUxEval(t *testing.T) {
	t.Run("creates new UxEval with correct initial values", func(t *testing.T) {
		eval := NewUxEval()

		if eval == nil {
			t.Fatal("Expected UxEval to not be nil")
		}
		if eval.Rules != 0 {
			t.Errorf("Expected Rules to be 0, got %d", eval.Rules)
		}
		if eval.Problems != 0 {
			t.Errorf("Expected Problems to be 0, got %d", eval.Problems)
		}
		if eval.Lines.Load() != 0 {
			t.Errorf("Expected Lines to be 0, got %d", eval.Lines.Load())
		}
		if eval.Bytes.Value() != 0 {
			t.Errorf("Expected Bytes to be 0, got %d", eval.Bytes.Value())
		}
		if eval.done == nil {
			t.Error("Expected done channel to be initialized")
		}
	})
}

func TestUxEvalT_IncrementRuleTracker(t *testing.T) {
	t.Run("increments rule counter", func(t *testing.T) {
		eval := NewUxEval()
		eval.IncrementRuleTracker(1)
		if eval.Rules != 1 {
			t.Errorf("Expected Rules to be 1, got %d", eval.Rules)
		}
	})
}

func TestUxEvalT_IncrementProblemsTracker(t *testing.T) {
	t.Run("increments problems counter", func(t *testing.T) {
		eval := NewUxEval()
		eval.IncrementProblemsTracker(1)
		if eval.Problems != 1 {
			t.Errorf("Expected Problems to be 1, got %d", eval.Problems)
		}
	})
}

func TestUxEvalT_StartLinesTracker(t *testing.T) {
	t.Run("updates lines count from atomic counter", func(t *testing.T) {
		eval := NewUxEval()
		var lines atomic.Int64
		lines.Store(100)

		killCh := make(chan struct{})
		eval.StartLinesTracker(&lines, killCh)

		// Give some time for the goroutine to process
		time.Sleep(100 * time.Millisecond)

		// Close the kill channel to stop the tracker
		close(killCh)

		// Give some time for the goroutine to finish
		time.Sleep(100 * time.Millisecond)

		if eval.Lines.Load() != 100 {
			t.Errorf("Expected Lines to be 100, got %d", eval.Lines.Load())
		}
	})
}

func TestUxEvalT_FinalStats(t *testing.T) {
	t.Run("returns correct final statistics", func(t *testing.T) {
		eval := NewUxEval()
		eval.Rules = 5
		eval.Problems = 2
		eval.Lines.Store(1000)
		eval.Bytes.Increment(5000)

		// Close the done channel to simulate completion
		close(eval.done)

		stats, err := eval.FinalStats()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Convert map values to the correct type for comparison
		rules, ok := stats["rules"].(uint32)
		if !ok {
			t.Fatal("Expected rules to be uint32")
		}
		if rules != 5 {
			t.Errorf("Expected rules to be 5, got %d", rules)
		}

		problems, ok := stats["problems"].(uint32)
		if !ok {
			t.Fatal("Expected problems to be uint32")
		}
		if problems != 2 {
			t.Errorf("Expected problems to be 2, got %d", problems)
		}

		lines, ok := stats["lines"].(int64)
		if !ok {
			t.Fatal("Expected lines to be int64")
		}
		if lines != 1000 {
			t.Errorf("Expected lines to be 1000, got %d", lines)
		}

		bytes, ok := stats["bytes"].(int64)
		if !ok {
			t.Fatal("Expected bytes to be int64")
		}
		if bytes != 5000 {
			t.Errorf("Expected bytes to be 5000, got %d", bytes)
		}
	})
}
