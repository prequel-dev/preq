package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/auth"
	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/engine"
	"github.com/prequel-dev/preq/internal/pkg/logs"
	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/prequel-dev/preq/internal/pkg/rules"
	"github.com/prequel-dev/preq/internal/pkg/sigs"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/prequel-compiler/pkg/datasrc"

	"github.com/Masterminds/semver"
	"github.com/alecthomas/kong"
	"github.com/posener/complete"
	"github.com/rs/zerolog/log"
	"github.com/willabides/kongplete"
)

const (
	tlsPort      = 8080
	udpPort      = 8081
	defStop      = "+inf"
	baseAddr     = "api-beta.prequel.dev"
	configFile   = "config.yaml"
	stdoutReport = "-"
)

var (
	defaultConfigDir = filepath.Join(os.Getenv("HOME"), ".preq")
	ruleToken        = filepath.Join(defaultConfigDir, ".ruletoken")
	ruleUpdateFile   = filepath.Join(defaultConfigDir, ".ruleupdate")
)

var cli struct {
	Disabled      bool   `short:"d" help:"Do not run community CREs"`
	Stop          string `short:"e" help:"Stop time"`
	Generate      bool   `short:"g" help:"Generate data sources template"`
	JsonLogs      bool   `short:"j" help:"Print logs in JSON format to stderr" default:"false"`
	Skip          int    `short:"k" help:"Skip the first N lines for timestamp detection" default:"50"`
	Level         string `short:"l" help:"Print logs at this level to stderr"`
	Filename      string `short:"n" help:"Report or data source template output file name"`
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
	opts = append(opts, resolve.WithTimestampTries(cli.Skip))
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
			kong.Name(ux.ProcessName()),
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

	logOpts := []logs.InitOpt{
		logs.WithLevel(cli.Level),
	}

	if !cli.JsonLogs {
		logOpts = append(logOpts, logs.WithPretty())
	}

	// Initialize logger first before any other logging
	logs.InitLogger(logOpts...)

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

	if c, err = config.LoadConfig(defaultConfigDir, configFile); err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		ux.ConfigError(err)
		os.Exit(1)
	}

	// Log in for community rule updates
	if token, err = auth.Login(ctx, baseAddr, ruleToken); err != nil {
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

	// Get stop time
	if stop, err = utils.ParseTime(cli.Stop, defStop); err != nil {
		log.Error().Err(err).Msg("Failed to parse stop time")
		ux.ConfigError(err)
		os.Exit(1)
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
		os.Exit(1)
	}

	if cli.Generate {

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
			os.Exit(1)
		}

		if fn, err = ux.WriteDataSourceTemplate(cli.Filename, currRulesVer, template); err != nil {
			log.Error().Err(err).Msg("Failed to write data source template")
			ux.DataError(err)
			os.Exit(1)
		}

		if fn != "" {
			fmt.Fprintf(os.Stdout, "Wrote data source template to %s\n", fn)
		}

		os.Exit(0)
	}

	if len(sources) == 0 {
		ux.PrintUsage()
		os.Exit(1)
	}

	if !cli.Quiet {
		go func() {
			pw.Render()
			renderExit <- struct{}{}
		}()
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

	switch {
	case cli.Filename == stdoutReport:
		if err = report.PrintReport(); err != nil {
			log.Error().Err(err).Msg("Failed to print report")
			ux.RulesError(err)
			os.Exit(1)
		}
	case cli.Filename != "":
		if reportPath, err = report.Write(cli.Filename); err != nil {
			log.Error().Err(err).Msg("Failed to write full report")
			ux.RulesError(err)
			os.Exit(1)
		}

		if !cli.Quiet {
			fmt.Fprintf(os.Stdout, "\nWrote report to %s\n", reportPath)
		}
	}
}
