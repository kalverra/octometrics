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
	err := logging.Setup("test.log", "debug", true)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestGatherCommit(t *testing.T) {
	t.Parallel()
	t.Skip("not implemented properly yet")

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
	c := github.NewClient(mockedHTTPClient)
	commit, err := Commit(c, "kalverra", "octometrics", "mocked-sha-1", false)
	require.NoError(t, err, "error getting commit info")
	require.NotNil(t, commit, "commit should not be nil")
}
