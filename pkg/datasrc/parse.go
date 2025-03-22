package datasrc

import (
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// version: 0.0.1
// sources:
//   - name: thurderbird
//     type: keda-operator
//     desc: "https://github.com/logpai/loghub/tree/master/Thunderbird"
//     locations:
//       - timestamp:
//           regex: "^- (\\d{10}) \\d{4}.\\d{2}.\\d{2} "
//           format: "epochseconds"
//         path: /Users/sgc/src/sample_log_files/Thunderbird.log
//   - name: syslog
//     type: log
//     desc: "sample syslog"
//     locations:
//       - path: /tmp/wonk/syslog
//		   window: 5m

type DataSources struct {
	Version string   `yaml:"version"`
	Sources []Source `yaml:"sources"`
}

type Source struct {
	Type      string        `yaml:"type"`
	Name      string        `yaml:"name,omitempty"`
	Desc      string        `yaml:"desc,omitempty"`
	Window    time.Duration `yaml:"window,omitempty"`
	Timestamp *Timestamp    `yaml:"timestamp,omitempty"`
	Locations []Location    `yaml:"locations"`
}

type Timestamp struct {
	Regex  string `yaml:"regex"`
	Format string `yaml:"format"`
}

type Location struct {
	Path      string        `yaml:"path,omitempty"`
	Type      string        `yaml:"type,omitempty"`
	Window    time.Duration `yaml:"window,omitempty"`
	Timestamp *Timestamp    `yaml:"timestamp,omitempty"`
}

func Parse(data []byte) (*DataSources, error) {
	var ds DataSources
	err := yaml.Unmarshal(data, &ds)
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

func Validate(ds *DataSources) error {
	return nil
}

func ParseFile(fn string) (*DataSources, error) {

	fh, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	data, err := io.ReadAll(fh)
	if err != nil {
		return nil, err
	}

	return Parse(data)
}
