package gather

import (
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"
)

func TestGatherWorkflowRun(t *testing.T) {
	t.Parallel()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			mockWorkflowRun,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			github.Jobs{
				TotalCount: github.Ptr(len(mockJobs)),
				Jobs:       mockJobs[:2],
			},
			github.Jobs{
				TotalCount: github.Ptr(4),
				Jobs:       mockJobs[2:],
			},
		),
		mock.WithRequestMatch(
			mock.GetReposActionsRunsTimingByOwnerByRepoByRunId,
			mockWorkflowRunUsage,
		),
	)

	log, testDataDir, client := testSetup(t, mockedHTTPClient)

	workflowRun, err := WorkflowRun(log, client, gatherOwner, gatherRepo, mockWorkflowRun.GetID(), ForceUpdate(), CustomDataFolder(testDataDir))
	require.NoError(t, err, "error getting workflow run info")
	require.NotNil(t, workflowRun, "workflow run should not be nil")
	require.Equal(t, mockWorkflowRun.GetID(), workflowRun.GetID(), "workflow run ID should match")
	require.NotNil(t, workflowRun.Jobs, "workflow run jobs should not be nil")
	require.NotNil(t, workflowRun.Usage, "workflow run usage should not be nil")
	require.Len(t, workflowRun.Jobs, len(mockJobs), "workflow run should have 4 jobs")
	require.Equal(t, endTime, workflowRun.GetRunCompletedAt(), "workflow run completed at should match")

	require.NotNil(t, mockWorkflowRunUsage.GetBillable(), "need mock workflow run usage billable data for assertions")
	billableData := *mockWorkflowRunUsage.GetBillable()
	for jobIndex, job := range workflowRun.Jobs {
		require.NotNil(t, job, "job should not be nil")
		require.NotNil(t, job.WorkflowJob, "job workflow job should not be nil")
		require.NotNil(t, job.GetRunner(), "job runner should not be nil")
		require.NotNil(t, job.GetCost(), "job cost should not be nil")

		expectedJob := mockJobs[jobIndex]
		var (
			expectedRunner string
			expectedCost   int64
		)

		for runner, data := range billableData {
			for _, jobRun := range data.JobRuns {
				if int64(jobRun.GetJobID()) == job.GetID() {
					expectedRunner = runner
					runnerCost, ok := rateByRunner[runner]
					require.True(t, ok, "runner '%s' not found in rateByRunner", runner)
					expectedCost = jobRun.GetDurationMS() / 1000 / 60 * runnerCost
					break
				}
			}
		}

		require.Equal(t, expectedJob.GetName(), job.GetName(), "job name should match")
		require.Equal(t, expectedRunner, job.GetRunner(), "job runner should match")
		require.Equal(t, expectedCost, job.GetCost(), "job cost should match")
	}
}
