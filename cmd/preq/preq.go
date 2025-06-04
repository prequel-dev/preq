package main

import (
	"os"

	"github.com/prequel-dev/preq/internal/pkg/cli"
	"github.com/prequel-dev/preq/internal/pkg/logs"
	"github.com/prequel-dev/preq/internal/pkg/sigs"
	"github.com/prequel-dev/preq/internal/pkg/ux"

	"github.com/alecthomas/kong"
	"github.com/posener/complete"
	"github.com/willabides/kongplete"
)

var vars = kong.Vars{
	"actionHelp":        ux.HelpAction,
	"disabledHelp":      ux.HelpDisabled,
	"generateHelp":      ux.HelpGenerate,
	"cronHelp":          ux.HelpCron,
	"levelHelp":         ux.HelpLevel,
	"nameHelp":          ux.HelpName,
	"quietHelp":         ux.HelpQuiet,
	"rulesHelp":         ux.HelpRules,
	"sourceHelp":        ux.HelpSource,
	"versionHelp":       ux.HelpVersion,
	"acceptUpdatesHelp": ux.HelpAcceptUpdates,
}

func main() {

	var (
		ctx    = sigs.InitSignals()
		parser = kong.Must(
			&cli.Options,
			kong.Name(ux.ProcessName()),
			kong.Description(ux.AppDesc),
			kong.UsageOnError(),
			kong.Vars(vars),
		)
		err error
	)

	// Run kongplete.Complete to handle completion requests
	kongplete.Complete(parser,
		kongplete.WithPredictor("file", complete.PredictFiles("*")),
	)

	kong.Parse(&cli.Options, vars)

	logOpts := []logs.InitOpt{
		logs.WithLevel(cli.Options.Level),
		logs.WithPretty(),
	}

	// Initialize logger first before any other logging
	logs.InitLogger(logOpts...)

	if err = cli.InitAndExecute(ctx); err != nil {
		os.Exit(1)
	}
}
