package testhelpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/logging"
)

const (
	testLogFile        = "test.log.json"
	testLogLevelEnvVar = "OCTOMETRICS_TEST_LOG_LEVEL"
)

type Option func(*options)

type options struct {
	logLevel string
}

// Silent disables tests logging to console.
// Can also be set via the -silence-test-logs flag.
// This option takes precedence over the -silence-test-logs flag.
func Silent() Option {
	return func(o *options) {
		o.logLevel = "disabled"
	}
}

func LogLevel(level string) Option {
	return func(o *options) {
		o.logLevel = level
	}
}

func defaultOptions() *options {
	return &options{
		logLevel: "debug",
	}
}

// Setup creates a new test directory and returns a logger for the test.
// The test directory is removed when the test is finished, unless the test fails.
func Setup(tb testing.TB, options ...Option) (log zerolog.Logger, testDir string) {
	tb.Helper()

	opts := defaultOptions()
	envLogLevel := os.Getenv(testLogLevelEnvVar)
	if envLogLevel != "" {
		opts.logLevel = envLogLevel
	}
	for _, opt := range options {
		opt(opts)
	}

	testDir = filepath.Join("test_results", tb.Name())
	err := os.RemoveAll(testDir)
	require.NoError(tb, err, "error removing test_results dir")
	err = os.MkdirAll(testDir, 0700)
	require.NoError(tb, err, "error creating test_results dir")

	logFile := filepath.Join(testDir, testLogFile)
	loggingOpts := []logging.Option{
		logging.WithFileName(logFile),
		logging.WithLevel(opts.logLevel),
	}
	log, err = logging.New(loggingOpts...)
	log = log.With().Str("test_name", tb.Name()).Logger()
	require.NoError(tb, err, "error setting up logging")

	tb.Cleanup(func() {
		if tb.Failed() {
			log.Error().Msg("test failed, keeping data dir for debugging")
			return
		}

		log.Debug().Msg("Test completed, removing data dir")
		err := os.RemoveAll(testDir)
		require.NoError(tb, err, "error removing test_results dir")
	})
	return log, testDir
}
