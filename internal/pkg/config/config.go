package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prequel-dev/detection-engine/internal/pkg/resolve"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var (
	DefaultConfig = `
timestamp_regexes:
  - format: epochany
    pattern: |
      "time":(\d{16,19})
  - format: rfc3339
    pattern: |
      (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+\-]\d{2}:\d{2}))
  - format: "2006/01/02 03:04:05"
    pattern: |
      (\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})
`
)

type Config struct {
	TimestampRegexes []Regex        `yaml:"timestamp_regexes"`
	Rules            Rules          `yaml:"rules"`
	UpdateFrequency  *time.Duration `yaml:"update_frequency"`
	RulesVersion     string         `yaml:"rules_version"`
	AcceptUpdates    bool           `yaml:"accept_updates"`
	DataSources      string         `yaml:"data_sources"`
}

type Rules struct {
	Paths    []string `yaml:"paths"`
	Disabled bool     `yaml:"disable_community_rules"`
}

type Regex struct {
	Pattern string `yaml:"pattern"`
	Format  string `yaml:"format"`
}

func LoadConfig(dir, file string) (*Config, error) {
	var config Config

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(filepath.Join(dir, file)); os.IsNotExist(err) {
		if err := WriteDefaultConfig(filepath.Join(dir, file)); err != nil {
			log.Error().Err(err).Msg("Failed to write default config")
			return nil, err
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func WriteDefaultConfig(path string) error {
	return os.WriteFile(path, []byte(DefaultConfig), 0644)
}

func LoadConfigFromBytes(data string) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Config) ResolveOpts() (opts []resolve.OptT) {

	if len(c.TimestampRegexes) > 0 {
		var specs []resolve.FmtSpec
		for _, r := range c.TimestampRegexes {
			specs = append(specs, resolve.FmtSpec{
				Pattern: strings.TrimSpace(r.Pattern),
				Format:  resolve.TimestampFmt(strings.TrimSpace(r.Format)),
			})
		}
		opts = append(opts, resolve.WithStampRegex(specs...))
	}

	return

}
