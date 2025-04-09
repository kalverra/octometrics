package testhelpers

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/logging"
)

var (
	silentTests = flag.Bool("silent", false, "Disable test logging to console")
)

type Option func(*options)

type options struct {
	silent bool
}

// Silent disables tests logging to console.
// Can also be set via the -silent flag.
// This option takes precedence over the -silent flag.
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

	silent := *silentTests

	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	silent = silent || opts.silent

	parts := strings.Split(tb.Name(), "/")
	for i, part := range parts {
		if part != "" {
			parts[i] = fmt.Sprintf("%s_test_results", part)
		}
	}

	testDir = filepath.Join(parts...)
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
