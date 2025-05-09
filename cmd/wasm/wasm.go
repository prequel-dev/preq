//go:build wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
	"syscall/js"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/config"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/preq/internal/pkg/verz"
	"github.com/prequel-dev/preq/pkg/eval"
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
			cfg, inputData, ruleData string
			reportDoc                ux.ReportDocT
			stats                    ux.StatsT
			err                      error
		)

		log.Info().
			Str("version", verz.Semver()).
			Str("hash", verz.Githash).
			Str("date", verz.Date).
			Msg("Wasm preq engine version")

		inputData = args[0].String()
		ruleData = args[1].String()

		// Permit events to arrive out of order within a 1 hour window by default
		cfg = config.Marshal(config.WithWindow(time.Hour))

		if reportDoc, stats, err = eval.Detect(ctx, cfg, inputData, ruleData); err != nil {
			return errJson(err)
		}

		return respJson(reportDoc, stats)
	})

	return detectFunc
}

func main() {

	ctx := context.Background()

	js.Global().Set("detect", detectWrapper(ctx))

	select {}
}
