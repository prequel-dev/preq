package logs

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestShortenCaller(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		line     int
		expected string
	}{
		{
			name:     "long unix path",
			file:     "/home/user/go/src/github.com/prequel-dev/preq/internal/pkg/cli/cli.go",
			line:     42,
			expected: "cli.go:42",
		},
		{
			name:     "long windows path",
			file:     `C:\Users\user\go\src\github.com\prequel-dev\preq\internal\pkg\cli\cli.go`,
			line:     101,
			expected: `C:\Users\user\go\src\github.com\prequel-dev\preq\internal\pkg\cli\cli.go:101`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := shortenCaller(0, tc.file, tc.line)
			if result != tc.expected {
				t.Errorf("Expected '%s', but got '%s'", tc.expected, result)
			}
		})
	}
}

func TestMkTimestampFormatter(t *testing.T) {
	const testTimeFormat = time.RFC1123

	var buf bytes.Buffer

	writer := zerolog.ConsoleWriter{
		Out:             &buf,
		FormatTimestamp: mkTimestampFormatter(testTimeFormat, 37),
		NoColor:         true,
	}

	logger := zerolog.New(writer).With().Timestamp().Logger()

	logTime := time.Date(2023, 10, 28, 10, 30, 0, 0, time.UTC)
	expectedTimeString := logTime.Local().Format(testTimeFormat)

	zerolog.TimestampFunc = func() time.Time {
		return logTime
	}
	t.Cleanup(func() {
		zerolog.TimestampFunc = time.Now
	})

	logger.Info().Msg("hello world")

	output := buf.String()

	if !strings.Contains(output, expectedTimeString) {
		t.Errorf("Expected log output to contain timestamp '%s', but it was not found in '%s'", expectedTimeString, output)
	}
}
