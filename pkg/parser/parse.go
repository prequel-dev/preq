package parser

import (
	"gopkg.in/yaml.v3"
)

// Note that we prefer lower camel case like Kubernetes

type ParseRuleT struct {
	Metadata ParseRuleMetadataT `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Cre      ParseCreT          `yaml:"cre,omitempty" json:"cre,omitempty"`
	Rule     ParseRuleDataT     `yaml:"rule,omitempty" json:"rule,omitempty"`
}

type ParseRuleMetadataT struct {
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`
	Id      string `yaml:"id,omitempty" json:"id,omitempty"`
	Hash    string `yaml:"hash,omitempty" json:"hash,omitempty"`
	Gen     uint   `yaml:"generation,omitempty" json:"generation,omitempty"`
	Kind    string `yaml:"kind,omitempty" json:"kind,omitempty"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

type ParseRuleDataT struct {
	Sequence *ParseSequenceT `yaml:"sequence,omitempty"`
	Set      *ParseSetT      `yaml:"set,omitempty"`
}

type ParseApplicationT struct {
	Name          string `yaml:"name,omitempty" json:"name,omitempty"`
	ProcessName   string `yaml:"processName,omitempty" json:"process_name,omitempty"`
	ProcessPath   string `yaml:"processPath,omitempty" json:"process_path,omitempty"`
	ContainerName string `yaml:"containerName,omitempty" json:"container_name,omitempty"`
	ImageUrl      string `yaml:"imageUrl,omitempty" json:"image_url,omitempty"`
	RepoUrl       string `yaml:"repoUrl,omitempty" json:"repo_url,omitempty"`
	Version       string `yaml:"version,omitempty" json:"version,omitempty"`
}

const (
	SeverityCritical = 0
	SeverityHigh     = 1
	SeverityMedium   = 2
	SeverityLow      = 3
	SeverityInfo     = 4
)

type ParseCreT struct {
	Id           string              `yaml:"id,omitempty" json:"id,omitempty"`
	Severity     uint                `yaml:"severity,omitempty" json:"severity,omitempty"`
	Title        string              `yaml:"title,omitempty" json:"title,omitempty"`
	Category     string              `yaml:"category,omitempty" json:"category,omitempty"`
	Tags         []string            `yaml:"tags,omitempty" json:"tags,omitempty"`
	Author       string              `yaml:"author,omitempty" json:"author,omitempty"`
	Description  string              `yaml:"description,omitempty" json:"description,omitempty"`
	Impact       string              `yaml:"impact,omitempty" json:"impact,omitempty"`
	Cause        string              `yaml:"cause,omitempty" json:"cause,omitempty"`
	Mitigation   string              `yaml:"mitigation,omitempty" json:"mitigation,omitempty"`
	References   []string            `yaml:"references,omitempty" json:"references,omitempty"`
	Reports      uint                `yaml:"reports,omitempty" json:"reports,omitempty"`
	Applications []ParseApplicationT `yaml:"applications,omitempty" json:"applications,omitempty"`
}

type ParseSequenceT struct {
	Window       string       `yaml:"window"`
	Correlations []string     `yaml:"correlations,omitempty"`
	Event        *ParseEventT `yaml:"event,omitempty"`
	Origin       bool         `yaml:"origin,omitempty"`
	Order        []ParseTermT `yaml:"order,omitempty"`
	Negate       []ParseTermT `yaml:"negate,omitempty"`
}

type ParseNegateOptsT struct {
	Window   string `yaml:"window,omitempty"`
	Slide    string `yaml:"slide,omitempty"`
	Anchor   uint32 `yaml:"anchor,omitempty"`
	Absolute bool   `yaml:"absolute,omitempty"`
}

type ParseTermT struct {
	Field      string            `yaml:"field,omitempty"`
	StrValue   string            `yaml:"value,omitempty"`
	JqValue    string            `yaml:"jq,omitempty"`
	RegexValue string            `yaml:"regex,omitempty"`
	Count      int               `yaml:"count,omitempty"`
	Set        *ParseSetT        `yaml:"set,omitempty"`
	Sequence   *ParseSequenceT   `yaml:"sequence,omitempty"`
	NegateOpts *ParseNegateOptsT `yaml:",inline,omitempty"`
}

type ParseSetT struct {
	Window       string       `yaml:"window,omitempty"`
	Correlations []string     `yaml:"correlations,omitempty"`
	Event        *ParseEventT `yaml:"event,omitempty"`
	Match        []ParseTermT `yaml:"match,omitempty"`
	Negate       []ParseTermT `yaml:"negate,omitempty"`
}

func (o *ParseTermT) UnmarshalYAML(unmarshal func(any) error) error {
	var str string
	if err := unmarshal(&str); err == nil {
		o.StrValue = str
		return nil
	}
	var temp struct {
		Field      string            `yaml:"field,omitempty"`
		StrValue   string            `yaml:"value,omitempty"`
		JqValue    string            `yaml:"jq,omitempty"`
		RegexValue string            `yaml:"regex,omitempty"`
		Count      int               `yaml:"count,omitempty"`
		Set        *ParseSetT        `yaml:"set,omitempty"`
		Sequence   *ParseSequenceT   `yaml:"sequence,omitempty"`
		NegateOpts *ParseNegateOptsT `yaml:",inline,omitempty"`
	}
	if err := unmarshal(&temp); err != nil {
		return err
	}
	o.Field = temp.Field
	o.StrValue = temp.StrValue
	o.JqValue = temp.JqValue
	o.RegexValue = temp.RegexValue
	o.Count = temp.Count
	o.Set = temp.Set
	o.Sequence = temp.Sequence
	o.NegateOpts = temp.NegateOpts
	return nil
}

type ParseEventT struct {
	Source string `yaml:"source"`
	Origin bool   `yaml:"origin,omitempty" json:"origin,omitempty"`
}

type RulesT struct {
	Rules []ParseRuleT          `yaml:"rules"`
	Terms map[string]ParseTermT `yaml:"terms"`
}

func _parse(data []byte) (RulesT, error) {
	var rules RulesT
	err := yaml.Unmarshal(data, &rules)
	return rules, err
}
