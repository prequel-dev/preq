package eval

import (
	"context"

	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/engine"
	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/prequel-dev/preq/internal/pkg/timez"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/rs/zerolog/log"
)

func Detect(ctx context.Context, cfg, data, rule string) (ux.ReportDocT, ux.StatsT, error) {

	var (
		c            *config.Config
		run          *engine.RuntimeT
		report       *ux.ReportT
		ruleMatchers *engine.RuleMatchersT
		sources      []*engine.LogData
		reportData   ux.ReportDocT
		stats        ux.StatsT
		err          error
	)

	if len(cfg) == 0 {
		log.Warn().Msg("No config provided, using default")
		cfg = config.DefaultConfig
	}

	if c, err = config.LoadConfigFromBytes(cfg); err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		return nil, nil, err
	}

	opts := c.ResolveOpts()
	opts = append(opts, resolve.WithTimestampTries(timez.DefaultSkip))

	if sources, err = resolve.PipeEval([]byte(data), opts...); err != nil {
		log.Error().Err(err).Msg("Failed to create pipe reader")
		return nil, nil, err
	}

	run = engine.New(utils.GetStopTime(), ux.NewUxEval())
	defer run.Close()

	report = ux.NewReport(nil)

	if ruleMatchers, err = run.CompileRules([]byte(rule), report); err != nil {
		log.Error().Err(err).Msg("Failed to compile rules")
		return nil, nil, err
	}

	if err = run.Run(ctx, ruleMatchers, sources, report); err != nil {
		log.Error().Err(err).Msg("Failed to run stdin")
		return nil, nil, err
	}

	if reportData, err = report.CreateReport(); err != nil {
		log.Error().Err(err).Msg("Failed to create report")
		return nil, nil, err
	}

	stats, err = run.Ux.FinalStats()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get final stats, continue...")
	}

	return reportData, stats, nil

}
