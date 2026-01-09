package resolve

import (
	"bytes"

	"github.com/prequel-dev/prequel-logmatch/pkg/format"
	"github.com/prequel-dev/prequel-logmatch/pkg/timez"
	"github.com/rs/zerolog/log"
)

const (
	detectSampleSize = 16 * 1024
)

type TimestampFmt = timez.TimestampFmt

type FmtSpec struct {
	Pattern string
	Format  TimestampFmt
}

type OptT func(*optsT)

func WithCustomFmt(regex, fmt string) func(*optsT) {
	return func(o *optsT) {
		o.customFmt = fmt
		o.customRegex = regex
	}
}

func WithStampRegex(stampRegex ...FmtSpec) func(*optsT) {
	return func(o *optsT) {
		o.stampRegex = stampRegex
	}
}

func WithWindow(window int64) func(*optsT) {
	return func(o *optsT) {
		o.window = window
	}
}

func WithTimestampTries(tries int) func(*optsT) {
	return func(o *optsT) {
		o.timestampTries = tries
	}
}

func (o *optsT) tryCustom() bool {
	return o.customFmt != "" || o.customRegex != ""
}

func NewLogFactory(data []byte, opts ...OptT) (format.FactoryI, int64, error) {
	o := parseOpts(opts...)

	var (
		err      error
		stamp    int64
		maxTries = o.timestampTries
		factory  format.FactoryI
	)

	log.Debug().Int("maxTries", maxTries).Msg("Trying custom timestamp format")

	if o.tryCustom() {
		return timez.TryTimestampFormat(o.customRegex, timez.TimestampFmt(o.customFmt), data, maxTries)
	}

	// Detect format
	if factory, stamp, err = format.Detect(bytes.NewReader(data)); err == nil {
		return factory, stamp, nil
	}

	// Failed to detect format, try timestamp regexes if any
	for _, spec := range o.stampRegex {
		if factory, stamp, err = timez.TryTimestampFormat(spec.Pattern, spec.Format, data, maxTries); err == nil {
			break
		}
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to detect timestamp format")
		return nil, 0, err
	}

	return factory, stamp, nil
}

type optsT struct {
	customFmt      string
	customRegex    string
	stampRegex     []FmtSpec
	window         int64
	timestampTries int
}

func parseOpts(opts ...OptT) *optsT {
	o := &optsT{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
