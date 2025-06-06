package test

import (
        "context"
        "reflect"
        "strings"
        "testing"

        krewpkg "github.com/prequel-dev/preq/cmd/plugin/krew"
        "github.com/prequel-dev/preq/internal/pkg/cli"
        "github.com/google/go-cmp/cmp"
        "github.com/spf13/pflag"
        "k8s.io/cli-runtime/pkg/genericclioptions"
)

func flagNameFromField(name string) string {
	var out []rune
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out = append(out, '-')
			}
			out = append(out, r+'a'-'A')
		} else {
			out = append(out, r)
		}
	}
	return strings.ToLower(string(out))
}

func cliFlagSet() map[string]struct{} {
	flags := map[string]struct{}{}
	t := reflect.TypeOf(cli.Options)
	for i := 0; i < t.NumField(); i++ {
		name := flagNameFromField(t.Field(i).Name)
		flags[name] = struct{}{}
	}
	return flags
}

func krewFlagSet() map[string]struct{} {
	cmd := krewpkg.RootCmd(context.Background(), krewpkg.NewRunOptions(genericclioptions.IOStreams{}))
	flags := map[string]struct{}{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags[f.Name] = struct{}{}
	})
	return flags
}

func TestKrewAndCLIMatch(t *testing.T) {
        cliFlags := cliFlagSet()
        krewFlags := krewFlagSet()

        if diff := cmp.Diff(cliFlags, krewFlags); diff != "" {
                t.Fatalf("flags mismatch (-cli +krew):\n%s", diff)
        }
}

func TestCoverageOutput(t *testing.T) {
	t.Logf("coverage: %.2f%%", testing.Coverage()*100)
}
