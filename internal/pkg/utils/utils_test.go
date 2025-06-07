package utils_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/prequel-dev/preq/internal/pkg/utils"
)

func TestSha256Sum(t *testing.T) {
	got := utils.Sha256Sum([]byte("hello world"))
	want := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if got != want {
		t.Fatalf("expected %s got %s", want, got)
	}
}

func TestUrlBase(t *testing.T) {
	base, err := utils.UrlBase("https://example.com/path/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "file.txt" {
		t.Fatalf("expected file.txt got %s", base)
	}
	if _, err := utils.UrlBase(":bad://url"); err == nil {
		t.Fatalf("expected error for bad url")
	}
}

func TestCopyFileAndOpenRulesFile(t *testing.T) {
	srcFile, err := os.CreateTemp(t.TempDir(), "src-*.txt")
	if err != nil {
		t.Fatalf("%v", err)
	}
	srcFile.WriteString("content")
	srcFile.Close()

	dstPath := srcFile.Name() + "-dst"
	if err := utils.CopyFile(srcFile.Name(), dstPath); err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	dstBytes, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(dstBytes) != "content" {
		t.Fatalf("copy mismatch")
	}

	r, cleanup, err := utils.OpenRulesFile(dstPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	data, _ := io.ReadAll(r)
	cleanup()
	if string(data) != "content" {
		t.Fatalf("expected content from open")
	}

	// gzipped file
	gzPath := dstPath + ".gz"
	f, _ := os.Create(gzPath)
	gz := gzip.NewWriter(f)
	gz.Write([]byte("gzipped"))
	gz.Close()
	f.Close()

	r, cleanup, err = utils.OpenRulesFile(gzPath)
	if err != nil {
		t.Fatalf("open gzip failed: %v", err)
	}
	data, _ = io.ReadAll(r)
	cleanup()
	if string(data) != "gzipped" {
		t.Fatalf("expected gzipped content")
	}
}

func TestGunzipBytes(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/data.gz"
	f, _ := os.Create(path)
	gz := gzip.NewWriter(f)
	gz.Write([]byte("hello"))
	gz.Close()
	f.Close()

	out, err := utils.GunzipBytes(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("expected hello got %s", string(out))
	}
}

func TestExtractSectionBytes(t *testing.T) {
	yamlData := "section: other\n" +
		"---\n" +
		"section: rules\nfoo: bar\n"
	b, err := utils.ExtractSectionBytes(bytes.NewReader([]byte(yamlData)), "rules")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !bytes.Contains(b, []byte("foo: bar")) {
		t.Fatalf("unexpected section bytes: %s", string(b))
	}
}

func TestExtractSectionBytesNotFound(t *testing.T) {
	yamlData := "section: other\n"
	_, err := utils.ExtractSectionBytes(bytes.NewReader([]byte(yamlData)), "rules")
	if err == nil {
		t.Fatalf("expected error for missing section")
	}
}

func TestGetOSInfoAndStopTime(t *testing.T) {
	info := utils.GetOSInfo()
	if !strings.Contains(info, runtime.GOOS) {
		t.Fatalf("unexpected os info: %s", info)
	}
	if utils.GetStopTime() <= 0 {
		t.Fatalf("expected positive stop time")
	}
}

func TestParseRules(t *testing.T) {
	rule := "rules:\n  - cre:\n      id: r1\n    rule:\n      set:\n        window: 1s\n        event:\n          source: t\n        match:\n          - test\n"
	r, err := utils.ParseRules(bytes.NewBufferString(rule), utils.WithGenIds())
	if err != nil {
		t.Fatalf("ParseRules: %v", err)
	}
	if len(r.Rules) != 1 {
		t.Fatalf("expected 1 rule got %d", len(r.Rules))
	}
}

func TestParseRulesPathMultiDoc(t *testing.T) {
	data := "section: other\n---\nsection: rules\n" +
		"rules:\n  - metadata:\n      id: m1\n    cre:\n      id: r2\n    rule:\n      set:\n        window: 1s\n        event:\n          source: t\n        match:\n          - test\n"
	tmp := filepath.Join(t.TempDir(), "rules.yaml")
	os.WriteFile(tmp, []byte(data), 0644)
	r, err := utils.ParseRulesPath(tmp, utils.WithMultiDoc())
	if err != nil {
		t.Fatalf("ParseRulesPath: %v", err)
	}
	if len(r.Rules) != 1 {
		t.Fatalf("expected 1 rule got %d", len(r.Rules))
	}
}
