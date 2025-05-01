package resolve

import (
	"bytes"
	"io"
	"os"

	"github.com/prequel-dev/prequel-logmatch/pkg/format"
	"github.com/rs/zerolog/log"
)

func PipeStdin(opts ...OptT) ([]*LogData, error) {
	stdin, err := _pipeStdin(opts...)
	if err != nil {
		return nil, err
	}
	if stdin == nil {
		return nil, nil
	}

	return []*LogData{
		NewLogData([]LogSrcI{stdin}, "stdin", "*"),
	}, nil
}

func PipeEval(data []byte, opts ...OptT) ([]*LogData, error) {
	rdr, err := newPipeReader(bytes.NewReader(data), opts...)
	if err != nil {
		return nil, err
	}
	if rdr == nil {
		return nil, nil
	}

	return []*LogData{
		NewLogData([]LogSrcI{rdr}, "stdin", "*"),
	}, nil
}

func newPipeReader(r io.Reader, opts ...OptT) (*PipeRdrT, error) {
	// Read a sample to detect format
	buf := make([]byte, detectSampleSize)
	n, err := io.ReadFull(r, buf)
	switch err {
	case nil, io.ErrUnexpectedEOF: // NOOP
	default:
		return nil, err
	}

	buf = buf[:n] // shrink to actual read size

	// Perform detection
	o := parseOpts(opts...)
	factory, _, err := NewLogFactory(buf, opts...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create log factory")
		return nil, err
	}

	// Only fold on Regex or rfc3339Nano; doesn't make sense on CRI or JSON
	var fold bool
	switch factory.String() {
	case format.FactoryRegex, format.FactoryRfc3339Nano:
		fold = true
	}

	return &PipeRdrT{
		src:      r,
		prologue: bytes.NewBuffer(buf),
		factory:  factory,
		window:   o.window,
		fold:     fold,
	}, nil
}

func _pipeStdin(opts ...OptT) (*PipeRdrT, error) {

	fi, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}

	// If stdin is not a pipe, return nil
	if (fi.Mode() & os.ModeCharDevice) != 0 {
		return nil, nil
	}

	return newPipeReader(os.Stdin, opts...)
}

type PipeRdrT struct {
	src      io.Reader
	window   int64
	prologue *bytes.Buffer
	factory  format.FactoryI
	fold     bool
}

func (p *PipeRdrT) Parser() format.ParserI {
	return p.factory.New()
}

func (p *PipeRdrT) Close() error {
	return nil
}

func (p *PipeRdrT) Size() int64 {
	return -1
}

func (p *PipeRdrT) Name() string {
	return "stdin"
}

func (p *PipeRdrT) Fold() bool {
	return p.fold
}

func (p *PipeRdrT) Window() int64 {
	return p.window
}

func (p *PipeRdrT) Read(b []byte) (int, error) {
	if p.prologue != nil {
		n, err := p.prologue.Read(b)
		if p.prologue.Len() == 0 {
			p.prologue = nil
		}
		return n, err
	}
	return p.src.Read(b)
}
