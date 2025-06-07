package timez_test

import (
	"testing"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/timez"
)

func TestGetTimestampFormat(t *testing.T) {
	cb, err := timez.GetTimestampFormat(timez.FmtRfc3339)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ts, err := cb([]byte("2025-01-02T03:04:05Z"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	want := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC).UnixNano()
	if ts != want {
		t.Fatalf("expected %d got %d", want, ts)
	}
}

func TestEpochParserAndAny(t *testing.T) {
	cb, _ := timez.GetTimestampFormat(timez.FmtEpochMillis)
	ts, err := cb([]byte("42"))
	if err != nil || ts != 42*int64(time.Millisecond) {
		t.Fatalf("epoch millis failed")
	}

	cb, _ = timez.GetTimestampFormat(timez.FmtEpochAny)
	ts, err = cb([]byte("1000"))
	if err != nil || ts != 1000*int64(time.Second) {
		t.Fatalf("epoch any failed")
	}
}

func TestTryTimestampFormat(t *testing.T) {
	line := "2025-06-06T12:00:00Z first line\nsecond" // newline ensures only first line used
	factory, ts, err := timez.TryTimestampFormat(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`, timez.FmtRfc3339, []byte(line), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if factory == nil {
		t.Fatal("expected factory")
	}
	want := time.Date(2025, 6, 6, 12, 0, 0, 0, time.UTC).UnixNano()
	if ts != want {
		t.Fatalf("timestamp mismatch")
	}
}
