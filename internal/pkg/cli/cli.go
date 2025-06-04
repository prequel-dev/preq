package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/prequel-dev/preq/internal/pkg/auth"
	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/engine"
	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/prequel-dev/preq/internal/pkg/rules"
	"github.com/prequel-dev/preq/internal/pkg/runbook"
	"github.com/prequel-dev/preq/internal/pkg/timez"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/prequel-compiler/pkg/datasrc"
	"github.com/rs/zerolog/log"
)

var Options struct {
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
}

var (
	// https://specifications.freedesktop.org/basedir-spec/latest/
	defaultConfigDir = filepath.Join(os.Getenv("HOME"), ".config", "preq")
	ruleToken        = filepath.Join(defaultConfigDir, ".ruletoken")
	ruleUpdateFile   = filepath.Join(defaultConfigDir, ".ruleupdate")
)

const (
	tlsPort    = 443
	udpPort    = 8081
	defStop    = "+inf"
	baseAddr   = "app-beta.prequel.dev"
	configFile = "config.yaml"
)

func tsOpts(c *config.Config) []resolve.OptT {
	opts := c.ResolveOpts()
	if c.Window > 0 {
		opts = append(opts, resolve.WithWindow(int64(c.Window)))
	}
	opts = append(opts, resolve.WithTimestampTries(c.Skip))
	return opts
}

func parseSources(fn string, opts ...resolve.OptT) ([]*resolve.LogData, error) {

	ds, err := datasrc.ParseFile(fn)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse data sources file")
		return nil, err
	}

	if err := datasrc.Validate(ds); err != nil {
		log.Error().Err(err).Msg("Failed to validate data sources")
		return nil, err
	}

	return resolve.Resolve(ds, opts...), nil
}

func InitAndExecute(ctx context.Context) error {
	var (
		c          *config.Config
		token      string
		rulesPaths []utils.RulePathT
		err        error
	)

	switch {
	case Options.Version:

		var (
			currRulesVer  *semver.Version
			currRulesPath string
		)

		if currRulesVer, currRulesPath, err = rules.GetCurrentRulesVersion(defaultConfigDir); err != nil {
			log.Error().Err(err).Msg("Failed to get current rules version")
		}

		ux.PrintVersion(defaultConfigDir, currRulesPath, currRulesVer)
		return nil
	}

	if c, err = config.LoadConfig(defaultConfigDir, configFile); err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		ux.ConfigError(err)
		return err
	}

	// Log in for community rule updates
	if token, err = auth.Login(ctx, baseAddr, ruleToken); err != nil {
		log.Error().Err(err).Msg("Failed to login")

		// A notice will be printed if the email is not verified
		if err != auth.ErrEmailNotVerified {
			ux.AuthError(err)
		}

		return err
	}

	if Options.AcceptUpdates {
		c.AcceptUpdates = true
	}

	if Options.Disabled {
		c.Rules.Disabled = true
	}

	if c.Skip == 0 {
		c.Skip = timez.DefaultSkip
	}

	rulesPaths, err = rules.GetRules(ctx, c, defaultConfigDir, Options.Rules, token, ruleUpdateFile, baseAddr, tlsPort, udpPort)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get rules")
		ux.RulesError(err)
		return err
	}

	var (
		topts    = tsOpts(c)
		sources  []*engine.LogData
		useStdin = len(Options.Source) == 0 && c.DataSources == ""
	)

	if useStdin {
		sources, err = resolve.PipeStdin(topts...)
		if err != nil {
			log.Error().Err(err).Msg("Failed to read stdin")
			ux.DataError(err)
			return err
		}
	} else {
		var source = c.DataSources
		// CLI overrides source config
		if Options.Source != "" {
			source = Options.Source
		}
		sources, err = parseSources(source, topts...)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse data sources")
			ux.DataError(err)
			return err
		}
	}

	var (
		pw           = ux.RootProgress(!useStdin)
		renderExit   = make(chan struct{})
		r            = engine.New(utils.GetStopTime(), ux.NewUxCmd(pw))
		report       = ux.NewReport(pw)
		reportPath   string
		ruleMatchers *engine.RuleMatchersT
	)

	defer r.Close()

	if ruleMatchers, err = r.LoadRulesPaths(report, rulesPaths); err != nil {
		log.Error().Err(err).Msg("Failed to load rules")
		ux.RulesError(err)
		return err
	}

	if Options.Cron {
		if err := ux.PrintCronJobTemplate(Options.Name, defaultConfigDir, rulesPaths[0].Path); err != nil {
			log.Error().Err(err).Msg("Failed to write cronjob template")
			ux.ConfigError(err)
			return err
		}
		return nil
	}

	if Options.Generate {

		var (
			currRulesVer *semver.Version
			template     []byte
			fn           string
		)

		if currRulesVer, _, err = rules.GetCurrentRulesVersion(defaultConfigDir); err != nil {
			log.Error().Err(err).Msg("Failed to get current rules version")
		}

		if template, err = ruleMatchers.DataSourceTemplate(currRulesVer); err != nil {
			log.Error().Err(err).Msg("Failed to generate data source template")
			ux.RulesError(err)
			return err
		}

		if fn, err = ux.WriteDataSourceTemplate(Options.Name, currRulesVer, template); err != nil {
			log.Error().Err(err).Msg("Failed to write data source template")
			ux.DataError(err)
			return err
		}

		if fn != "" {
			fmt.Fprintf(os.Stdout, "Wrote data source template to %s\n", fn)
		}

		return nil
	}

	if len(sources) == 0 {
		ux.PrintUsage()
		return nil
	}

	if !Options.Quiet {
		go func() {
			pw.Render()
			renderExit <- struct{}{}
		}()
	}

	if err = r.Run(ctx, ruleMatchers, sources, report); err != nil {
		log.Error().Err(err).Msg("Failed to run runtime")
		ux.RulesError(err)
		return err
	}

	if err = report.DisplayCREs(); err != nil {
		log.Error().Err(err).Msg("Failed to display CREs")
		ux.RulesError(err)
		return err
	}

	pw.Stop()

LOOP:
	for {

		if Options.Quiet {
			break LOOP
		}

		select {
		case <-ctx.Done():
			break LOOP
		case <-renderExit:
			break LOOP
		}
	}

	switch {
	case report.Size() == 0:
		log.Debug().Msg("No CREs found")
		return nil

	case Options.Action != "":
		log.Debug().Msgf("Running action %s", Options.Action)

		report, err := report.CreateReport()
		if err != nil {
			log.Error().Err(err).Msg("Failed to create report")
			ux.RulesError(err)
			return err
		}

		if err := runbook.Runbook(ctx, Options.Action, report); err != nil {
			log.Error().Err(err).Msg("Failed to run action")
			ux.RulesError(err)
			return err
		}

	case c.Notification.Type == ux.NotificationSlack:

		log.Debug().Msgf("Posting Slack notification to %s", c.Notification.Webhook)

		if err = report.PostSlackDetection(ctx, c.Notification.Webhook, Options.Name); err != nil {
			log.Error().Err(err).Msg("Failed to post Slack notification")
			ux.RulesError(err)
			return err
		}

		if !Options.Quiet {

			// Print reports to stdout when notifications are enabled
			if err = report.PrintReport(); err != nil {
				log.Error().Err(err).Msg("Failed to print report")
				ux.RulesError(err)
				return err
			}

			fmt.Fprintf(os.Stdout, "\nSent Slack notification\n")
		}

	case Options.Name == ux.OutputStdout:
		if err = report.PrintReport(); err != nil {
			log.Error().Err(err).Msg("Failed to print report")
			ux.RulesError(err)
			return err
		}

	default:
		if reportPath, err = report.Write(Options.Name); err != nil {
			log.Error().Err(err).Msg("Failed to write full report")
			ux.RulesError(err)
			return err
		}

		if !Options.Quiet {
			fmt.Fprintf(os.Stdout, "\nWrote report to %s\n", reportPath)
		}
	}

	return nil
}
