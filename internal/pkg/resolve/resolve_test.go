package resolve

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prequel-dev/prequel-compiler/pkg/datasrc"
)

func createTestFile(t *testing.T, dir, name, content string, useGzip bool) string {
	t.Helper()
	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	var fileContent strings.Builder
	fileContent.WriteString(content + "\n")
	for fileContent.Len() < detectSampleSize+100 {
		fileContent.WriteString("This is a filler line to make the file large enough.\n")
	}

	finalBytes := []byte(fileContent.String())

	if useGzip {
		gz := gzip.NewWriter(f)
		if _, err := gz.Write(finalBytes); err != nil {
			t.Fatalf("Failed to write gzipped content: %v", err)
		}
		gz.Close()
	} else {
		if _, err := f.Write(finalBytes); err != nil {
			t.Fatalf("Failed to write plain content: %v", err)
		}
	}
	f.Close()
	return path
}

func TestNewLogSrc(t *testing.T) {
	tempDir := t.TempDir()
	logContent := "2023-10-28T10:30:00Z This is a test log line."

	t.Run("with plain text file", func(t *testing.T) {
		path := createTestFile(t, tempDir, "test.log", logContent, false)

		src, err := newLogSrc(path)
		if err != nil {
			t.Fatalf("newLogSrc failed for plain file: %v", err)
		}
		defer src.Close()

		info, _ := os.Stat(path)
		if src.Size() != info.Size() {
			t.Errorf("Expected size %d, got %d", info.Size(), src.Size())
		}
	})

	t.Run("with gzipped file", func(t *testing.T) {
		path := createTestFile(t, tempDir, "test.log.gz", logContent, true)

		src, err := newLogSrc(path)
		if err != nil {
			t.Fatalf("newLogSrc failed for gzipped file: %v", err)
		}
		defer src.Close()

		if src.Size() != -1 {
			t.Errorf("Expected size -1 for gzip, got %d", src.Size())
		}
	})

	t.Run("with window option", func(t *testing.T) {
		path := createTestFile(t, tempDir, "window.log", logContent, false)
		expectedWindow := int64(30)

		src, err := newLogSrc(path, WithWindow(expectedWindow))
		if err != nil {
			t.Fatalf("newLogSrc failed: %v", err)
		}
		defer src.Close()

		if src.Window() != expectedWindow {
			t.Errorf("Expected window %d, got %d", expectedWindow, src.Window())
		}
	})

	t.Run("with non-existent file", func(t *testing.T) {
		_, err := newLogSrc(filepath.Join(tempDir, "not-real.log"))
		if err == nil {
			t.Fatal("Expected an error for a non-existent file, but got nil")
		}
	})
}

func TestResolveSource(t *testing.T) {
	tempDir := t.TempDir()
	logContent := "2023-10-28T10:40:00Z some log content"
	logPath := createTestFile(t, tempDir, "app.log", logContent, false)

	t.Run("with valid source", func(t *testing.T) {
		dss := &datasrc.DataSources{
			Sources: []datasrc.Source{
				{
					Name:      "my-app",
					Type:      "log",
					Locations: []datasrc.Location{{Path: logPath}},
				},
			},
		}

		results := Resolve(dss)
		if len(results) != 1 {
			t.Fatalf("Expected 1 resolved source, got %d", len(results))
		}
		if results[0].Name() != "my-app" {
			t.Errorf("Expected resolved source name to be 'my-app', got '%s'", results[0].Name())
		}
	})
}

func TestPipeStdin(t *testing.T) {
	var b strings.Builder
	b.WriteString("2023-10-28T11:00:00Z Piped log content\n")
	for b.Len() < detectSampleSize+100 {
		b.WriteString("Filler line for stdin test.\n")
	}
	logContent := b.String()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}

	originalStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = originalStdin
	})

	go func() {
		defer w.Close()
		w.Write([]byte(logContent))
	}()

	results, err := PipeStdin()
	if err != nil {
		t.Fatalf("PipeStdin returned an unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected PipeStdin to return 1 LogData source, but got %d", len(results))
	}
}
