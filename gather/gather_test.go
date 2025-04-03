package gather

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/logging"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var (
	gatherOwner = "kalverra"
	gatherRepo  = "octometrics"
	verbose     bool
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Check if the -v flag is set
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "test.v" { // The verbose flag is internally named "test.v"
			verbose = true
		}
	})

	exitCode := m.Run()
	os.Exit(exitCode)
}

// testDataDir creates a temporary data directory for testing purposes.
func testSetup(t *testing.T, mockedHTTPClient *http.Client) (log zerolog.Logger, testDir string, client *github.Client) {
	t.Helper()

	baseDirName := fmt.Sprintf("./%s_testdata", t.Name())
	err := os.RemoveAll(baseDirName)
	require.NoError(t, err, "error removing testdata dir")
	err = os.Mkdir(baseDirName, 0700)
	require.NoError(t, err, "error creating testdata dir")

	logFile := filepath.Join(baseDirName, "test.log.json")
	loggingOpts := []logging.Option{
		logging.WithFileName(logFile),
		logging.WithLevel("trace"),
	}
	if !verbose {
		loggingOpts = append(loggingOpts, logging.DisableConsoleLog())
	}
	log, err = logging.New(loggingOpts...)
	log = log.With().Str("test_name", t.Name()).Logger()
	require.NoError(t, err, "error setting up logging")

	t.Cleanup(func() {
		log = log.With().Bool("test_failed", t.Failed()).Str("data_dir", baseDirName).Logger()
		if t.Failed() {
			log.Error().Msg("test failed, keeping data dir for debugging")
			return
		}

		log.Debug().Msg("test completed, removing data dir")
		err := os.RemoveAll(baseDirName)
		require.NoError(t, err, "error removing testdata dir")
	})

	client, err = GitHubClient(log, MockGitHubToken, mockedHTTPClient)
	require.NoError(t, err, "error creating GitHub client")
	return log, baseDirName, client
}
