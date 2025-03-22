package resolve

import (
	"bytes"

	"github.com/prequel-dev/detection-engine/internal/pkg/timez"
	"github.com/prequel-dev/prequel-logmatch/pkg/format"
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

func (o *optsT) tryCustom() bool {
	return o.customFmt != "" || o.customRegex != ""
}

func NewLogFactory(data []byte, opts ...OptT) (format.FactoryI, int64, error) {
	o := parseOpts(opts...)

	var (
		err     error
		stamp   int64
		factory format.FactoryI
	)

	if o.tryCustom() {
		return timez.TryTimestampFormat(o.customRegex, timez.TimestampFmt(o.customFmt), data)
	}

	// Detect format
	if factory, stamp, err = format.Detect(bytes.NewReader(data)); err == nil {
		return factory, stamp, nil
	}

	// Failed to detect format, try timestamp regexes if any
	for _, spec := range o.stampRegex {
		if factory, stamp, err = timez.TryTimestampFormat(spec.Pattern, spec.Format, data); err == nil {
			break
		}
	}

	if err != nil {
		return nil, 0, err
	}

	return factory, stamp, nil
}

type optsT struct {
	customFmt   string
	customRegex string
	stampRegex  []FmtSpec
	window      int64
}

func parseOpts(opts ...OptT) *optsT {
	o := &optsT{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
