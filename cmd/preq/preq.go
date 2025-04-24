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

func main() {

	var (
		ctx    = sigs.InitSignals()
		parser = kong.Must(
			&cli.Options,
			kong.Name(ux.ProcessName()),
			kong.Description(ux.AppDesc),
			kong.UsageOnError(),
		)
		err error
	)

	// Run kongplete.Complete to handle completion requests
	kongplete.Complete(parser,
		kongplete.WithPredictor("file", complete.PredictFiles("*")),
	)

	kong.Parse(&cli.Options)

	logOpts := []logs.InitOpt{
		logs.WithLevel(cli.Options.Level),
	}

	if !cli.Options.JsonLogs {
		logOpts = append(logOpts, logs.WithPretty())
	}

	// Initialize logger first before any other logging
	logs.InitLogger(logOpts...)

	if err = cli.InitAndExecute(ctx); err != nil {
		os.Exit(1)
	}
}
