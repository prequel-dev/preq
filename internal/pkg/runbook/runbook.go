package runbook

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

/*
actions:
  - type: slack
    regex: "CRE-2025-00*"
    slack:
      webhook_url: https://hooks.slack.com/services/...
      message_template: |
           *preq detection*: [{{ field .cre "Id" }}] {{ field .cre "Title" }}

           {{ (index .hits 0).Timestamp }}: {{ (index .hits 0).Entry }}
  - type: exec
    regex: "CRE-2025-0025"
    exec:
      path: ./action.sh
      expr: |
        echo "Critical incident: {{ field .cre "Id" }}"
      runtime: bash -
      args:
        - '{{ field .cre "Id" }}'
        - '{{ len .hits }}'
  - type: jira
    regex: "CRE-2025-0025"
    jira:
      project_key: KAN
      webhook_url: https://prequel-team.atlassian.net/rest/api/3/issue
      secret_env: JIRA_TOKEN
      summary_template: |
        *preq detection*: [{{ field .cre "Id" }}] {{ field .cre "Title" }}
      description_template: |
        {{ (index .hits 0).Timestamp }}: {{ (index .hits 0).Entry }}
  - type: linear
    regex: "CRE-2025-0026"
    linear:
      team_id: 9cfb482a-81e3-4154-b5b9-2c805e70a02d
      secret_env: LINEAR_TOKEN
      title_template: |
        [{{ field .cre "Id" }}] {{ field .cre "Title" }}
      description_template: |
        {{ stripdash (field .cre "Description") }}
        ### Impact
        {{ stripdash (field .cre "Impact") }}
        ### Cause
        {{ stripdash (field .cre "Cause") }}
        ## Mitigation
        {{ field .cre "Mitigation" }}
        {{- $refs := field .cre "References" -}}
        {{- if $refs }}
        ### References
        {{ range $refs }}
        - {{ . }}
        {{ end }}
        {{- end }}
        +++ ## Events
        {{- range .hits }}
          ```
          {{ .Entry }}
          ```
        {{- end }}
        +++
*/

const (
	ActionTypeSlack  = "slack"
	ActionTypeJira   = "jira"
	ActionTypeLinear = "linear"
	ActionTypeExec   = "exec"
)

type Action interface {
	Execute(ctx context.Context, cre map[string]any) error
}

type configFile struct {
	Actions []actionConfig `yaml:"actions"`
}

type actionConfig struct {
	Type  string `yaml:"type"`
	Regex string `yaml:"regex,omitempty"`

	Slack *slackConfig `yaml:"slack,omitempty"`
	Jira  *jiraConfig  `yaml:"jira,omitempty"`
	Linear *linearConfig `yaml:"linear,omitempty"`
	Exec  *execConfig  `yaml:"exec,omitempty"`
}

func extractCreId(ev map[string]any) string {
	if cre, ok := ev["cre"]; ok {
		// map variant
		if m, ok := cre.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				return id
			}
			if id, ok := m["ID"].(string); ok {
				return id
			}
		}
		// struct variant
		v := reflect.ValueOf(cre)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.IsValid() && v.Kind() == reflect.Struct {
			f := v.FieldByName("ID")
			if f.IsValid() && f.Kind() == reflect.String {
				return f.String()
			}
		}
	}
	// fallback to top-level id
	if id, ok := ev["id"].(string); ok {
		return id
	}
	return ""
}

// ----- decorator that runs the action only when CRE ID matches ---------------
type filteredAction struct {
	pattern *regexp.Regexp
	inner   Action
}

func (f *filteredAction) Execute(ctx context.Context, ev map[string]any) error {
	if f.pattern == nil { // no filter → always run
		return f.inner.Execute(ctx, ev)
	}
	if id := extractCreId(ev); id != "" && f.pattern.MatchString(id) {
		return f.inner.Execute(ctx, ev) // match → run
	}
	return nil // no match → silently skip
}

func buildActions(cfgPath string) ([]Action, error) {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var file configFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, err
	}

	actions := make([]Action, 0, len(file.Actions))
	for i, c := range file.Actions {
		var a Action
		switch c.Type {
		case ActionTypeSlack:
			if c.Slack == nil {
				return nil, fmt.Errorf("missing slack section for action #%d", i)
			}
			a, err = newSlackAction(*c.Slack)
		case ActionTypeJira:
			if c.Jira == nil {
				return nil, fmt.Errorf("missing jira section for action #%d", i)
			}
			a, err = newJiraAction(*c.Jira)
		case ActionTypeExec:
			if c.Exec == nil {
				return nil, fmt.Errorf("missing exec section for action #%d", i)
			}
			a, err = newExecAction(*c.Exec)
		case ActionTypeLinear:
			if c.Linear == nil {
				return nil, fmt.Errorf("missing linear section for action #%d", i)
			}
			a, err = newLinearAction(*c.Linear)
		default:
			err = fmt.Errorf("unknown action type %q (index %d)", c.Type, i)
		}
		if err != nil {
			return nil, err
		}

		if c.Regex != "" {
			re, err := regexp.Compile(c.Regex)
			if err != nil {
				return nil, fmt.Errorf("invalid cre_id_regex for action #%d: %w", i, err)
			}
			a = &filteredAction{pattern: re, inner: a}
		}
		actions = append(actions, a)
	}
	return actions, nil
}

// template helper function to extract fields from CRE reports
func funcMap() template.FuncMap {
	return template.FuncMap{
		// field works with map[string]any OR struct / *struct
		"field": func(obj any, name string) any {
			if obj == nil {
				log.Error().Msg("field: obj is nil")
				return nil
			}
			// map
			if m, ok := obj.(map[string]any); ok {
				log.Info().Msgf("field: obj is map[string]any, name: %s", name)
				return m[name]
			}
			// struct via reflection
			v := reflect.ValueOf(obj)
			if v.Kind() == reflect.Pointer {
				log.Info().Msg("field: obj is pointer")
				v = v.Elem()
			}
			if v.IsValid() && v.Kind() == reflect.Struct {
				log.Info().Msgf("field: obj is struct, name: %s", name)
				f := v.FieldByName(name)
				if f.IsValid() {
					log.Info().Msgf("field: obj is struct, name: %s, value: %v", name, f.Interface())
					return f.Interface()
				}
			}
			log.Error().Msgf("field: unknown type: %T", obj)
			return nil // unknown
		},
		"stripdash": func(v any) string {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if strings.HasPrefix(s, "- ") {
					return strings.TrimPrefix(s, "- ")
				}
			}
			return fmt.Sprintf("%v", v)
		},
	}
}

func executeTemplate(out *string, tmpl *template.Template, data any) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	*out = buf.String()
	return nil
}

func Runbook(ctx context.Context, cfgPath string, report ux.ReportDocT) error {

	actions, err := buildActions(cfgPath)
	if err != nil {
		return err
	}

	for _, a := range actions {
		for _, cre := range report {
			if err := a.Execute(ctx, cre); err != nil {
				return err
			}
		}
	}

	return nil
}
