package gather

import (
	"testing"
	"time"

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
			&github.Jobs{
				TotalCount: github.Ptr(len(mockJobs)),
				Jobs:       mockJobs[:2],
			},
			&github.Jobs{
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

	workflowRun, targetFile, err := WorkflowRun(
		log, client, gatherOwner, gatherRepo, mockWorkflowRun.GetID(), CustomDataFolder(testDataDir),
	)
	require.NoError(t, err, "error getting workflow run info")
	require.NotNil(t, workflowRun, "workflow run should not be nil")
	require.FileExists(t, targetFile, "workflow run file should exist")

	// Check if the file is written correctly
	readData, readFile, err := WorkflowRun(
		log, client, gatherOwner, gatherRepo, mockWorkflowRun.GetID(), CustomDataFolder(testDataDir),
	)
	require.NoError(t, err, "error reading workflow run info from file")
	require.NotNil(t, readData, "read workflow run data should not be nil")
	require.Equal(t, targetFile, readFile, "read workflow run file should match original written file")
	require.Equal(t, workflowRun, readData, "read workflow run data should match original data")

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

var (
	startTime       = time.Date(2025, 04, 20, 0, 1, 0, 0, time.UTC)
	createdTime     = time.Date(2025, 04, 20, 0, 0, 0, 0, time.UTC)
	endTime         = time.Date(2025, 04, 20, 1, 1, 0, 0, time.UTC)
	mockWorkflowRun = &github.WorkflowRun{
		ID:               github.Ptr(int64(1)),
		Name:             github.Ptr("mocked-workflow-run"),
		NodeID:           github.Ptr("mocked-node-id"),
		HeadBranch:       github.Ptr("mocked-head-branch"),
		HeadSHA:          github.Ptr("mocked-sha"),
		Path:             github.Ptr("mocked-workflow-path.yml"),
		RunNumber:        github.Ptr(1),
		Event:            github.Ptr("push"),
		DisplayTitle:     github.Ptr("mocked-display-title"),
		Status:           github.Ptr("completed"),
		Conclusion:       github.Ptr("success"),
		WorkflowID:       github.Ptr(int64(1)),
		CheckSuiteID:     github.Ptr(int64(1)),
		CheckSuiteNodeID: github.Ptr("mocked-check-suite-node-id"),
		URL:              github.Ptr("https://api.github.com/repos/kalverra/octometrics/actions/runs/1"),
		RunStartedAt:     github.Ptr(github.Timestamp{Time: startTime}),
		CreatedAt:        github.Ptr(github.Timestamp{Time: createdTime}),
		UpdatedAt:        github.Ptr(github.Timestamp{Time: endTime}),
		WorkflowURL:      github.Ptr("https://api.github.com/repos/kalverra/octometrics/actions/workflows/1"),
		Repository: &github.Repository{
			ID:       github.Ptr(int64(1)),
			Name:     github.Ptr("octometrics"),
			FullName: github.Ptr("kalverra/octometrics"),
		},
	}
	mockJobs = []*github.WorkflowJob{
		{
			ID:          github.Ptr(int64(1)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-1"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          github.Ptr(int64(2)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-2"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          github.Ptr(int64(3)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-3"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-3"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          github.Ptr(int64(4)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-4"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-3"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-4"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
	}
	mockWorkflowRunUsage = github.WorkflowRunUsage{
		Billable: &github.WorkflowRunBillMap{
			"UBUNTU": &github.WorkflowRunBill{
				TotalMS: github.Ptr(int64(0)),
				Jobs:    github.Ptr(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      github.Ptr(1),
						DurationMS: github.Ptr(int64(0)),
					},
				},
			},
			"UBUNTU_16_CORE": &github.WorkflowRunBill{
				TotalMS: github.Ptr(int64(endTime.Sub(startTime).Milliseconds() * 2)),
				Jobs:    github.Ptr(2),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      github.Ptr(2),
						DurationMS: github.Ptr(endTime.Sub(startTime).Milliseconds()),
					},
					{
						JobID:      github.Ptr(3),
						DurationMS: github.Ptr(endTime.Sub(startTime).Milliseconds()),
					},
				},
			},
			"UBUNTU_8_CORE_ARM": &github.WorkflowRunBill{
				TotalMS: github.Ptr(int64(endTime.Sub(startTime).Milliseconds())),
				Jobs:    github.Ptr(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      github.Ptr(4),
						DurationMS: github.Ptr(endTime.Sub(startTime).Milliseconds()),
					},
				},
			},
		},
	}
)
