package gather

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestIsInProgress(t *testing.T) {
	t.Parallel()

	// WorkflowRunData
	var wfNil *WorkflowRunData
	assert.False(t, wfNil.IsInProgress())

	wfInProg := &WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			Status: new("in_progress"),
		},
	}
	assert.True(t, wfInProg.IsInProgress())

	wfQueued := &WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			Status: new("queued"),
		},
	}
	assert.True(t, wfQueued.IsInProgress())

	wfCompleted := &WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			Status:     new("completed"),
			Conclusion: new("success"),
		},
	}
	assert.False(t, wfCompleted.IsInProgress())

	// CommitData
	var commitNil *CommitData
	assert.False(t, commitNil.IsInProgress())

	commitInProg := &CommitData{Conclusion: "in_progress"}
	assert.True(t, commitInProg.IsInProgress())

	commitEmpty := &CommitData{Conclusion: ""}
	assert.True(t, commitEmpty.IsInProgress())

	commitCompleted := &CommitData{
		Conclusion: "success",
		WorkflowRuns: []*WorkflowRunData{
			wfCompleted,
		},
	}
	assert.False(t, commitCompleted.IsInProgress())

	commitWithInProgWf := &CommitData{
		Conclusion: "success",
		WorkflowRuns: []*WorkflowRunData{
			wfInProg,
		},
	}
	assert.True(t, commitWithInProgWf.IsInProgress())

	// PullRequestData
	var prNil *PullRequestData
	assert.False(t, prNil.IsInProgress())

	prOpen := &PullRequestData{
		PullRequest: &github.PullRequest{
			State: new("open"),
		},
	}
	assert.True(t, prOpen.IsInProgress())

	prClosed := &PullRequestData{
		PullRequest: &github.PullRequest{
			State: new("closed"),
		},
		CommitData: []*CommitData{
			commitCompleted,
		},
	}
	assert.False(t, prClosed.IsInProgress())

	prClosedWithInProgCommit := &PullRequestData{
		PullRequest: &github.PullRequest{
			State: new("closed"),
		},
		CommitData: []*CommitData{
			commitInProg,
		},
	}
	assert.True(t, prClosedWithInProgCommit.IsInProgress())
}

func TestWorkflowRun_AutoUpdateInProgress(t *testing.T) {
	t.Parallel()

	log, testDataDir := testhelpers.Setup(t)
	runID := int64(888)

	// Pre-create an in-progress workflow run JSON on disk
	targetDir := filepath.Join(testDataDir, testGatherOwner, testGatherRepo, WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(targetDir, 0700))
	targetFile := filepath.Join(targetDir, fmt.Sprintf("%d.json", runID))

	staleData := &WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			ID:         new(int64(888)),
			Name:       new("test-workflow"),
			Status:     new("in_progress"),
			Conclusion: new(""),
			Repository: &github.Repository{
				Name:  new(testGatherRepo),
				Owner: &github.User{Login: new(testGatherOwner)},
			},
		},
	}
	staleBytes, err := json.Marshal(staleData)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(targetFile, staleBytes, 0600))

	// Mock GitHub client returning COMPLETED run
	completedRun := *staleData.WorkflowRun
	completedRun.Status = new("completed")
	completedRun.Conclusion = new("success")

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			completedRun,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{
				TotalCount: new(1),
				Jobs:       []*github.WorkflowJob{mockJobs[0]},
			},
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

	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	// Invalidate memory cache if any
	cacheKey := fmt.Sprintf("%s:cost=true", targetFile)
	workflowRunCache.Delete(cacheKey)

	fetched, file, err := WorkflowRun(
		t.Context(),
		log,
		client,
		testGatherOwner,
		testGatherRepo,
		runID,
		CustomDataFolder(testDataDir),
	)
	require.NoError(t, err)
	require.Equal(t, targetFile, file)
	require.Equal(t, "completed", fetched.GetStatus())
	require.Equal(t, "success", fetched.GetConclusion())

	// Verify disk file now reflects completed status
	diskData, err := readJSONFile[*WorkflowRunData](targetFile)
	require.NoError(t, err)
	require.Equal(t, "completed", diskData.GetStatus())
}

func TestWorkflowRun_TimestampSmartCache(t *testing.T) {
	t.Parallel()

	log, testDataDir := testhelpers.Setup(t)
	runID := int64(999)
	updatedTime := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	targetDir := filepath.Join(testDataDir, testGatherOwner, testGatherRepo, WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(targetDir, 0700))
	targetFile := filepath.Join(targetDir, fmt.Sprintf("%d.json", runID))

	existingData := &WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			ID:         new(int64(999)),
			Name:       new("test-workflow-timestamp"),
			Status:     new("completed"),
			Conclusion: new("success"),
			UpdatedAt:  new(github.Timestamp{Time: updatedTime}),
			Repository: &github.Repository{
				Name:  new(testGatherRepo),
				Owner: &github.User{Login: new(testGatherOwner)},
			},
		},
		Jobs: []*JobData{
			{
				WorkflowJob: mockJobs[0],
				Runner:      "UBUNTU",
				Cost:        10,
			},
		},
		Cost:         10,
		CostGathered: true,
	}
	existingBytes, err := json.Marshal(existingData)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(targetFile, existingBytes, 0600))

	// Mock GitHub returning same UpdatedAt
	remoteRun := *existingData.WorkflowRun

	var timingCalls atomic.Int32
	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			remoteRun,
		),
		mock.WithRequestMatchHandler(
			mock.GetReposActionsRunsTimingByOwnerByRepoByRunId,
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				timingCalls.Add(1)
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(github.WorkflowRunUsage{})
			}),
		),
	)

	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	// Invalidate memory cache to test disk timestamp comparison
	cacheKey := fmt.Sprintf("%s:cost=true", targetFile)
	workflowRunCache.Delete(cacheKey)

	fetched, file, err := WorkflowRun(
		t.Context(),
		log,
		client,
		testGatherOwner,
		testGatherRepo,
		runID,
		CustomDataFolder(testDataDir),
		ForceUpdate(), // Force update triggers API check, but timestamp match reuses jobs/cost!
	)
	require.NoError(t, err)
	require.Equal(t, targetFile, file)
	require.Equal(t, "completed", fetched.GetStatus())
	require.Equal(t, int64(10), fetched.GetCost())
}

func TestWorkflowRun_CorruptCacheRecovery(t *testing.T) {
	t.Parallel()

	log, testDataDir := testhelpers.Setup(t)
	runID := int64(7771)

	targetDir := filepath.Join(testDataDir, testGatherOwner, testGatherRepo, WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(targetDir, 0700))
	targetFile := filepath.Join(targetDir, fmt.Sprintf("%d.json", runID))

	// Write garbage JSON to cache file
	require.NoError(t, os.WriteFile(targetFile, []byte("{invalid json corrupt content..."), 0600))

	completedRun := *mockWorkflowRun
	completedRun.ID = new(int64(7771))

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			completedRun,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{
				TotalCount: new(1),
				Jobs:       []*github.WorkflowJob{mockJobs[0]},
			},
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

	client, err := NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	fetched, file, err := WorkflowRun(
		t.Context(),
		log,
		client,
		testGatherOwner,
		testGatherRepo,
		runID,
		CustomDataFolder(testDataDir),
	)
	require.NoError(t, err, "should recover from corrupt cache file via API")
	require.Equal(t, targetFile, file)
	require.Equal(t, runID, fetched.GetID())
}
