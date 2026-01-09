package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/config"
)

func TestLoadConfig_FileDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.LoadConfig(dir, "cfg.yaml", config.WithWindow(3*time.Second))
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Window != 3*time.Second {
		t.Fatalf("expected window 3s got %v", cfg.Window)
	}
	if len(cfg.TimestampRegexes) == 0 {
		t.Fatalf("expected default timestamp regexes")
	}
}

func TestLoadConfig_FileExists(t *testing.T) {
	dir := t.TempDir()

	// Create a config file
	configPath := filepath.Join(dir, "cfg.yaml")
	configContent := `window: 5s
skip: 10
dataSources: "test-source"
acceptUpdates: true
rulesVersion: "v1.0"
timestamps:
  - pattern: "test-pattern"
    format: "test-format"
rules:
  paths:
    - "/path/to/rules"
  disableCommunityRules: true
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := config.LoadConfig(dir, "cfg.yaml")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Window != 5*time.Second {
		t.Fatalf("expected window 5s got %v", cfg.Window)
	}
	if cfg.Skip != 10 {
		t.Fatalf("expected skip 10 got %v", cfg.Skip)
	}
	if cfg.DataSources != "test-source" {
		t.Fatalf("expected dataSources 'test-source' got %v", cfg.DataSources)
	}
	if !cfg.AcceptUpdates {
		t.Fatalf("expected acceptUpdates true")
	}
	if cfg.RulesVersion != "v1.0" {
		t.Fatalf("expected rulesVersion 'v1.0' got %v", cfg.RulesVersion)
	}
	if len(cfg.TimestampRegexes) == 0 {
		t.Fatalf("expected timestamp regexes")
	}
	if cfg.TimestampRegexes[0].Pattern != "test-pattern" {
		t.Fatalf("expected pattern 'test-pattern' got %v", cfg.TimestampRegexes[0].Pattern)
	}
	if len(cfg.Rules.Paths) == 0 {
		t.Fatalf("expected rules paths")
	}
	if !cfg.Rules.Disabled {
		t.Fatalf("expected rules disabled true")
	}
}

func TestLoadConfig_StatError(t *testing.T) {
	dir := t.TempDir()

	// Create a directory and then make it unreadable
	subdir := filepath.Join(dir, "unreadable")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create a file in the directory first
	configPath := filepath.Join(subdir, "cfg.yaml")
	if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Make the directory unreadable (no execute permission means can't access files in it)
	if err := os.Chmod(subdir, 0000); err != nil {
		t.Fatalf("Failed to chmod directory: %v", err)
	}
	defer os.Chmod(subdir, 0755) // Restore for cleanup

	_, err := config.LoadConfig(subdir, "cfg.yaml")
	if err == nil {
		t.Fatalf("expected error for inaccessible directory")
	}
}

func TestLoadConfig_OpenFileError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.yaml")

	// Create a file with restricted permissions
	if err := os.WriteFile(configPath, []byte("test"), 0000); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := config.LoadConfig(dir, "cfg.yaml")
	if err == nil {
		t.Fatalf("expected error when opening file with no permissions")
	}
}

func TestReadConfig(t *testing.T) {
	configContent := `window: 2s
skip: 5
dataSources: "my-source"
acceptUpdates: false
rulesVersion: "v2.0"
timestamps:
  - pattern: "\\d{4}-\\d{2}-\\d{2}"
    format: "2006-01-02"
rules:
  paths:
    - "/rules/path1"
    - "/rules/path2"
  disableCommunityRules: false
`
	reader := strings.NewReader(configContent)
	cfg, err := config.ReadConfig(reader)
	if err != nil {
		t.Fatalf("ReadConfig error: %v", err)
	}

	if cfg.Window != 2*time.Second {
		t.Fatalf("expected window 2s got %v", cfg.Window)
	}
	if cfg.Skip != 5 {
		t.Fatalf("expected skip 5 got %v", cfg.Skip)
	}
	if cfg.DataSources != "my-source" {
		t.Fatalf("expected dataSources 'my-source' got %v", cfg.DataSources)
	}
	if cfg.AcceptUpdates {
		t.Fatalf("expected acceptUpdates false")
	}
	if len(cfg.TimestampRegexes) != 1 {
		t.Fatalf("expected 1 timestamp regex got %v", len(cfg.TimestampRegexes))
	}
	if len(cfg.Rules.Paths) != 2 {
		t.Fatalf("expected 2 rules paths got %v", len(cfg.Rules.Paths))
	}
}

func TestReadConfig_InvalidYAML(t *testing.T) {
	invalidYAML := `invalid: yaml: content: [[[`
	reader := strings.NewReader(invalidYAML)
	_, err := config.ReadConfig(reader)
	if err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, os.ErrInvalid
}

func TestReadConfig_ReadError(t *testing.T) {
	_, err := config.ReadConfig(&errorReader{})
	if err == nil {
		t.Fatalf("expected error when reading fails")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Window != 0 {
		t.Fatalf("expected default window 0 got %v", cfg.Window)
	}
	if len(cfg.TimestampRegexes) == 0 {
		t.Fatalf("expected default timestamp regexes")
	}
}

func TestDefaultConfig_WithWindow(t *testing.T) {
	cfg := config.DefaultConfig(config.WithWindow(10 * time.Second))

	if cfg.Window != 10*time.Second {
		t.Fatalf("expected window 10s got %v", cfg.Window)
	}
	if len(cfg.TimestampRegexes) == 0 {
		t.Fatalf("expected default timestamp regexes")
	}
}

func TestResolveOpts(t *testing.T) {
	cfg := &config.Config{
		TimestampRegexes: []config.Regex{
			{Pattern: "  test-pattern  ", Format: "  test-format  "},
			{Pattern: "pattern2", Format: "format2"},
		},
	}

	opts := cfg.ResolveOpts()
	if len(opts) == 0 {
		t.Fatalf("expected resolve opts")
	}
}

func TestResolveOpts_NoTimestamps(t *testing.T) {
	cfg := &config.Config{
		TimestampRegexes: []config.Regex{},
	}

	opts := cfg.ResolveOpts()
	if len(opts) != 0 {
		t.Fatalf("expected no resolve opts got %v", len(opts))
	}
}

func TestWithWindow(t *testing.T) {
	duration := 5 * time.Second
	optFunc := config.WithWindow(duration)

	cfg := config.DefaultConfig(optFunc)
	if cfg.Window != duration {
		t.Fatalf("expected window %v got %v", duration, cfg.Window)
	}
}
