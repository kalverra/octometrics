package gather

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/kalverra/octometrics/internal/testhelpers"
	"github.com/kalverra/octometrics/monitor"
)

func TestGatherWorkflowRun_InProgress(t *testing.T) {
	t.Parallel()

	mockWorkflowRunInProgress := *mockWorkflowRun
	mockWorkflowRunInProgress.ID = new(int64(2))
	mockWorkflowRunInProgress.Status = new("in_progress")
	mockWorkflowRunInProgress.Conclusion = new("")
	mockWorkflowRunInProgress.UpdatedAt = new(github.Timestamp{Time: startTime})

	mockJobInProgress := *mockJobs[0]
	mockJobInProgress.Status = new("in_progress")
	mockJobInProgress.Conclusion = new("")
	mockJobInProgress.StartedAt = new(github.Timestamp{Time: startTime})
	mockJobInProgress.CompletedAt = nil
	mockJobInProgress.Steps = []*github.TaskStep{
		{
			Name:        new("mocked-step-1"),
			Status:      new("in_progress"),
			StartedAt:   new(github.Timestamp{Time: startTime}),
			CompletedAt: nil,
		},
	}

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			mockWorkflowRunInProgress,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{
				TotalCount: new(1),
				Jobs:       []*github.WorkflowJob{&mockJobInProgress},
			},
		),
	)

	log, testDataDir := testhelpers.Setup(t)
	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err, "error creating GitHub client")

	workflowRun, targetFile, err := WorkflowRun(
		t.Context(),
		log, client, testGatherOwner, testGatherRepo, mockWorkflowRunInProgress.GetID(), CustomDataFolder(testDataDir),
	)
	require.NoError(t, err, "error getting workflow run info")
	require.NotNil(t, workflowRun, "workflow run should not be nil")
	require.FileExists(t, targetFile, "workflow run file should exist")

	require.Equal(t, mockWorkflowRunInProgress.GetID(), workflowRun.GetID(), "workflow run ID should match")
	require.Equal(t, mockWorkflowRunInProgress.GetName(), workflowRun.GetName(), "workflow run name should match")
	require.Equal(t, mockWorkflowRunInProgress.GetStatus(), workflowRun.GetStatus(), "workflow run status should match")
	require.Equal(
		t,
		mockWorkflowRunInProgress.GetConclusion(),
		workflowRun.GetConclusion(),
		"workflow run conclusion should match",
	)
	require.Equal(
		t,
		mockWorkflowRunInProgress.GetUpdatedAt(),
		workflowRun.GetUpdatedAt(),
		"workflow run updated at should match",
	)
	require.Equal(
		t,
		mockWorkflowRunInProgress.GetCreatedAt(),
		workflowRun.GetCreatedAt(),
		"workflow run created at should match",
	)
}

func TestGatherWorkflowRun(t *testing.T) {
	t.Parallel()

	var (
		mockGitHubDownloadPath = "/mock/artifact/download"
		mockGitHubDownloadURL  = "https://api.github.com" + mockGitHubDownloadPath
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
		t.Context(),
		log,
		client,
		testGatherOwner,
		testGatherRepo,
		mockWorkflowRun.GetID(),
		CustomDataFolder(testDataDir),
		WithCost(),
	)
	require.NoError(t, err, "error getting workflow run info")
	require.NotNil(t, workflowRun, "workflow run should not be nil")
	require.FileExists(t, targetFile, "workflow run file should exist")

	readData, readFile, err := WorkflowRun(
		t.Context(),
		log,
		client,
		testGatherOwner,
		testGatherRepo,
		mockWorkflowRun.GetID(),
		CustomDataFolder(testDataDir),
		WithCost(),
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
					expectedCost = billableMinutes(jobRun.GetDurationMS()) * runnerCost
					break
				}
			}
		}

		if expectedRunner == "" {
			expectedRunner = getRunnerFromLabels(expectedJob.Labels)
		}

		require.Equal(t, expectedJob.GetName(), job.GetName(), "job name should match")
		require.Equal(t, expectedRunner, job.GetRunner(), "job runner should match")
		require.Equal(t, expectedCost, job.GetCost(), "job cost should match")
	}
}

func TestGetRunnerFromLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{
			name:   "empty labels",
			labels: []string{},
			want:   "",
		},
		{
			name:   "ubuntu label",
			labels: []string{"ubuntu-latest", "other"},
			want:   "ubuntu-latest",
		},
		{
			name:   "windows label",
			labels: []string{"windows-2022", "other"},
			want:   "windows-2022",
		},
		{
			name:   "macos label",
			labels: []string{"macos-13", "other"},
			want:   "macos-13",
		},
		{
			name:   "self-hosted label",
			labels: []string{"self-hosted", "linux", "x64"},
			want:   "self-hosted",
		},
		{
			name:   "no common label",
			labels: []string{"my-custom-runner", "another"},
			want:   "my-custom-runner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := getRunnerFromLabels(tt.labels)
			require.Equal(t, tt.want, got)
		})
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
			Labels:      []string{"ubuntu-latest"},
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
			Labels:      []string{"ubuntu-latest"},
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
			Labels:      []string{"ubuntu-latest"},
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
			Labels:      []string{"macos-latest"},
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

func TestJobsData_RetryOn502(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	var attempts atomic.Int32
	jobsResponse, err := json.Marshal(&github.Jobs{
		TotalCount: new(1),
		Jobs:       []*github.WorkflowJob{mockJobs[0]},
	})
	require.NoError(t, err)

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "50", r.URL.Query().Get("per_page"))
				if attempts.Add(1) == 1 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadGateway)
					_, _ = w.Write([]byte(`{"message":"Server Error"}`))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jobsResponse)
			}),
		),
	)

	oldDelay := workflowJobsRetryDelay
	workflowJobsRetryDelay = func(int) time.Duration { return 0 }
	t.Cleanup(func() { workflowJobsRetryDelay = oldDelay })

	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	jobs, err := jobsData(t.Context(), client, testGatherOwner, testGatherRepo, mockWorkflowRun.GetID())
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, int32(2), attempts.Load())
}

func TestReadAllLimited(t *testing.T) {
	t.Parallel()

	data, err := readAllLimited(strings.NewReader("hello"), 10)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))

	_, err = readAllLimited(strings.NewReader("hello"), 3)
	require.Error(t, err)
}

func TestSafeMonitorJSONLZipEntry(t *testing.T) {
	t.Parallel()

	require.True(
		t,
		safeMonitorJSONLZipEntry(&zip.File{FileHeader: zip.FileHeader{Name: "octometrics.monitor.log.jsonl"}}),
	)
	require.True(
		t,
		safeMonitorJSONLZipEntry(&zip.File{FileHeader: zip.FileHeader{Name: "job/octometrics.monitor.log.jsonl"}}),
	)
	require.False(
		t,
		safeMonitorJSONLZipEntry(
			&zip.File{FileHeader: zip.FileHeader{Name: "../../../tmp/octometrics.monitor.log.jsonl"}},
		),
	)
	require.False(
		t,
		safeMonitorJSONLZipEntry(&zip.File{FileHeader: zip.FileHeader{Name: "/abs/octometrics.monitor.log.jsonl"}}),
	)
	require.False(
		t,
		safeMonitorJSONLZipEntry(&zip.File{FileHeader: zip.FileHeader{Name: `win\octometrics.monitor.log.jsonl`}}),
	)
	require.False(t, safeMonitorJSONLZipEntry(&zip.File{FileHeader: zip.FileHeader{Name: "wrong.log.jsonl"}}))
}

func TestProcessJobs_RunsOnAlwaysFetchLogs(t *testing.T) {
	t.Parallel()

	logCalled := false
	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposActionsJobsLogsByOwnerByRepoByJobId,
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				logCalled = true
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	log, _ := testhelpers.Setup(t)
	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	now := time.Now()
	// Skipped job with runs-on label
	skippedJob := &github.WorkflowJob{
		ID:          new(int64(101)),
		Name:        new("skipped-job"),
		Status:      new("completed"),
		Conclusion:  new("skipped"),
		StartedAt:   &github.Timestamp{Time: now},
		CompletedAt: &github.Timestamp{Time: now},
		Labels:      []string{"runs-on=123/cpu=4/ram=16/family=c7i/spot=true"},
	}

	// Nonzero duration runs-on job that can be priced by label estimator
	pricedJob := &github.WorkflowJob{
		ID:          new(int64(102)),
		Name:        new("priced-job"),
		Status:      new("completed"),
		Conclusion:  new("success"),
		StartedAt:   &github.Timestamp{Time: now},
		CompletedAt: &github.Timestamp{Time: now.Add(2 * time.Minute)},
		Labels:      []string{"2cpu-linux-x64"},
	}

	data := &WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			Status: new("completed"),
		},
	}

	log, tempDir := testhelpers.Setup(t)
	processJobs(
		t.Context(),
		log,
		client,
		"owner",
		"repo",
		data,
		[]*github.WorkflowJob{skippedJob, pricedJob},
		nil,
		true,
		tempDir,
	)

	assert.True(
		t,
		logCalled,
		"fetchRunsOnCostFromLogs should be called for runs-on jobs even when label estimate is available",
	)
	assert.Len(t, data.Jobs, 2)
	assert.Equal(t, int64(0), data.Jobs[0].Cost)
	assert.Positive(t, data.Jobs[1].Cost)
	assert.True(t, data.Jobs[1].CostEstimate, "cost should be estimate when log fetch fails to return exact cost")
}

func TestWorkflowRun_ReadCacheAndSingleflight(t *testing.T) {
	t.Parallel()

	var fetchCount atomic.Int32
	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fetchCount.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id": 99999, "status": "completed", "conclusion": "success"}`))
			}),
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{TotalCount: new(0), Jobs: []*github.WorkflowJob{}},
		),
		mock.WithRequestMatch(
			mock.GetReposActionsRunsTimingByOwnerByRepoByRunId,
			&github.WorkflowRunUsage{},
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsArtifactsByOwnerByRepoByRunId,
			&github.ArtifactList{TotalCount: new(int64(0)), Artifacts: []*github.Artifact{}},
		),
	)

	log, tempDir := testhelpers.Setup(t)
	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	const n = 10
	var eg errgroup.Group
	for range n {
		eg.Go(func() error {
			_, _, err := WorkflowRun(t.Context(), log, client, "owner", "repo", 99999, CustomDataFolder(tempDir))
			return err
		})
	}
	require.NoError(t, eg.Wait())

	assert.Equal(
		t,
		int32(1),
		fetchCount.Load(),
		"GitHub API should only be called once due to singleflight and read cache",
	)

	// Call again, should hit read cache directly
	_, _, err = WorkflowRun(t.Context(), log, client, "owner", "repo", 99999, CustomDataFolder(tempDir))
	require.NoError(t, err)
	assert.Equal(t, int32(1), fetchCount.Load(), "Cache hit should not trigger additional GitHub API call")
}

func TestBuildJobBillingIndex_KnownRunners(t *testing.T) {
	t.Parallel()

	usage := &github.WorkflowRunUsage{
		Billable: &github.WorkflowRunBillMap{
			"UBUNTU": &github.WorkflowRunBill{
				JobRuns: []*github.WorkflowRunJobRun{
					{JobID: new(1), DurationMS: new(int64(60000))},
				},
			},
			"MACOS": &github.WorkflowRunBill{
				JobRuns: []*github.WorkflowRunJobRun{
					{JobID: new(2), DurationMS: new(int64(60000))},
				},
			},
			"WINDOWS": &github.WorkflowRunBill{
				JobRuns: []*github.WorkflowRunJobRun{
					{JobID: new(3), DurationMS: new(int64(60000))},
				},
			},
		},
	}

	index := buildJobBillingIndex(usage)

	runner, cost, err := calculateJobRunBilling(1, index)
	require.NoError(t, err)
	require.Equal(t, "UBUNTU", runner)
	require.Equal(t, int64(8), cost, "ubuntu cost should be 0.8 cents per minute")

	runner, cost, err = calculateJobRunBilling(2, index)
	require.NoError(t, err)
	require.Equal(t, "MACOS", runner)
	require.Positive(t, cost, "macOS job should have a non-zero cost")

	runner, cost, err = calculateJobRunBilling(3, index)
	require.NoError(t, err)
	require.Equal(t, "WINDOWS", runner)
	require.Positive(t, cost, "Windows job should have a non-zero cost")
}

func TestWorkflowRun_CacheKeyOptionAware(t *testing.T) {
	t.Parallel()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id": 88888, "status": "completed", "conclusion": "success"}`))
			}),
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{TotalCount: new(0), Jobs: []*github.WorkflowJob{}},
		),
		mock.WithRequestMatch(
			mock.GetReposActionsRunsTimingByOwnerByRepoByRunId,
			&github.WorkflowRunUsage{Billable: &github.WorkflowRunBillMap{}},
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsArtifactsByOwnerByRepoByRunId,
			&github.ArtifactList{TotalCount: new(int64(0)), Artifacts: []*github.Artifact{}},
		),
	)

	log, tempDir := testhelpers.Setup(t)
	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	wfWithoutCost, _, err := WorkflowRun(
		t.Context(),
		log,
		client,
		"owner",
		"repo",
		88888,
		CustomDataFolder(tempDir),
		WithoutCost(),
		ForceUpdate(),
	)
	require.NoError(t, err)
	assert.False(t, wfWithoutCost.CostGathered)

	wfWithCost, _, err := WorkflowRun(t.Context(), log, client, "owner", "repo", 88888, CustomDataFolder(tempDir))
	require.NoError(t, err)
	assert.True(t, wfWithCost.CostGathered)
}
