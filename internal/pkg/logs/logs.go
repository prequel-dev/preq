package logs

import (
	"encoding/json"
	"fmt"
	slog "log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Shorten caller to only filename; from Zerolog docs.
func shortenCaller(pc uintptr, file string, line int) string {
	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}
	file = short
	return file + ":" + strconv.Itoa(line)
}

const (
	colorWhite = 37
)

func colorize(s string, color int) string {
	return fmt.Sprintf("\033[%dm%s\033[0m", color, s)
}

// From zerolog.consoleDefaultFormatTimestamp;
func mkTimestampFormatter(timeFormat string, color int) zerolog.Formatter {

	return func(i interface{}) string {
		t := "<nil>"
		switch tt := i.(type) {
		case string:
			ts, err := time.ParseInLocation(zerolog.TimeFieldFormat, tt, time.Local)
			if err != nil {
				t = tt
			} else {
				t = ts.Local().Format(timeFormat)
			}
		case json.Number:
			i, err := tt.Int64()
			if err != nil {
				t = tt.String()
			} else {
				var sec, nsec int64

				switch zerolog.TimeFieldFormat {
				case zerolog.TimeFormatUnixNano:
					sec, nsec = 0, i
				case zerolog.TimeFormatUnixMicro:
					sec, nsec = 0, int64(time.Duration(i)*time.Microsecond)
				case zerolog.TimeFormatUnixMs:
					sec, nsec = 0, int64(time.Duration(i)*time.Millisecond)
				default:
					sec, nsec = i, 0
				}

				ts := time.Unix(sec, nsec)
				t = ts.Format(timeFormat)
			}
		}

		return colorize(t, color)
	}
}

type stubLogWriter struct {
	lvl zerolog.Level
	log zerolog.Logger
}

func (s *stubLogWriter) Write(p []byte) (n int, err error) {
	// The logging library insists on a LF here
	msg := strings.TrimRight(string(p), "\n")
	s.log.WithLevel(s.lvl).Msg(msg)
	return len(p), nil
}

type Opts struct {
	Level  string
	Pretty bool
}

func WithLevel(level string) InitOpt {
	return func(o *Opts) {
		o.Level = level
	}
}

func WithPretty() InitOpt {
	return func(o *Opts) {
		o.Pretty = true
	}
}

type InitOpt func(*Opts)

func InitLogger(opts ...InitOpt) {

	var (
		o = &Opts{}
	)

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	zerolog.CallerMarshalFunc = shortenCaller

	for _, opt := range opts {
		opt(o)
	}

	zlvl, _ := zerolog.ParseLevel(o.Level)
	zerolog.SetGlobalLevel(zlvl)

	nlog := log.Logger

	if o.Pretty {
		// Normally we use the default FormatTimestamp functionality in zerolog;
		// However the default hard-coded colors are not suitable for the color blind.
		output := zerolog.ConsoleWriter{
			Out:             os.Stderr,
			FormatTimestamp: mkTimestampFormatter(time.StampMicro, colorWhite),
		}

		nlog = log.Output(output)
	}

	// Turn on caller. This has a runtime penalty; possibly turn off in production.
	nlog = nlog.With().Caller().Logger()

	log.Logger = nlog

	// Install our stub writer on default standard logger
	slog.SetOutput(&stubLogWriter{zerolog.InfoLevel, nlog})
}
