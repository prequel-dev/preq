package utils

import (
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
	"strings"
	"time"

	"github.com/prequel-dev/prequel-compiler/pkg/parser"
)

var (
	ErrGzip  = errors.New("gzip error")
	ErrRead  = errors.New("read error")
	ErrWrite = errors.New("write error")
)

func ParseTime(tstr, def string) (ts int64, err error) {
	tstr = strings.TrimSpace(tstr)
	if tstr == "" {
		tstr = def
	}

	now := time.Now()

	switch tstr {
	case "now":
		ts = now.UnixNano()
	case "-infinity", "-inf", "-∞", "0":
		ts = 0
	case "infinity", "+inf", "inf", "∞", "+":
		ts = math.MaxInt64
	default:
		if d, err := time.ParseDuration(tstr); err == nil {
			ts = now.Add(d).UnixNano()
		} else if stamp, err := time.Parse(time.RFC3339, tstr); err == nil {
			ts = stamp.UnixNano()
		} else {
			return 0, fmt.Errorf("fail parse timestamp: %w", err)
		}
	}

	return ts, nil
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

func ParseRulesPath(path string) (*parser.RulesT, error) {
	var (
		reader io.Reader
		close  func()
		err    error
	)

	if reader, close, err = OpenRulesFile(path); err != nil {
		return nil, err
	}
	defer close()

	return parser.Read(reader)
}

func ParseRules(rdr io.Reader) (*parser.RulesT, error) {
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
