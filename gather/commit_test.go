package gather

import (
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestGatherCommit(t *testing.T) {
	t.Parallel()
	t.Skip("Check runs in the mock client seem to be broken, skipping test")

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposCommitsByOwnerByRepoByRef,
			mockedCommit,
		),
		mock.WithRequestMatchPages(
			mock.GetReposCommitsCheckRunsByOwnerByRepoByRef,
			github.ListCheckRunsResults{
				Total:     new(2),
				CheckRuns: mockedCheckRuns[:1],
			},
			github.ListCheckRunsResults{
				Total:     new(2),
				CheckRuns: mockedCheckRuns[1:],
			},
		),
	)

	log, testDataDir := testhelpers.Setup(t)
	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err, "error creating GitHub client")

	commit, err := Commit(
		log,
		client,
		testGatherOwner,
		testGatherRepo,
		mockedSha,
		ForceUpdate(),
		CustomDataFolder(testDataDir),
	)
	require.NoError(t, err, "error getting commit info")
	require.NotNil(t, commit, "commit should not be nil")
}

var (
	mockedSha    = "mocked-sha"
	mockedCommit = &github.RepositoryCommit{
		SHA: new(mockedSha),
		Commit: &github.Commit{
			SHA: new(mockedSha),
		},
	}
	mockedCheckRuns = []*github.CheckRun{
		{
			ID:          new(int64(1)),
			Name:        new("mocked-check-run-1"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   new(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
			CompletedAt: new(github.Timestamp{Time: time.Now()}),
			HTMLURL:     new("https://github.com/kalverra/octometrics/actions/runs/1"),
			HeadSHA:     new(mockedSha),
		},
		{
			ID:          new(int64(2)),
			Name:        new("mocked-check-run-2"),
			Status:      new("completed"),
			Conclusion:  new("failure"),
			StartedAt:   new(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
			CompletedAt: new(github.Timestamp{Time: time.Now()}),
			HTMLURL:     new("https://github.com/kalverra/octometrics/actions/runs/2"),
			HeadSHA:     new(mockedSha),
		},
	}
)
