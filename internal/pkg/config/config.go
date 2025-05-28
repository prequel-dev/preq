package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var (
	defaultConfig = `timestamps:

  # Example: {"level":"error","error":"context deadline exceeded","time":1744570895480541,"caller":"server.go:462"}
  - format: epochany
    pattern: |
      "time":(\d{16,19})

  # Example: 2006-01-02T15:04:05Z07:00 <log message>
  - format: rfc3339
    pattern: |
      ^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+\-]\d{2}:\d{2}))

  # Example: 2006/01/02 03:04:05 <log message>
  - format: "2006/01/02 03:04:05"
    pattern: |
      ^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})

  # Example: 2006-01-02 15:04:05.000 <log message>
  # Source: ISO 8601
  - format: "2006-01-02 15:04:05.000"
    pattern: |
      ^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})

  # Example: Apr 30 23:36:47.715984 WRN <log message>
  # Source: RFC 3164 extended
  - format: "Jan 2 15:04:05.000000"
    pattern: |
      ^([A-Z][a-z]{2}\s{1,2}\d{1,2}\s\d{2}:\d{2}:\d{2}\.\d{6})

  # Example: Jan 2 15:04:05 <log message>
  # Source: RFC 3164
  - format: "Jan 2 15:04:05"
    pattern: |
      ^([A-Z][a-z]{2}\s{1,2}\d{1,2}\s\d{2}:\d{2}:\d{2})

  # Example: 2006-01-02 15:04:05 <log message>
  # Source: w3c, Postgres
  - format: "2006-01-02 15:04:05"
    pattern: |
      ^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})

  # Example: I0102 15:04:05.000000 <log message>
  # Source: go/klog
  - format: "0102 15:04:05.000000"
    pattern: |
      ^[IWEF](\d{4} \d{2}:\d{2}:\d{2}\.\d{6})

  # Example: [2006-01-02 15:04:05,000] <log message>
  - format: "2006-01-02 15:04:05,000"
    pattern: |
      ^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3})\]

  # Example: 2006-01-02 15:04:05.000000-0700 <log message>
  - format: "2006-01-02 15:04:05.000000-0700"
    pattern: |
      ^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{6}[+-]\d{4})

  # Example: 2006/01/02 15:04:05 <log message>
  - format: "2006/01/02 15:04:05"
    pattern: |
      ^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})

  # Example: 01/02/2006, 15:04:05 <log message>
  # Source: IIS format
  - format: "01/02/2006, 15:04:05"
    pattern: |
      ^(\d{2}/\d{2}/\d{4}, \d{2}:\d{2}:\d{2})
 
  # Example: 02 Jan 2006 15:04:05.000 <log message>
  - format: "02 Jan 2006 15:04:05.000" 
    pattern: |
      ^(\d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2}\.\d{3})
  
  # Example: 2006 Jan 02 15:04:05.000 <log message>
  - format: "2006 Jan 02 15:04:05.000"
    pattern: |
      ^(\d{4} [A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2}\.\d{3})

  # Example: 02/Jan/2006:15:04:05.000 <log message>
  - format: "02/Jan/2006:15:04:05.000"
    pattern: |
      ^(\d{2}/[A-Z][a-z]{2}/\d{4}:\d{2}:\d{2}:\d{2}\.\d{3})

  # Example: 01/02/2006 03:04:05 PM <log message>
  - format: "01/02/2006 03:04:05 PM"
    pattern: |
      ^(\d{2}/\d{2}/\d{4} \d{2}:\d{2}:\d{2} (AM|PM))

  # Example: 2006 Jan 02 15:04:05 <log message>
  - format: "2006 Jan 02 15:04:05" 
    pattern: |
      ^(\d{4} [A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2})

  # Example: 2006-01-02 15:04:05.000 <log message>
  - format: "2006-01-02 15:04:05.000"
    pattern: |
      ^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})

  # Example: {"timestamp":"2025-03-26T14:01:02Z","level":"info", "message":"..."}
  # Source: Postgres JSON output
  - format: rfc3339
    pattern: |
      "timestamp"\s*:\s*"([^"]+)"
 
  # Example: {"ts":"2025-03-26T14:01:02Z","level":"info", "message":"..."}
  # Source: metallb
  - format: rfc3339
    pattern: |
      "ts"\s*:\s*"([^"]+)"

  # Example: [7] 2025/04/25 02:01:04.339092 [ERR] 10.0.6.53:27827 - cid:10110160 - TLS handshake error: EOF
  # Source: NATS
  - format: "2006/01/02 15:04:05.000000"
    pattern: |
      ^\[\d+\]\s+(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{6})

  # Example: {"creationTimestamp":"2025-04-23T20:50:35Z","name":"insecure-nginx-conf","namespace":"default","resourceVersion":"825013"}
  # Source: Kubernetes events, configmaps
  - format: rfc3339
    pattern: |
      "creationTimestamp":"([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z)"

  # Example: 2025-04-24T21:55:08.535-0500	INFO	example-log-entry
  # Source: ZAP production
  - format: "2006-01-02T15:04:05.000-0700"
    pattern: |
      ^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[+-]\d{4})

  # Example: {"level":"info","ts":1745549708.5355184,"msg":"example-log-entry"}
  # Source: ZAP development
  - format: epochany
    pattern: |
      "ts"\s*:\s*([0-9]+)(?:\.[0-9]+)?

  # Example: ts=2025-03-10T13:52:40.623431174Z level=info msg="tail routine: tail channel closed...
  # Source: Loki
  - format: "2006-01-02T15:04:05.000000000Z"
    pattern: |
      ts=([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{9}Z)

  # Example: {"event": "DD_API_KEY undefined. Metrics, logs and events will not be reported to DataDog", "timestamp": "2025-02-12T18:12:58.715528Z", "level": "warn...
  # Source: DataDog
  - format: "2006-01-02T15:04:05.000000Z"
    pattern: |
      "timestamp"\s*:\s*"([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{6}Z)"

  # Example: {"Id":19,"Version":1,"Opcode":13,"RecordId":1493,"LogName":"System","ProcessId":4324,"ThreadId":10456,"MachineName":"windows","TimeCreated":"\/Date(1743448267142)\/"}
  # Source; Windows events via Get-Events w/ JSON output
  - format: epochany
    pattern: |
      /Date\((\d+)\)

  # Example: time="2025-02-12T18:12:58.715528Z"
  # Source: argocd
  - format: rfc3339
    pattern: |
      time="([^"]+)"
`

	windowConfig = `window: %s`
)

type NotificationWebhook struct {
	Type    string `yaml:"type"`
	Webhook string `yaml:"webhook"`
}

type Config struct {
	TimestampRegexes []Regex             `yaml:"timestamps"`
	Rules            Rules               `yaml:"rules"`
	UpdateFrequency  *time.Duration      `yaml:"updateFrequency"`
	RulesVersion     string              `yaml:"rulesVersion"`
	AcceptUpdates    bool                `yaml:"acceptUpdates"`
	DataSources      string              `yaml:"dataSources"`
	Notification     NotificationWebhook `yaml:"notification"`
	Window           time.Duration       `yaml:"window"`
	Skip             int                 `yaml:"skip"`
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

func Marshal(opts ...OptT) string {
	o := parseOpts(opts...)

	if o.window > 0 {
		wcfg := fmt.Sprintf(windowConfig, o.window)
		return fmt.Sprintf("%s\n%s\n", defaultConfig, wcfg)
	}

	return defaultConfig
}

func LoadConfig(dir, file string, opts ...OptT) (*Config, error) {

	var config Config

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(filepath.Join(dir, file)); os.IsNotExist(err) {
		if err := WriteDefaultConfig(filepath.Join(dir, file), opts...); err != nil {
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

func WriteDefaultConfig(path string, opts ...OptT) error {
	cfg := Marshal(opts...)
	return os.WriteFile(path, []byte(cfg), 0644)
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
