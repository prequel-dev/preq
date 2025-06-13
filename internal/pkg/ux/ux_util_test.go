package ux

import (
	"os"
	"strings"
	"testing"

	"github.com/Masterminds/semver"
)

func TestPrintVersion(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	// Call PrintVersion with a nil version
	PrintVersion("configDir", "currRulesPath", nil)

	// Close the pipe and read the output
	w.Close()
	var buf [1024]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// Verify output contains expected strings
	if !strings.Contains(output, "No rules installed") {
		t.Error("Expected output to contain 'No rules installed'")
	}
	if !strings.Contains(output, "Learn more at https://docs.prequel.dev") {
		t.Error("Expected output to contain 'Learn more at https://docs.prequel.dev'")
	}
	if !strings.Contains(output, "darwin/amd64") {
		t.Error("Expected output to contain OS/arch")
	}
}

func TestPrintUsage(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	// Call PrintUsage
	PrintUsage()

	// Close the pipe and read the output
	w.Close()
	var buf [1024]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// Verify output contains expected strings
	if !strings.Contains(output, "Usage:") {
		t.Errorf("Expected output to contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "See --help or visit https://docs.prequel.dev for more information") {
		t.Error("Expected output to contain help message")
	}
}

func TestNewProgressWriter(t *testing.T) {
	pw := NewProgressWriter(3)
	if pw == nil {
		t.Error("Expected NewProgressWriter to return a non-nil progress.Writer")
	}
}

func TestRootProgress(t *testing.T) {
	pw := RootProgress(true)
	if pw == nil {
		t.Error("Expected RootProgress to return a non-nil progress.Writer")
	}
}

func TestNewRuleTracker(t *testing.T) {
	tracker := NewRuleTracker()
	if tracker.Message != "Parsing rules" {
		t.Error("Expected NewRuleTracker to set correct message")
	}
}

func TestNewProblemsTracker(t *testing.T) {
	tracker := NewProblemsTracker()
	if tracker.Message != "Problems detected" {
		t.Error("Expected NewProblemsTracker to set correct message")
	}
}

func TestNewLineTracker(t *testing.T) {
	tracker := NewLineTracker()
	if tracker.Message != "Matching lines" {
		t.Error("Expected NewLineTracker to set correct message")
	}
}

func TestNewDownloadTracker(t *testing.T) {
	tracker := NewDownloadTracker(1000)
	if tracker.Total != 1000 {
		t.Error("Expected NewDownloadTracker to set correct total")
	}
}

func TestPrintEmailVerifyNotice(t *testing.T) {
	// Redirect stderr to capture output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	// Call PrintEmailVerifyNotice
	PrintEmailVerifyNotice("test@example.com")

	// Close the pipe and read the output
	w.Close()
	var buf [1024]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// Verify output contains expected strings
	if !strings.Contains(output, "You're one step away! Please verify your email") {
		t.Error("Expected output to contain email verification title")
	}
	if !strings.Contains(output, "test@example.com") {
		t.Error("Expected output to contain email address")
	}
}

func TestProcessName(t *testing.T) {
	name := ProcessName()
	if name == "" {
		t.Error("Expected ProcessName to return a non-empty string")
	}
}

func TestPrintDeviceAuthUrl(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	// Call PrintDeviceAuthUrl
	PrintDeviceAuthUrl("https://example.com/auth")

	// Close the pipe and read the output
	w.Close()
	var buf [1024]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// Verify output contains expected strings
	if !strings.Contains(output, "https://example.com/auth") {
		t.Error("Expected output to contain auth URL")
	}
}

func TestWriteDataSourceTemplate(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "preq-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Call WriteDataSourceTemplate
	ver, _ := semver.NewVersion("1.0.0")
	template := []byte("# Test template")
	output, err := WriteDataSourceTemplate("test", ver, template)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output == "" {
		t.Error("Expected WriteDataSourceTemplate to return a non-empty output path")
	}

	// Verify file exists
	if _, err := os.Stat(output); os.IsNotExist(err) {
		t.Error("Expected file to exist")
	}
}
