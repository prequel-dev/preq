//go:build wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
	"syscall/js"

	"github.com/prequel-dev/detection-engine/internal/pkg/config"
	"github.com/prequel-dev/detection-engine/internal/pkg/engine"
	"github.com/prequel-dev/detection-engine/internal/pkg/resolve"
	"github.com/prequel-dev/detection-engine/internal/pkg/utils"
	"github.com/prequel-dev/detection-engine/internal/pkg/ux"
	"github.com/prequel-dev/detection-engine/internal/pkg/verz"
	"github.com/rs/zerolog/log"
)

var (
	ErrInvalidArgs = errors.New("invalid number of arguments passed")
)

const (
	expectedArgs = 3
)

type ResultT struct {
	Success bool   `json:"success"`
	Result  any    `json:"result"`
	Stats   any    `json:"stats"`
	Error   string `json:"error"`
}

func respJson(r any, stats any) string {
	var (
		res ResultT
		out []byte
		err error
	)

	res.Success = true
	res.Result = r
	res.Stats = stats
	if out, err = json.Marshal(res); err != nil {
		return `{"success": false, "error": "` + err.Error() + `"}`
	}
	return string(out)
}

func errJson(e error) string {
	var (
		res ResultT
		out []byte
		err error
	)

	res.Success = false
	res.Error = e.Error()
	if out, err = json.Marshal(res); err != nil {
		return `{"success": false, "error": "` + err.Error() + `"}`
	}
	return string(out)
}

func detectWrapper(ctx context.Context) js.Func {
	detectFunc := js.FuncOf(func(this js.Value, args []js.Value) any {

		var (
			c                        *config.Config
			cfg, inputData, ruleData string
			stop                     int64
			run                      *engine.RuntimeT
			report                   *ux.ReportT
			ruleMatchers             *engine.RuleMatchersT
			sources                  []*engine.LogData
			reportData               any
			err                      error
		)

		log.Info().
			Str("version", verz.Semver()).
			Str("hash", verz.Githash).
			Str("date", verz.Date).
			Msg("Wasm prequel engine version")

		inputData = args[0].String()
		ruleData = args[1].String()

		if len(cfg) == 0 {
			log.Warn().Msg("No config provided, using default")
			cfg = config.DefaultConfig
		}

		if c, err = config.LoadConfigFromBytes(cfg); err != nil {
			log.Error().Err(err).Msg("Failed to load config")
			return errJson(err)
		}

		if stop, err = utils.ParseTime("", "∞"); err != nil {
			log.Error().Err(err).Msg("Failed to parse stop time")
			return errJson(err)
		}

		opts := c.ResolveOpts()
		if sources, err = resolve.PipeWasm([]byte(inputData), opts...); err != nil {
			log.Error().Err(err).Msg("Failed to create pipe reader")
			return errJson(err)
		}

		run = engine.New(stop, ux.NewUxWasm())
		defer run.Close()

		report = ux.NewReport(nil)

		if ruleMatchers, err = run.CompileRules([]byte(ruleData), report); err != nil {
			log.Error().Err(err).Msg("Failed to compile rules")
			return errJson(err)
		}

		if err = run.Run(ctx, ruleMatchers, sources, report); err != nil {
			log.Error().Err(err).Msg("Failed to run stdin")
			return errJson(err)
		}

		if reportData, err = report.CreateReport(); err != nil {
			log.Error().Err(err).Msg("Failed to create report")
			return errJson(err)
		}

		stats, err := run.Ux.FinalStats()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get final stats, continue...")
		}

		return respJson(reportData, stats)
	})

	return detectFunc
}

func prettyJson(input string) (string, error) {
	var raw any
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(pretty), nil
}

func main() {

	ctx := context.Background()

	js.Global().Set("detect", detectWrapper(ctx))

	select {}
}
