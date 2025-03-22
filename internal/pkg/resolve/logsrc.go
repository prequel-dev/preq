package resolve

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/prequel-dev/prequel-logmatch/pkg/format"
	"github.com/rs/zerolog/log"
)

type LogSrcI interface {
	io.ReadCloser
	Size() int64
	Name() string
	Fold() bool
	Window() int64
	Parser() format.ParserI
}

type logSrc struct {
	sz      int64
	ts      int64
	window  int64
	fh      *os.File
	rd      io.Reader
	factory format.FactoryI
	fold    bool
}

func newLogSrc(fn string, opts ...OptT) (src *logSrc, err error) {
	var fh *os.File
	defer func() {
		if err != nil && fh != nil {
			if cerr := fh.Close(); cerr != nil {
				log.Error().Err(cerr).Msg("Failed to close file")
				err = errors.Join(err, cerr)
			}
		}
	}()

	if fh, err = os.Open(fn); err != nil {
		return
	}

	rd, err := newReader(fn, fh)
	if err != nil {
		return
	}

	var buffer = make([]byte, detectSampleSize)
	n, err := io.ReadFull(rd, buffer)
	switch err {
	case nil, io.ErrUnexpectedEOF: // NOOP
	default:
		return nil, err
	}
	buffer = buffer[:n]

	o := parseOpts(opts...)
	factory, ts, err := NewLogFactory(buffer, opts...)

	if err != nil {
		return
	}

	if _, err = fh.Seek(0, io.SeekStart); err != nil {
		return
	}

	if rd, err = newReader(fn, fh); err != nil {
		return
	}

	var sz int64 = -1
	if !isGzip(fn) {
		if info, err := fh.Stat(); err == nil {
			sz = info.Size()
		}
	}

	// Only fold on Regex; doesn't make sense on CRI or JSON
	fold := factory.String() == format.FactoryRegex

	return &logSrc{
		sz:      sz,
		ts:      ts,
		fh:      fh,
		rd:      rd,
		factory: factory,
		window:  o.window,
		fold:    fold,
	}, nil
}

func isGzip(fn string) bool {
	return strings.HasSuffix(fn, ".gz") || strings.HasSuffix(fn, ".gzip")
}

func newReader(fn string, src io.Reader) (io.Reader, error) {
	if !isGzip(fn) {
		return src, nil
	}

	return gzip.NewReader(src)
}

func (ls *logSrc) Size() int64 {
	return ls.sz
}

func (ls *logSrc) Read(p []byte) (n int, err error) {
	return ls.rd.Read(p)
}

func (ls *logSrc) Close() error {
	return ls.fh.Close()
}

func (ls *logSrc) Parser() format.ParserI {
	return ls.factory.New()
}

func (ls *logSrc) Name() string {
	return ls.fh.Name()
}

func (ls *logSrc) Fold() bool {
	return ls.fold
}
func (ls *logSrc) Window() int64 {
	return ls.window
}
