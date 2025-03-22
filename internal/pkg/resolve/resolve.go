package resolve

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"

	"path/filepath"

	"github.com/prequel-dev/detection-engine/pkg/datasrc"
	"github.com/rs/zerolog/log"
)

var (
	ErrorSourceType = errors.New("unsupported source type")
)

const (
	logType = "log"
)

type (
	DataSources = datasrc.DataSources
)

func Resolve(dss *DataSources, opts ...OptT) []*LogData {
	var sources []*LogData

	for _, src := range dss.Sources {

		dataSrc, err := resolveSource(src, opts...)
		if err != nil {
			log.Info().
				Err(err).
				Str("name", src.Name).
				Str("type", src.Type).
				Msg("Failed to resolve source")
		} else {
			sources = append(sources, dataSrc)
		}
	}

	return sources
}

func resolveSource(src datasrc.Source, opts ...OptT) (*LogData, error) {
	var (
		errList []error
	)

	ts := src.Timestamp

	if src.Window != 0 {
		opts = append(opts, WithWindow(int64(src.Window)))
	}

	for idx, location := range src.Locations {

		switch location.Type {
		case "", logType:
			if slogs, err := resolveLog(location, ts, opts...); err == nil {
				dataSrc := NewLogData(slogs, src.Name, src.Type)
				return dataSrc, nil
			} else {
				log.Info().
					Err(err).
					Int("idx", idx).
					Msg("Failed to resolve log source")
				errList = append(errList, err)
			}

		default:
			log.Info().
				Int("idx", idx).
				Str("type", location.Type).
				Msg("Unsupported source type")
			errList = append(errList, fmt.Errorf("%w: %s", ErrorSourceType, location.Type))
		}
	}
	return nil, errors.Join(errList...)
}

func resolveLog(location datasrc.Location, ts *datasrc.Timestamp, opts ...OptT) ([]LogSrcI, error) {

	matches, err := filepath.Glob(location.Path)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return nil, os.ErrNotExist
	}

	// Use location timestamp if provided, otherwise source timestamp (if specified)
	if location.Timestamp != nil {
		ts = location.Timestamp
	}

	if ts != nil && ts.Regex != "" && ts.Format != "" {
		opts = append(opts, WithCustomFmt(ts.Regex, ts.Format))
	}

	if location.Window != 0 {
		opts = append(opts, WithWindow(int64(location.Window)))
	}

	var (
		errList  []error
		resolved []*logSrc
	)

	for _, match := range matches {
		lsrc, err := newLogSrc(match, opts...)
		if err != nil {
			log.Info().
				Err(err).
				Str("path", match).
				Msg("Failed to interpret log")
			errList = append(errList, err)
			continue
		}

		log.Info().
			Str("path", match).
			Str("format", lsrc.factory.String()).
			Int64("ts", lsrc.ts).
			Msg("Resolved log")

		resolved = append(resolved, lsrc)
	}

	if len(resolved) == 0 {
		return nil, errors.Join(errList...)
	}

	slices.SortFunc(resolved, func(a, b *logSrc) int {
		return cmp.Compare(a.ts, b.ts)
	})

	slogs := make([]LogSrcI, len(resolved))
	for idx, log := range resolved {
		slogs[idx] = log
	}

	return slogs, nil
}
