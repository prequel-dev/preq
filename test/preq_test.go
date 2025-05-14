package test

import (
	"context"
	"os"
	"testing"

	"github.com/prequel-dev/preq/internal/pkg/logs"
	"github.com/prequel-dev/preq/pkg/eval"
	"github.com/rs/zerolog/log"
)

func initLogger() {
	logs.InitLogger(
		logs.WithPretty(),
		logs.WithLevel(""))
}

func TestMain(m *testing.M) {
	initLogger()
	code := m.Run()
	os.Exit(code)
}

func TestSuccessExamples(t *testing.T) {

	var tests = map[string]struct {
		rulePath string
		dataPath string
	}{
		"Example00": {
			rulePath: "../examples/00-rules-document-example.yaml",
			dataPath: "../examples/00-example.log",
		},
		"Example01": {
			rulePath: "../examples/01-set-single-example.yaml",
			dataPath: "../examples/01-example.log",
		},
		"Example02-good": {
			rulePath: "../examples/02-set-multiple-example-good-window.yaml",
			dataPath: "../examples/02-example.log",
		},
		"Example03": {
			rulePath: "../examples/03-set-negative-example.yaml",
			dataPath: "../examples/03-example.log",
		},
		"Example04": {
			rulePath: "../examples/04-set-1x1-example.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example08": {
			rulePath: "../examples/08-sequence-example-good-window.yaml",
			dataPath: "../examples/08-example.log",
		},
		"Example09": {
			rulePath: "../examples/09-sequence-negate-example.yaml",
			dataPath: "../examples/09-example.log",
		},
		"Example13": {
			rulePath: "../examples/13-string-example.yaml",
			dataPath: "../examples/13-example.log",
		},
		"Example14": {
			rulePath: "../examples/14-string-example.yaml",
			dataPath: "../examples/14-example.log",
		},
		"Example15": {
			rulePath: "../examples/15-regex-example.yaml",
			dataPath: "../examples/15-example.log",
		},
		"Example16": {
			rulePath: "../examples/16-regex-example.yaml",
			dataPath: "../examples/16-example.log",
		},
		"Example17": {
			rulePath: "../examples/17-jq-example.yaml",
			dataPath: "../examples/17-example.log",
		},
		"Example18": {
			rulePath: "../examples/18-jq-example.yaml",
			dataPath: "../examples/18-example.log",
		},
		"Example21": {
			rulePath: "../examples/21-negative-example.yaml",
			dataPath: "../examples/21-example.log",
		},
		"Example22": {
			rulePath: "../examples/21-negative-example.yaml",
			dataPath: "../examples/22-example.log",
		},
		"Example23": {
			rulePath: "../examples/21-negative-example.yaml",
			dataPath: "../examples/23-example.log",
		},
		"Example25": {
			rulePath: "../examples/25-negate-options-1x1.yaml",
			dataPath: "../examples/25-example-new-negate-time.log",
		},
		"Example26": {
			rulePath: "../examples/26-negate-window.yaml",
			dataPath: "../examples/26-example-moved-negative.log",
		},
		"Example27": {
			rulePath: "../examples/27-negate-window.yaml",
			dataPath: "../examples/27-example-smaller.log",
		},
		"Example27-shorter": {
			rulePath: "../examples/27-negate-window-shorter.yaml",
			dataPath: "../examples/27-example.log",
		},
		"Example29": {
			rulePath: "../examples/29-negate-slide-anchor-1.yaml",
			dataPath: "../examples/29-example.log",
		},
		"Example29-window": {
			rulePath: "../examples/29-negate-slide-anchor-1-window.yaml",
			dataPath: "../examples/29-example-fp-moved.log",
		},
	}

	ctx := context.Background()

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			log.Info().Str("rule", test.rulePath).Msg("Running test")

			ruleData, err := os.ReadFile(test.rulePath)
			if err != nil {
				t.Fatalf("Error reading rule file %s: %v", test.rulePath, err)
			}

			data, err := os.ReadFile(test.dataPath)
			if err != nil {
				t.Fatalf("Error reading data file %s: %v", test.dataPath, err)
			}

			_, stats, err := eval.Detect(ctx, "", string(data), string(ruleData))
			if err != nil {
				t.Fatalf("Error running detection: %v", err)
			}

			if stats["problems"] == 0 {
				t.Fatalf("Expected problems, got %d", stats["problems"])
			}
		})
	}
}

func TestMissExamples(t *testing.T) {

	var tests = map[string]struct {
		rulePath string
		dataPath string
	}{
		"Example02-miss": {
			rulePath: "../examples/02-set-multiple-example-bad-window.yaml",
			dataPath: "../examples/02-example.log",
		},
		"Example08-miss": {
			rulePath: "../examples/08-sequence-example-bad-window.yaml",
			dataPath: "../examples/08-example.log",
		},
		"Example19-miss": {
			rulePath: "../examples/19-bad-literal-block-example.yaml",
			dataPath: "../examples/18-example.log",
		},
		"Example20-miss": {
			rulePath: "../examples/20-bad-regex-example.yaml",
			dataPath: "../examples/18-example.log",
		},
		"Example21-miss": {
			rulePath: "../examples/21-negative-example.yaml",
			dataPath: "../examples/21-example.log",
		},
		"Example24-miss": {
			rulePath: "../examples/24-multiple-negatives.yaml",
			dataPath: "../examples/24-example.log",
		},
		"Example25-miss": {
			rulePath: "../examples/25-negate-options-1x1.yaml",
			dataPath: "../examples/25-example.log",
		},
		"Example26-miss": {
			rulePath: "../examples/26-negate-window.yaml",
			dataPath: "../examples/26-example.log",
		},
		"Example27-miss": {
			rulePath: "../examples/27-negate-window.yaml",
			dataPath: "../examples/27-example.log",
		},
		"Example28-miss": {
			rulePath: "../examples/28-negate-anchor.yaml",
			dataPath: "../examples/28-example.log",
		},
		"Example29-miss": {
			rulePath: "../examples/29-negate-slide.yaml",
			dataPath: "../examples/29-example.log",
		},
		"Example29-miss-window": {
			rulePath: "../examples/29-negate-slide-anchor-1-window.yaml",
			dataPath: "../examples/29-example.log",
		},
		"Example30-miss": {
			rulePath: "../examples/30-negate-absolute.yaml",
			dataPath: "../examples/30-example.log",
		},
		"Missing-IDs": {
			rulePath: "missing-ids.yaml",
			dataPath: "missing-ids.log",
		},
	}

	ctx := context.Background()

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			log.Info().Str("rule", test.rulePath).Msg("Running test")

			ruleData, err := os.ReadFile(test.rulePath)
			if err != nil {
				t.Fatalf("Error reading rule file %s: %v", test.rulePath, err)
			}

			data, err := os.ReadFile(test.dataPath)
			if err != nil {
				t.Fatalf("Error reading data file %s: %v", test.dataPath, err)
			}

			_, stats, err := eval.Detect(ctx, "", string(data), string(ruleData))
			if err != nil {
				t.Fatalf("Error running detection: %v", err)
			}

			if stats["problems"] != uint32(0) {
				t.Fatalf("Expected no problems, got %d", stats["problems"])
			}
		})
	}
}

func TestFailureExamples(t *testing.T) {

	var tests = map[string]struct {
		rulePath string
		dataPath string
	}{
		"Example05-bad": {
			rulePath: "../examples/05-bad-set-example.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example06-bad": {
			rulePath: "../examples/06-bad-set-example.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example07-bad-sequence": {
			rulePath: "../examples/07-bad-sequence-example.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example07-bad-window": {
			rulePath: "../examples/07-bad-set-window.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example10-bad": {
			rulePath: "../examples/10-bad-sequence-match-example.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example11-bad": {
			rulePath: "../examples/11-bad-sequence-one-condition.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example12-bad": {
			rulePath: "../examples/12-bad-sequence-one-negate.yaml",
			dataPath: "../examples/04-example.log",
		},
		"Example31-bad": {
			rulePath: "../examples/31-bad-negate-anchor.yaml",
			dataPath: "../examples/04-example.log",
		},
	}

	ctx := context.Background()

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			log.Info().Str("rule", test.rulePath).Msg("Running test")

			ruleData, err := os.ReadFile(test.rulePath)
			if err != nil {
				t.Fatalf("Error reading rule file %s: %v", test.rulePath, err)
			}

			data, err := os.ReadFile(test.dataPath)
			if err != nil {
				t.Fatalf("Error reading data file %s: %v", test.dataPath, err)
			}

			_, _, err = eval.Detect(ctx, "", string(data), string(ruleData))
			if err == nil {
				t.Fatalf("Expected error running detection: %v", err)
			}
		})
	}
}
