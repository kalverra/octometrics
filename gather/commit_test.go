package gather

import (
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
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
				Total:     github.Ptr(2),
				CheckRuns: mockedCheckRuns[:1],
			},
			github.ListCheckRunsResults{
				Total:     github.Ptr(2),
				CheckRuns: mockedCheckRuns[1:],
			},
		),
	)

	log, testDataDir := testhelpers.Setup(t)
	client := github.NewClient(mockedHTTPClient)

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
		SHA: github.Ptr(mockedSha),
		Commit: &github.Commit{
			SHA: github.Ptr(mockedSha),
		},
	}
	mockedCheckRuns = []*github.CheckRun{
		{
			ID:          github.Ptr(int64(1)),
			Name:        github.Ptr("mocked-check-run-1"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
			CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
			HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/1"),
			HeadSHA:     github.Ptr(mockedSha),
		},
		{
			ID:          github.Ptr(int64(2)),
			Name:        github.Ptr("mocked-check-run-2"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("failure"),
			StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
			CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
			HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/2"),
			HeadSHA:     github.Ptr(mockedSha),
		},
	}
)
