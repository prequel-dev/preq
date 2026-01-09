package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/prequel-dev/prequel-logmatch/pkg/timez"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type Config struct {
	TimestampRegexes []Regex        `yaml:"timestamps"`
	Rules            Rules          `yaml:"rules"`
	UpdateFrequency  *time.Duration `yaml:"updateFrequency"`
	RulesVersion     string         `yaml:"rulesVersion"`
	AcceptUpdates    bool           `yaml:"acceptUpdates"`
	DataSources      string         `yaml:"dataSources"`
	Window           time.Duration  `yaml:"window"`
	Skip             int            `yaml:"skip"`
}

type Rules struct {
	Paths    []string `yaml:"paths"`
	Disabled bool     `yaml:"disableCommunityRules"`
}

type Regex struct {
	Pattern string `yaml:"pattern"`
	Format  string `yaml:"format"`
}

type OptT func(*optsT)

type optsT struct {
	window time.Duration
}

func WithWindow(window time.Duration) func(*optsT) {
	return func(o *optsT) {
		o.window = window
	}
}

func parseOpts(opts ...OptT) *optsT {
	o := &optsT{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// LoadConfig loads the configuration from the specified directory and file.
// If the file does not exist, it creates a return a default configuration.

func LoadConfig(dir, file string, opts ...OptT) (*Config, error) {

	spec := filepath.Join(dir, file)
	_, err := os.Stat(spec)

	switch {
	case err == nil: // NOOP
	case os.IsNotExist(err):
		log.Info().
			Str("file", spec).
			Msg("Configuration file does not exist, using default configuration")
		return DefaultConfig(opts...), nil
	default:
		return nil, err
	}

	log.Info().Str("file", spec).Msg("Loading configuration file")
	fh, err := os.OpenFile(spec, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	return ReadConfig(fh)
}

func ReadConfig(rd io.Reader) (*Config, error) {
	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func DefaultConfig(opts ...OptT) *Config {
	o := parseOpts(opts...)

	c := &Config{
		Window: o.window,
	}
	for _, r := range timez.Defaults {
		c.TimestampRegexes = append(c.TimestampRegexes, Regex{
			Pattern: r.Pattern,
			Format:  string(r.Format),
		})
	}
	return c
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
