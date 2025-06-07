package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/prequel-compiler/pkg/datasrc"
)

func setupTest(t *testing.T) {
	t.Cleanup(func() {
		Options = struct {
			Action        string `short:"a" help:"${actionHelp}"`
			Disabled      bool   `short:"d" help:"${disabledHelp}"`
			Generate      bool   `short:"g" help:"${generateHelp}"`
			Cron          bool   `short:"j" help:"${cronHelp}"`
			Level         string `short:"l" help:"${levelHelp}"`
			Name          string `short:"o" help:"${nameHelp}"`
			Quiet         bool   `short:"q" help:"${quietHelp}"`
			Rules         string `short:"r" help:"${rulesHelp}"`
			Source        string `short:"s" help:"${sourceHelp}"`
			Version       bool   `short:"v" help:"${versionHelp}"`
			AcceptUpdates bool   `short:"y" help:"${acceptUpdatesHelp}"`
		}{}
	})
}

func TestDataSourceFileParsing(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid data source file", func(t *testing.T) {
		validContent := `
version: 1.0
sources:
  - name: my-log-source
    type: log
    desc: "my-log"
    locations:
      - path: /some/path/app.log
`
		validFile := filepath.Join(tempDir, "valid_sources.yaml")
		if err := os.WriteFile(validFile, []byte(validContent), 0644); err != nil {
			t.Fatalf("Failed to write valid source file: %v", err)
		}

		parsedData, err := datasrc.ParseFile(validFile)
		if err != nil {
			t.Fatalf("datasrc.ParseFile failed on a valid file: %v", err)
		}

		err = datasrc.Validate(parsedData)
		if err != nil {
			t.Fatalf("datasrc.Validate failed on valid parsed data: %v", err)
		}
	})

	t.Run("invalid yaml syntax", func(t *testing.T) {
		invalidContent := "version: 1.0\n  sources: - name: broken"
		invalidFile := filepath.Join(tempDir, "invalid_sources.yaml")
		if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to write invalid source file: %v", err)
		}

		_, err := datasrc.ParseFile(invalidFile)
		if err == nil {
			t.Fatal("Expected an error for malformed YAML, but got nil")
		}
	})

	t.Run("source file not found", func(t *testing.T) {
		_, err := datasrc.ParseFile(filepath.Join(tempDir, "non_existent_file.yaml"))
		if err == nil {
			t.Fatal("Expected an error for a non-existent file, but got nil")
		}
	})
}

func TestInitAndExecute_OptionParsing(t *testing.T) {
	setupTest(t)

	originalGetRules := getRulesFunc
	originalLoginUser := loginUserFunc
	t.Cleanup(func() {
		getRulesFunc = originalGetRules
		loginUserFunc = originalLoginUser
	})

	tempDir := t.TempDir()

	dummyRuleContent := `
rules:
  - cre:
      id: cre-2025-0000
    metadata:
      id: mC5rnfG5qz4TyHNscXKuJL
      hash: cBsS3QQY1fwPVFUfYkKtHQ
    rule:
      set:
        window: 5s
        event:
          source: cre.log.kafka
        match:
          - commonExpression1
          - "this is another match"
`
	dummyRuleFile := filepath.Join(tempDir, "dummy-rule.yaml")
	if err := os.WriteFile(dummyRuleFile, []byte(dummyRuleContent), 0644); err != nil {
		t.Fatalf("Failed to write dummy rule file: %v", err)
	}

	var capturedCLIRules string
	getRulesFunc = func(ctx context.Context, conf *config.Config, configDir, cmdLineRules, token, updateFile, baseAddr string, tlsPort, udpPort int) ([]utils.RulePathT, error) {
		capturedCLIRules = cmdLineRules
		return []utils.RulePathT{
			{Path: dummyRuleFile, Type: utils.RuleTypeUser},
		}, nil
	}

	loginUserFunc = func(ctx context.Context, s1, s2 string) (string, error) {
		return "dummy-token", nil
	}

	expectedRulePath := "/path/to/my/rule.yaml"
	Options.Rules = expectedRulePath

	dummySourceFile := filepath.Join(tempDir, "dummy-source.yaml")
	os.WriteFile(dummySourceFile, []byte("version: 1"), 0644)
	Options.Source = dummySourceFile

	originalStdout := os.Stdout
	originalStderr := os.Stderr

	os.Stdout = nil
	os.Stderr = nil

	t.Cleanup(func() {
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	})

	err := InitAndExecute(context.Background())

	if err != nil {
		t.Errorf("Expected InitAndExecute to run without error, but got: %v", err)
	}

	if capturedCLIRules != expectedRulePath {
		t.Errorf("Expected CLI option for rules to be '%s', but captured '%s'", expectedRulePath, capturedCLIRules)
	}
}
