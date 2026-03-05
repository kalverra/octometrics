package gather

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
	"github.com/kalverra/octometrics/monitor"
)

func TestGatherWorkflowRun(t *testing.T) {
	t.Parallel()

	var (
		mockGitHubDownloadPath = "/mock/artifact/download"
		mockGitHubDownloadURL  = "http://api.github.com" + mockGitHubDownloadPath
		mockZipFile            = filepath.Join(testDataDir, fmt.Sprintf("%s.zip", monitor.DataFile))
	)
	require.FileExists(t, mockZipFile, "test zip file should exist")
	require.NotEmpty(t, mockZipFile, "test zip file should not be empty")

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			mockWorkflowRun,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsArtifactsByOwnerByRepoByRunId,
			&github.ArtifactList{
				TotalCount: new(int64(len(mockArtifacts))),
				Artifacts:  mockArtifacts[:2],
			},
			&github.ArtifactList{
				TotalCount: new(int64(len(mockArtifacts))),
				Artifacts:  mockArtifacts[2:],
			},
		),
		mock.WithRequestMatchHandler(
			mock.GetReposActionsArtifactsByOwnerByRepoByArtifactIdByArchiveFormat,
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", mockGitHubDownloadURL)
				w.WriteHeader(http.StatusFound)
			}),
		),
		mock.WithRequestMatchHandler(
			mock.EndpointPattern{
				Method:  "GET",
				Pattern: mockGitHubDownloadPath,
			},
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/zip")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.zip", monitor.DataFile))
				http.ServeFile(w, r, mockZipFile)
			}),
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{
				TotalCount: new(len(mockJobs)),
				Jobs:       mockJobs[:2],
			},
			&github.Jobs{
				TotalCount: new(4),
				Jobs:       mockJobs[2:],
			},
		),
		mock.WithRequestMatch(
			mock.GetReposActionsRunsTimingByOwnerByRepoByRunId,
			mockWorkflowRunUsage,
		),
	)

	log, testDataDir := testhelpers.Setup(t)
	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err, "error creating GitHub client")

	workflowRun, targetFile, err := WorkflowRun(
		log, client, testGatherOwner, testGatherRepo, mockWorkflowRun.GetID(), CustomDataFolder(testDataDir),
	)
	require.NoError(t, err, "error getting workflow run info")
	require.NotNil(t, workflowRun, "workflow run should not be nil")
	require.FileExists(t, targetFile, "workflow run file should exist")

	readData, readFile, err := WorkflowRun(
		log, client, testGatherOwner, testGatherRepo, mockWorkflowRun.GetID(), CustomDataFolder(testDataDir),
	)

	// Check if the file is written correctly
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
		ID:               new(int64(1)),
		Name:             new("mocked-workflow-run"),
		NodeID:           new("mocked-node-id"),
		HeadBranch:       new("mocked-head-branch"),
		HeadSHA:          new("mocked-sha"),
		Path:             new("mocked-workflow-path.yml"),
		RunNumber:        new(1),
		Event:            new("push"),
		DisplayTitle:     new("mocked-display-title"),
		Status:           new("completed"),
		Conclusion:       new("success"),
		WorkflowID:       new(int64(1)),
		CheckSuiteID:     new(int64(1)),
		CheckSuiteNodeID: new("mocked-check-suite-node-id"),
		URL:              new("https://api.github.com/repos/kalverra/octometrics/actions/runs/1"),
		RunStartedAt:     new(github.Timestamp{Time: startTime}),
		CreatedAt:        new(github.Timestamp{Time: createdTime}),
		UpdatedAt:        new(github.Timestamp{Time: endTime}),
		WorkflowURL:      new("https://api.github.com/repos/kalverra/octometrics/actions/workflows/1"),
		Repository: &github.Repository{
			ID:       new(int64(1)),
			Name:     new("octometrics"),
			FullName: new("kalverra/octometrics"),
		},
	}
	mockJobs = []*github.WorkflowJob{
		{
			ID:          new(int64(1)),
			RunID:       new(int64(1)),
			HeadBranch:  new("mocked-head-branch"),
			HeadSHA:     new("mocked-sha"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   new(github.Timestamp{Time: createdTime}),
			StartedAt:   new(github.Timestamp{Time: startTime}),
			CompletedAt: new(github.Timestamp{Time: endTime}),
			Name:        new("mocked-job-1"),
			Steps: []*github.TaskStep{
				{
					Name:        new("mocked-step-1"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          new(int64(2)),
			RunID:       new(int64(1)),
			HeadBranch:  new("mocked-head-branch"),
			HeadSHA:     new("mocked-sha"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   new(github.Timestamp{Time: createdTime}),
			StartedAt:   new(github.Timestamp{Time: startTime}),
			CompletedAt: new(github.Timestamp{Time: endTime}),
			Name:        new("mocked-job-2"),
			Steps: []*github.TaskStep{
				{
					Name:        new("mocked-step-1"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
				{
					Name:        new("mocked-step-2"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          new(int64(3)),
			RunID:       new(int64(1)),
			HeadBranch:  new("mocked-head-branch"),
			HeadSHA:     new("mocked-sha"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   new(github.Timestamp{Time: createdTime}),
			StartedAt:   new(github.Timestamp{Time: startTime}),
			CompletedAt: new(github.Timestamp{Time: endTime}),
			Name:        new("mocked-job-3"),
			Steps: []*github.TaskStep{
				{
					Name:        new("mocked-step-1"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
				{
					Name:        new("mocked-step-2"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
				{
					Name:        new("mocked-step-3"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          new(int64(4)),
			RunID:       new(int64(1)),
			HeadBranch:  new("mocked-head-branch"),
			HeadSHA:     new("mocked-sha"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   new(github.Timestamp{Time: createdTime}),
			StartedAt:   new(github.Timestamp{Time: startTime}),
			CompletedAt: new(github.Timestamp{Time: endTime}),
			Name:        new("mocked-job-4"),
			Steps: []*github.TaskStep{
				{
					Name:        new("mocked-step-1"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
				{
					Name:        new("mocked-step-2"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
				{
					Name:        new("mocked-step-3"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
				{
					Name:        new("mocked-step-4"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   new(github.Timestamp{Time: startTime}),
					CompletedAt: new(github.Timestamp{Time: endTime}),
				},
			},
		},
	}
	mockWorkflowRunUsage = github.WorkflowRunUsage{
		Billable: &github.WorkflowRunBillMap{
			"UBUNTU": &github.WorkflowRunBill{
				TotalMS: new(int64(0)),
				Jobs:    new(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      new(1),
						DurationMS: new(int64(0)),
					},
				},
			},
			"UBUNTU_16_CORE": &github.WorkflowRunBill{
				TotalMS: new(int64(endTime.Sub(startTime).Milliseconds() * 2)),
				Jobs:    new(2),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      new(2),
						DurationMS: new(endTime.Sub(startTime).Milliseconds()),
					},
					{
						JobID:      new(3),
						DurationMS: new(endTime.Sub(startTime).Milliseconds()),
					},
				},
			},
			"UBUNTU_8_CORE_ARM": &github.WorkflowRunBill{
				TotalMS: new(int64(endTime.Sub(startTime).Milliseconds())),
				Jobs:    new(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      new(4),
						DurationMS: new(endTime.Sub(startTime).Milliseconds()),
					},
				},
			},
		},
	}
	mockArtifacts = []*github.Artifact{
		{
			ID:          new(int64(1)),
			Name:        new(monitor.DataFile),
			SizeInBytes: new(int64(1000)),
		},
		{
			ID:          new(int64(2)),
			Name:        new("bad-artifact-1"),
			SizeInBytes: new(int64(200)),
		},
		{
			ID:          new(int64(3)),
			Name:        new("bad-artifact-2"),
			SizeInBytes: new(int64(300)),
		},
		{
			ID:          new(int64(4)),
			Name:        new("bad-artifact-3"),
			SizeInBytes: new(int64(400)),
		},
	}
)
