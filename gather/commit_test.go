package gather

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/logging"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// testDataDir creates a temporary data directory for testing purposes.
func testSetup(t *testing.T) (log zerolog.Logger, testDir string) {
	t.Helper()

	baseDirName := fmt.Sprintf("./%s_testdata", t.Name())
	err := os.RemoveAll(baseDirName)
	require.NoError(t, err, "error removing testdata dir")
	err = os.Mkdir(baseDirName, 0755)
	require.NoError(t, err, "error creating testdata dir")

	logFile := filepath.Join(baseDirName, "log.json")
	log, err = logging.New(
		logging.WithFileName(logFile),
		logging.WithLevel("trace"),
		logging.DisableConsoleLog(),
	)
	require.NoError(t, err, "error setting up logging")
	t.Cleanup(func() {
		if t.Failed() {
			log.Error().Str("data_dir", baseDirName).Msg("test failed, keeping data dir for debugging")
			return
		}

		err := os.RemoveAll(baseDirName)
		require.NoError(t, err, "error removing testdata dir")
	})
	return log, baseDirName
}

func TestGatherCommit(t *testing.T) {
	t.Parallel()
	t.Skip("skipping test for now, need to fix the test data generation")

	log, testDataDir := testSetup(t)

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposCommitsByOwnerByRepoByRef,
			github.RepositoryCommit{
				SHA:    github.Ptr("mocked-sha-1"),
				Commit: &github.Commit{},
			},
		),
		mock.WithRequestMatch(
			mock.GetReposCommitsCheckRunsByOwnerByRepoByRef,
			[]*github.CheckRun{
				{
					ID:          github.Ptr(int64(1)),
					Name:        github.Ptr("mocked-check-run-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
					CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
					HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/1"),
				},
				{
					ID:          github.Ptr(int64(2)),
					Name:        github.Ptr("mocked-check-run-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("failure"),
					StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
					CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
					HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/2"),
				},
			},
		),
	)

	c := github.NewClient(mockedHTTPClient)
	commit, err := Commit(log, c, "kalverra", "octometrics", "mocked-sha-1", ForceUpdate(), CustomDataFolder(testDataDir))
	require.NoError(t, err, "error getting commit info")
	require.NotNil(t, commit, "commit should not be nil")
}
