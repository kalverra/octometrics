package gather

import (
	"os"
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/logging"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	err := logging.Setup("gather.test.log.json", "debug", true)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// testDataDir creates a temporary data directory for testing purposes.
func testDataDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "octometrics-test")
	require.NoError(t, err, "error creating temp dir")
	t.Cleanup(func() {
		err := os.RemoveAll(dir)
		require.NoError(t, err, "error removing temp dir")
	})
	return dir
}

func TestGatherCommit(t *testing.T) {
	t.Parallel()
	t.Skip("skipping test for now, need to fix the test data generation")

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
			github.CheckRun{
				ID:          github.Ptr(int64(1)),
				Name:        github.Ptr("mocked-check-run"),
				Status:      github.Ptr("completed"),
				Conclusion:  github.Ptr("success"),
				StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
				CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
				HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/1"),
			},
		),
	)

	testDataDir := testDataDir(t)
	c := github.NewClient(mockedHTTPClient)
	commit, err := Commit(c, "kalverra", "octometrics", "mocked-sha-1", ForceUpdate(), CustomDataFolder(testDataDir))
	require.NoError(t, err, "error getting commit info")
	require.NotNil(t, commit, "commit should not be nil")
}
