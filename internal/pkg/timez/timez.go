package timez

import (
	"bytes"
	"errors"
	"strconv"
	"time"

	"github.com/prequel-dev/prequel-logmatch/pkg/format"
	"github.com/rs/zerolog/log"
)

const (
	DefaultSkip = 50
)

var (
	ErrInvalidTimestampFormat = errors.New("invalid timestamp format")
)

type TimestampFmt string

const (
	FmtRfc3339      TimestampFmt = "rfc3339"
	FmtRfc3339Nano  TimestampFmt = "rfc3339nano"
	FmtUnix         TimestampFmt = "unix"
	FmtEpochAny     TimestampFmt = "epochany"
	FmtEpochSeconds TimestampFmt = "epochseconds"
	FmtEpochMillis  TimestampFmt = "epochmillis"
	FmtEpochMicros  TimestampFmt = "epochmicros"
	FmtEpochNanos   TimestampFmt = "epochnanos"
)

func GetTimestampFormat(f TimestampFmt) (format.TimeFormatCbT, error) {

	switch f {
	case FmtRfc3339:
		return format.WithTimeFormat(time.RFC3339), nil
	case FmtRfc3339Nano:
		return format.WithTimeFormat(time.RFC3339Nano), nil
	case FmtUnix:
		return format.WithTimeFormat(time.UnixDate), nil
	case FmtEpochAny:
		return epochAny, nil
	case FmtEpochSeconds:
		return epochSeconds, nil
	case FmtEpochMillis:
		return epochMillis, nil
	case FmtEpochMicros:
		return epochMicros, nil
	case FmtEpochNanos:
		return epochNanos, nil
	default:
		return format.WithTimeFormat(string(f)), nil
	}
}

var (
	epochSeconds = epochParser(time.Second)
	epochMillis  = epochParser(time.Millisecond)
	epochMicros  = epochParser(time.Microsecond)
	epochNanos   = epochParser(time.Nanosecond)
)

func epochParser(unit time.Duration) format.TimeFormatCbT {
	return func(m []byte) (int64, error) {
		v, err := strconv.ParseInt(string(m), 10, 64)
		if err != nil {
			return 0, ErrInvalidTimestampFormat
		}
		return v * int64(unit), nil
	}
}

func epochAny(m []byte) (int64, error) {
	v, err := strconv.ParseInt(string(m), 10, 64)
	if err != nil {
		return 0, ErrInvalidTimestampFormat
	}

	sz := len(m)
	switch {
	case sz > 16:
		// NOOP: v *= int64(time.Nanosecond)
	case sz > 13:
		v *= int64(time.Microsecond)
	case sz > 10:
		v *= int64(time.Millisecond)
	default:
		v *= int64(time.Second)
	}
	return v, nil

}

func TryTimestampFormat(exp string, fmtStr TimestampFmt, buf []byte, maxTries int) (format.FactoryI, int64, error) {

	var (
		ts      int64
		factory format.FactoryI
		cb      format.TimeFormatCbT
		err     error
	)

	log.Debug().Str("exp", exp).Str("fmt", string(fmtStr)).Msg("Trying timestamp format")

	if cb, err = GetTimestampFormat(fmtStr); err != nil {
		log.Error().Err(err).Msg("Failed to get timestamp format")
		return nil, 0, err
	}

	if factory, err = format.NewRegexFactory(exp, cb); err != nil {
		log.Error().Err(err).Msg("Failed to create regex factory")
		return nil, 0, err
	}

	f := factory.New()
	ts, err = f.ReadTimestamp(bytes.NewReader(buf))

	tries := 0
	for (err != nil || ts == 0) && tries < maxTries {
		// First line may contain a header; try up to N lines
		tries += 1
		if index := bytes.IndexByte(buf, '\n'); index != -1 {
			buf = buf[index+1:]
			ts, err = f.ReadTimestamp(bytes.NewReader(buf))
		} else {
			break
		}
	}

	if err != nil {
		log.Info().Err(err).Msg("Failed to read timestamp")
		return nil, 0, err
	}

	if ts == 0 {
		return nil, 0, ErrInvalidTimestampFormat
	}

	log.Debug().Str("exp", exp).Str("fmt", string(fmtStr)).Msg("Selected timestamp format")

	return factory, ts, nil
}
