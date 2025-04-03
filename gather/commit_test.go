package gather

import (
	"testing"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"
)

func TestGatherCommit(t *testing.T) {
	t.Parallel()
	t.Skip("skipping test, issues not fixed yet")

	mockedSha := "mocked-sha"
	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposCommitsByOwnerByRepoByRef,
			github.RepositoryCommit{
				SHA: github.Ptr(mockedSha),
				Commit: &github.Commit{
					SHA: github.Ptr(mockedSha),
				},
			},
		),
		mock.WithRequestMatchPages(
			mock.GetReposCommitsCheckRunsByOwnerByRepoByRef,
			[]github.CheckRun{
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
			},
			[]github.CheckRun{
				{
					ID:          github.Ptr(int64(3)),
					Name:        github.Ptr("mocked-check-run-3"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
					CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
					HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/3"),
					HeadSHA:     github.Ptr(mockedSha),
				},
				{
					ID:          github.Ptr(int64(4)),
					Name:        github.Ptr("mocked-check-run-4"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("failure"),
					StartedAt:   github.Ptr(github.Timestamp{Time: time.Now().Add(-time.Hour)}),
					CompletedAt: github.Ptr(github.Timestamp{Time: time.Now()}),
					HTMLURL:     github.Ptr("https://github.com/kalverra/octometrics/actions/runs/4"),
					HeadSHA:     github.Ptr(mockedSha),
				},
			},
		),
	)

	log, testDataDir, client := testSetup(t, mockedHTTPClient)

	commit, err := Commit(log, client, gatherOwner, gatherRepo, mockedSha, ForceUpdate(), CustomDataFolder(testDataDir))
	require.NoError(t, err, "error getting commit info")
	require.NotNil(t, commit, "commit should not be nil")
}
