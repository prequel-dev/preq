package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver"
	"github.com/prequel-dev/preq/internal/pkg/auth"
	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/engine"
	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/prequel-dev/preq/internal/pkg/rules"
	"github.com/prequel-dev/preq/internal/pkg/timez"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/prequel-compiler/pkg/datasrc"
	"github.com/rs/zerolog/log"
)

var Options struct {
	SlackNotification bool   `short:"a" help:"Send a Slack notification to the configured webhook when one or more CRE is detected"`
	Cron              bool   `short:"c" help:"Generate Kubernetes cronjob template"`
	Disabled          bool   `short:"d" help:"Do not run community CREs"`
	Stop              string `short:"e" help:"Stop time"`
	Generate          bool   `short:"g" help:"Generate data sources template"`
	JsonLogs          bool   `short:"j" help:"Print logs in JSON format to stderr" default:"false"`
	Skip              int    `short:"k" help:"Skip the first N lines for timestamp detection"`
	Level             string `short:"l" help:"Print logs at this level to stderr"`
	Name              string `short:"o" help:"Report output name, generated data source template name, or notification context name"`
	Quiet             bool   `short:"q" help:"Quiet mode, do not print progress"`
	Rules             string `short:"r" help:"Path to a CRE rules file"`
	Source            string `short:"s" help:"Path to a data source Yaml file"`
	Format            string `short:"t" help:"Format to use for timestamps"`
	Version           bool   `short:"v" help:"Print version and exit"`
	Window            string `short:"w" help:"Reorder lookback window duration"`
	Regex             string `short:"x" help:"Regex to match for extracting timestamps"`
	AcceptUpdates     bool   `short:"y" help:"Accept updates to rules or new release"`
}

var (
	defaultConfigDir = filepath.Join(os.Getenv("HOME"), ".preq")
	ruleToken        = filepath.Join(defaultConfigDir, ".ruletoken")
	ruleUpdateFile   = filepath.Join(defaultConfigDir, ".ruleupdate")
)

const (
	tlsPort    = 8080
	udpPort    = 8081
	defStop    = "+inf"
	baseAddr   = "api-beta.prequel.dev"
	configFile = "config.yaml"
)

func tsOpts(c *config.Config) []resolve.OptT {
	opts := c.ResolveOpts()
	if Options.Regex != "" || Options.Format != "" {
		opts = append(opts, resolve.WithCustomFmt(Options.Regex, Options.Format))
	}
	if Options.Window != "" {
		window, err := time.ParseDuration(Options.Window)
		if err != nil || window < 0 {
			log.Error().Err(err).Msg("Failed to parse window duration")
			ux.ConfigError(err)
			os.Exit(1)
		}
		opts = append(opts, resolve.WithWindow(int64(window)))
	}
	opts = append(opts, resolve.WithTimestampTries(Options.Skip))
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
		stop       int64
		token      string
		rulesPaths []string
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

	if Options.Skip == 0 {
		Options.Skip = timez.DefaultSkip
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

	// Get stop time
	if stop, err = utils.ParseTime(Options.Stop, defStop); err != nil {
		log.Error().Err(err).Msg("Failed to parse stop time")
		ux.ConfigError(err)
		return err
	}

	var (
		pw           = ux.RootProgress(!useStdin)
		renderExit   = make(chan struct{})
		r            = engine.New(stop, ux.NewUxCmd(pw))
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
		if err := ux.PrintCronJobTemplate(Options.Name, defaultConfigDir, rulesPaths[0]); err != nil {
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

	case Options.SlackNotification && c.Notification.Type == ux.NotificationSlack:

		log.Debug().Msgf("Posting Slack notification to %s", c.Notification.Webhook)

		if err = report.PostSlackDetection(ctx, c.Notification.Webhook, Options.Name); err != nil {
			log.Error().Err(err).Msg("Failed to post Slack notification")
			ux.RulesError(err)
			return err
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
