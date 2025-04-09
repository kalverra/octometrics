package testhelpers

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/logging"
)

var (
	silenceTestLogs = flag.Bool("silence-test-logs", false, "Disable test logging to console")
)

type Option func(*options)

type options struct {
	silent bool
}

// Silent disables tests logging to console.
// Can also be set via the -silence-test-logs flag.
// This option takes precedence over the -silence-test-logs flag.
func Silent() Option {
	return func(o *options) {
		o.silent = true
	}
}

func defaultOptions() *options {
	return &options{
		silent: false,
	}
}

// Setup creates a new test directory and returns a logger for the test.
// The test directory is removed when the test is finished, unless the test fails.
func Setup(tb testing.TB, options ...Option) (log zerolog.Logger, testDir string) {
	tb.Helper()

	silent := *silenceTestLogs

	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	silent = silent || opts.silent

	testDir = filepath.Join("test_results", tb.Name())
	err := os.RemoveAll(testDir)
	require.NoError(tb, err, "error removing test_results dir")
	err = os.MkdirAll(testDir, 0700)
	require.NoError(tb, err, "error creating test_results dir")

	logFile := filepath.Join(testDir, "test.log.json")
	loggingOpts := []logging.Option{
		logging.WithFileName(logFile),
		logging.WithLevel("debug"),
	}
	if silent {
		loggingOpts = append(loggingOpts, logging.DisableConsoleLog())
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
