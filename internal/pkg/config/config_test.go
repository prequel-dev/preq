package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/config"
)

func TestMarshal(t *testing.T) {
	out := config.Marshal()
	if !strings.Contains(out, "timestamps:") {
		t.Fatalf("expected timestamps in output")
	}

	out = config.Marshal(config.WithWindow(2 * time.Second))
	if !strings.Contains(out, "window: 2s") {
		t.Fatalf("expected window option in output")
	}
}

func TestLoadConfig(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(dir, "cfg.yaml")); err != nil {
		t.Fatalf("expected config file written: %v", err)
	}
}

func TestLoadConfigFromBytes(t *testing.T) {
	data := "timestamps: []\nwindow: 1s\n"
	cfg, err := config.LoadConfigFromBytes(data)
	if err != nil {
		t.Fatalf("LoadConfigFromBytes: %v", err)
	}
	if cfg.Window != time.Second {
		t.Fatalf("expected 1s window, got %v", cfg.Window)
	}
}
