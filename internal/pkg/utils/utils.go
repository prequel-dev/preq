package utils

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/prequel-dev/prequel-compiler/pkg/parser"

	"gopkg.in/yaml.v2"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

var (
	ErrGzip  = errors.New("gzip error")
	ErrRead  = errors.New("read error")
	ErrWrite = errors.New("write error")
)

var (
	sectionRules = "rules"
)

type RuleTypeT string

const (
	RuleTypeCre  RuleTypeT = "cre"
	RuleTypeUser RuleTypeT = "user"
)

type RulePathT struct {
	Path string
	Type RuleTypeT
}

func GetStopTime() (ts int64) {
	return math.MaxInt64
}

func GetOSInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func OpenRulesFile(filePath string) (io.Reader, func(), error) {

	var (
		file *os.File
		buf  [2]byte
		err  error
	)

	if file, err = os.Open(filePath); err != nil {
		return nil, nil, err
	}

	cleanup := func() { file.Close() }

	if _, err = file.Read(buf[:]); err != nil {
		file.Close()
		return nil, nil, err
	}

	if _, err = file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, nil, err
	}

	if buf[0] == 0x1f && buf[1] == 0x8b {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, nil, err
		}
		cleanup = func() {
			gzReader.Close()
			file.Close()
		}
		return gzReader, cleanup, nil
	}

	return file, cleanup, nil
}

func ExtractSectionBytes(rdr io.Reader, targetSection string) ([]byte, error) {
	yr := utilyaml.NewYAMLReader(bufio.NewReader(rdr))

	for {
		docBytes, err := yr.Read() // raw bytes up to the next '---'
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("section %q not found", targetSection)
			}
			return nil, err
		}

		var meta struct {
			Section string `yaml:"section"`
		}

		if err := yaml.Unmarshal(docBytes, &meta); err != nil {
			continue
		}

		if meta.Section == targetSection {
			return docBytes, nil
		}
	}
}

type ReaderOptT func(*readerOptsT)

func WithMultiDoc() func(*readerOptsT) {
	return func(o *readerOptsT) {
		o.multiDoc = true
	}
}

func WithGenIds() func(*readerOptsT) {
	return func(o *readerOptsT) {
		o.genIds = true
	}
}

type readerOptsT struct {
	multiDoc bool
	genIds   bool
}

func readerOpts(opts ...ReaderOptT) *readerOptsT {
	o := &readerOptsT{}
	for _, opt := range opts {
		opt(o)
	}

	return o
}

func ParseRulesPath(path string, opts ...ReaderOptT) (*parser.RulesT, error) {
	o := readerOpts(opts...)

	var (
		reader     io.Reader
		rulesBytes []byte
		readOpts   = make([]parser.ParseOptT, 0)
		close      func()
		err        error
	)

	if reader, close, err = OpenRulesFile(path); err != nil {
		return nil, err
	}
	defer close()

	if o.multiDoc {
		if rulesBytes, err = ExtractSectionBytes(reader, sectionRules); err != nil {
			return nil, err
		}
		return parser.Read(bytes.NewReader(rulesBytes))
	}

	if o.genIds {
		readOpts = append(readOpts, parser.WithGenIds())
	}

	return parser.Read(reader, readOpts...)
}

func ParseRules(rdr io.Reader, opts ...ReaderOptT) (*parser.RulesT, error) {
	o := readerOpts(opts...)

	if o.genIds {
		return parser.Read(rdr, parser.WithGenIds())
	}

	return parser.Read(rdr)
}

func GunzipBytes(path string) ([]byte, error) {

	var (
		compressedData []byte
		gzReader       *gzip.Reader
		decompressed   bytes.Buffer
		err            error
	)

	if compressedData, err = os.ReadFile(path); err != nil {
		return nil, ErrRead
	}

	if gzReader, err = gzip.NewReader(bytes.NewReader(compressedData)); err != nil {
		return nil, ErrGzip
	}
	defer gzReader.Close()

	if _, err = io.Copy(&decompressed, gzReader); err != nil {
		return nil, ErrWrite
	}

	return decompressed.Bytes(), nil
}

func Sha256Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	tmp := fmt.Sprintf("%s.tmp", dst)

	dstFile, err := os.Create(tmp)
	if err != nil {
		return err
	}

	// Copy file
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		dstFile.Close()
		return err
	}

	// Close dst file before rename to avoid permissions problems on Windows
	err = dstFile.Close()
	if err != nil {
		return err
	}

	// Copy permissions from source to destination
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	err = os.Chmod(tmp, srcInfo.Mode())
	if err != nil {
		return err
	}

	err = os.Rename(tmp, dst)
	if err != nil {
		return err
	}

	return nil
}

func UrlBase(fullUrl string) (string, error) {
	u, err := url.Parse(fullUrl)
	if err != nil {
		return "", err
	}
	return filepath.Base(u.Path), nil
}
