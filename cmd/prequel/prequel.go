package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prequel-dev/detection-engine/internal/pkg/auth"
	"github.com/prequel-dev/detection-engine/internal/pkg/config"
	"github.com/prequel-dev/detection-engine/internal/pkg/engine"
	"github.com/prequel-dev/detection-engine/internal/pkg/logs"
	"github.com/prequel-dev/detection-engine/internal/pkg/resolve"
	"github.com/prequel-dev/detection-engine/internal/pkg/rules"
	"github.com/prequel-dev/detection-engine/internal/pkg/sigs"
	"github.com/prequel-dev/detection-engine/internal/pkg/utils"
	"github.com/prequel-dev/detection-engine/internal/pkg/ux"
	"github.com/prequel-dev/detection-engine/pkg/datasrc"

	"github.com/Masterminds/semver"
	"github.com/alecthomas/kong"
	"github.com/posener/complete"
	"github.com/rs/zerolog/log"
	"github.com/willabides/kongplete"
)

const (
	tlsPort    = 8080
	udpPort    = 8081
	defStop    = "+inf"
	baseAddr   = "api-dev.prequel.dev"
	configFile = "config.yaml"
)

var (
	defaultConfigDir = filepath.Join(os.Getenv("HOME"), ".prequel")
	ruleToken        = filepath.Join(defaultConfigDir, ".ruletoken")
	ruleUpdateFile   = filepath.Join(defaultConfigDir, ".ruleupdate")
)

var cli struct {
	Disabled      bool   `short:"d" help:"Do not run community CREs"`
	Stop          string `short:"e" help:"Stop time"`
	JsonLogs      bool   `short:"j" help:"Print logs in JSON format to stderr" default:"false"`
	Level         string `short:"l" help:"Print logs at this level to stderr"`
	ReportFile    string `short:"n" help:"Report filename"`
	NoReport      bool   `short:"N" help:"Do not write a report"`
	Quiet         bool   `short:"q" help:"Quiet mode, do not print progress"`
	Rules         string `short:"r" help:"Path to a CRE file"`
	Source        string `short:"s" help:"Path to a data source file"`
	Format        string `short:"t" help:"Format to use for timestamps"`
	Version       bool   `short:"v" help:"Print version and exit"`
	Window        string `short:"w" help:"Reorder lookback window duration"`
	Regex         string `short:"x" help:"Regex to match for extracting timestamps"`
	AcceptUpdates bool   `short:"y" help:"Accept updates to rules or new release"`
}

func tsOpts(c *config.Config) []resolve.OptT {
	opts := c.ResolveOpts()
	if cli.Regex != "" || cli.Format != "" {
		opts = append(opts, resolve.WithCustomFmt(cli.Regex, cli.Format))
	}
	if cli.Window != "" {
		window, err := time.ParseDuration(cli.Window)
		if err != nil || window < 0 {
			log.Error().Err(err).Msg("Failed to parse window duration")
			ux.ConfigError(err)
			os.Exit(1)
		}
		opts = append(opts, resolve.WithWindow(int64(window)))
	}
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

func main() {

	var (
		ctx    = sigs.InitSignals()
		parser = kong.Must(
			&cli,
			kong.Name(ux.AppName),
			kong.Description(ux.AppDesc),
			kong.UsageOnError(),
		)
		c          *config.Config
		stop       int64
		token      string
		rulesPaths []string
		err        error
	)

	// Run kongplete.Complete to handle completion requests
	kongplete.Complete(parser,
		kongplete.WithPredictor("file", complete.PredictFiles("*")),
	)

	kong.Parse(&cli)

	switch {
	case cli.Version:

		var (
			currRulesVer  *semver.Version
			currRulesPath string
		)

		if currRulesVer, currRulesPath, err = rules.GetCurrentRulesVersion(defaultConfigDir); err != nil {
			log.Error().Err(err).Msg("Failed to get current rules version")
		}

		ux.PrintVersion(defaultConfigDir, currRulesPath, currRulesVer)
		os.Exit(0)
	}

	logs.InitLogger(
		logs.WithLevel(cli.Level),
		logs.WithPretty(),
	)

	if c, err = config.LoadConfig(defaultConfigDir, configFile); err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		ux.ConfigError(err)
		os.Exit(1)
	}

	// Log in for community rule updates
	if token, err = auth.Login(ctx, ruleToken); err != nil {
		log.Error().Err(err).Msg("Failed to login")

		// A notice will be printed if the email is not verified
		if err != auth.ErrEmailNotVerified {
			ux.AuthError(err)
		}

		os.Exit(1)
	}

	if cli.AcceptUpdates {
		c.AcceptUpdates = true
	}

	if cli.Disabled {
		c.Rules.Disabled = true
	}

	rulesPaths, err = rules.GetRules(ctx, c, defaultConfigDir, cli.Rules, token, ruleUpdateFile, baseAddr, tlsPort, udpPort)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get rules")
		ux.RulesError(err)
		os.Exit(1)
	}

	var (
		topts    = tsOpts(c)
		sources  []*engine.LogData
		useStdin = len(cli.Source) == 0 && c.DataSources == ""
	)

	if useStdin {
		sources, err = resolve.PipeStdin(topts...)
		if err != nil {
			log.Error().Err(err).Msg("Failed to read stdin")
			ux.DataError(err)
			os.Exit(1)
		}
	} else {
		var source = c.DataSources
		// CLI overrides source config
		if cli.Source != "" {
			source = cli.Source
		}
		sources, err = parseSources(source, topts...)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse data sources")
			ux.DataError(err)
			os.Exit(1)
		}
	}

	if len(sources) == 0 {
		log.Error().Msg("No data sources found")
		ux.DataError(fmt.Errorf("no data sources found"))
		os.Exit(1)
	}

	// Get stop time
	if stop, err = utils.ParseTime(cli.Stop, defStop); err != nil {
		log.Error().Err(err).Msg("Failed to parse stop time")
		ux.ConfigError(err)
		os.Exit(1)
	}

	pw := ux.RootProgress(!useStdin)

	var (
		renderExit   = make(chan struct{})
		reportPath   string
		ruleMatchers *engine.RuleMatchersT
	)

	if !cli.Quiet {
		go func() {
			pw.Render()
			renderExit <- struct{}{}
		}()
	}

	r := engine.New(stop, ux.NewUxCmd(pw))
	defer r.Close()

	report := ux.NewReport(pw)

	if ruleMatchers, err = r.LoadRulesPaths(report, rulesPaths); err != nil {
		log.Error().Err(err).Msg("Failed to load rules")
		ux.RulesError(err)
		os.Exit(1)
	}

	if err = r.Run(ctx, ruleMatchers, sources, report); err != nil {
		log.Error().Err(err).Msg("Failed to run runtime")
		ux.RulesError(err)
		os.Exit(1)
	}

	if err = report.DisplayCREs(); err != nil {
		log.Error().Err(err).Msg("Failed to display CREs")
		ux.RulesError(err)
		os.Exit(1)
	}

	if !cli.NoReport {
		if reportPath, err = report.Write(cli.ReportFile); err != nil {
			log.Error().Err(err).Msg("Failed to write full report")
			ux.RulesError(err)
			os.Exit(1)
		}
	}

	pw.Stop()

LOOP:
	for {

		if cli.Quiet {
			break LOOP
		}

		select {
		case <-ctx.Done():
			break LOOP
		case <-renderExit:
			break LOOP
		}
	}

	if reportPath != "" {
		fmt.Fprintf(os.Stdout, "\nWrote report to %s\n", reportPath)
	}
}
