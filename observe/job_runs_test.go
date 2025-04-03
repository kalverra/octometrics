package observe

import (
	"fmt"
	"os"
	"testing"

	"github.com/kalverra/octometrics/logging"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestJobs(t *testing.T) {
	t.Parallel()
	t.Skip("not implemented fully yet")

	log, testDir := testSetup(t)

	err := JobRuns(log, nil, observeOwner, observeRepo, 1, WithCustomOutputDir(testDir))
	require.NoError(t, err, "error observing job runs")
}

func testSetup(t *testing.T) (logger zerolog.Logger, testDir string) {
	t.Helper()

	testDir = t.TempDir()
	logFileName := fmt.Sprintf("%s.log.json", t.Name())
	logger, err := logging.New(
		logging.WithFileName(logFileName),
		logging.WithLevel("trace"),
	)
	require.NoError(t, err, "error creating logger")
	require.NotNil(t, logger, "logger should not be nil")
	logger = logger.With().Str("test", t.Name()).Logger()
	t.Cleanup(func() {
		if t.Failed() {
			logger.Error().Str("log_file", logFileName).Msg("test failed, keeping log file for debugging")
			return
		}

		err = os.Remove(logFileName)
		require.NoError(t, err, "error removing log file")
	})
	return logger, testDir
}
