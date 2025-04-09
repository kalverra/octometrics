package logging

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	//nolint:unused // only here to make it valid for others who call the test helpers
	silenceTestLogs = flag.Bool(
		"silence-test-logs",
		false,
		"Disable test logging to console",
	)
)

func TestLogging(t *testing.T) {
	t.Parallel()

	logFile := "logging_test.log"
	fileLogger, err := New(
		WithFileName(logFile),
		WithLevel("trace"),
		DisableConsoleLog(),
	)
	require.NoError(t, err, "error creating logger")
	require.NotNil(t, fileLogger, "logger should not be nil")
	require.FileExists(t, logFile, "log file should exist")
	t.Cleanup(func() {
		err := os.Remove(logFile)
		require.NoError(t, err, "error removing log file")
	})

	fileLogger.Info().Msg("This is an info log message.")
	fileLogger.Debug().Msg("This is a debug log message.")
	fileLogger.Error().Msg("This is an error log message.")
	fileLogger.Trace().Msg("This is a trace log message.")
	fileLogger.Warn().Msg("This is a warning log message.")

	logFileData, err := os.ReadFile(logFile)
	require.NoError(t, err, "error reading log file")
	require.NotEmpty(t, logFileData, "log file should not be empty")
	require.Contains(
		t,
		string(logFileData),
		"This is an info log message.",
		"log file should contain info log message",
	)
	require.Contains(
		t,
		string(logFileData),
		"This is a debug log message.",
		"log file should contain debug log message",
	)
	require.Contains(
		t,
		string(logFileData),
		"This is an error log message.",
		"log file should contain error log message",
	)
	require.Contains(
		t,
		string(logFileData),
		"This is a trace log message.",
		"log file should contain trace log message",
	)
	require.Contains(
		t,
		string(logFileData),
		"This is a warning log message.",
		"log file should contain warning log message",
	)
}
