package observe

import (
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/testhelpers"
)

var (
	testStartTime = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	testEndTime   = testStartTime.Add(5 * time.Minute)
)

func TestWorkflowRun_CostPropagated(t *testing.T) {
	t.Parallel()

	mockWorkflowRun := &github.WorkflowRun{
		ID:           new(int64(12345)),
		Name:         new("test-workflow"),
		Status:       new("completed"),
		Conclusion:   new("success"),
		Event:        new("push"),
		HeadSHA:      new("abc123"),
		HTMLURL:      new("https://github.com/owner/repo/actions/runs/12345"),
		RunStartedAt: &github.Timestamp{Time: testStartTime},
		CreatedAt:    &github.Timestamp{Time: testStartTime},
		UpdatedAt:    &github.Timestamp{Time: testEndTime},
		Repository: &github.Repository{
			Owner: &github.User{Login: new("owner")},
			Name:  new("repo"),
		},
		Actor: &github.User{Login: new("testuser")},
	}

	mockJob := &github.WorkflowJob{
		ID:          new(int64(1)),
		Name:        new("build"),
		Status:      new("completed"),
		Conclusion:  new("success"),
		StartedAt:   &github.Timestamp{Time: testStartTime},
		CompletedAt: &github.Timestamp{Time: testEndTime},
		RunAttempt:  new(int64(1)),
	}

	// Mock billing: 5 minutes on UBUNTU = 5 * 8 tenths-of-cent = 40
	mockUsage := &github.WorkflowRunUsage{
		Billable: &github.WorkflowRunBillMap{
			"UBUNTU": &github.WorkflowRunBill{
				TotalMS: new(testEndTime.Sub(testStartTime).Milliseconds()),
				Jobs:    new(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      new(1),
						DurationMS: new(testEndTime.Sub(testStartTime).Milliseconds()),
					},
				},
			},
		},
	}

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposActionsRunsByOwnerByRepoByRunId,
			mockWorkflowRun,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsJobsByOwnerByRepoByRunId,
			&github.Jobs{
				TotalCount: new(1),
				Jobs:       []*github.WorkflowJob{mockJob},
			},
		),
		mock.WithRequestMatch(
			mock.GetReposActionsRunsTimingByOwnerByRepoByRunId,
			mockUsage,
		),
		mock.WithRequestMatchPages(
			mock.GetReposActionsRunsArtifactsByOwnerByRepoByRunId,
			&github.ArtifactList{
				TotalCount: new(int64(0)),
				Artifacts:  []*github.Artifact{},
			},
		),
	)

	log, testDataDir := testhelpers.Setup(t)
	client, err := gather.NewGitHubClient(log, "mock-token", mockedHTTPClient.Transport)
	require.NoError(t, err)

	obs, err := WorkflowRun(
		log,
		client,
		"owner",
		"repo",
		12345,
		WithGatherOptions(
			gather.CustomDataFolder(testDataDir),
			gather.WithCost(),
		),
	)
	require.NoError(t, err)
	require.NotNil(t, obs)

	assert.Equal(t, "12345", obs.ID)
	assert.Equal(t, "test-workflow", obs.Name)
	assert.Equal(t, "owner", obs.Owner)
	assert.Equal(t, "repo", obs.Repo)
	assert.Equal(t, "success", obs.State)
	assert.Equal(t, "testuser", obs.Actor)
	assert.Equal(t, "workflow_run", obs.DataType)

	// 5 minutes on UBUNTU ($0.008/min) = 5 * 8 = 40 tenths-of-cent = $0.04
	expectedCost := int64(40)
	assert.Equal(t, expectedCost, obs.Cost, "Observation.Cost should match workflow run cost")
}

func TestWorkflowRunTimelineData_CostPropagated(t *testing.T) {
	t.Parallel()

	wfRun := &github.WorkflowRun{
		ID:           new(int64(1)),
		Name:         new("wf"),
		Event:        new("push"),
		RunStartedAt: &github.Timestamp{Time: testStartTime},
		Repository: &github.Repository{
			Name:  new("repo"),
			Owner: &github.User{Login: new("owner")},
		},
	}
	job := &github.WorkflowJob{
		ID:          new(int64(10)),
		Name:        new("build"),
		Status:      new("completed"),
		Conclusion:  new("success"),
		StartedAt:   &github.Timestamp{Time: testStartTime},
		CompletedAt: &github.Timestamp{Time: testEndTime},
	}
	jobData := &gather.JobData{
		WorkflowJob:  job,
		Cost:         40,
		CostGathered: true,
	}
	wfData := &gather.WorkflowRunData{
		WorkflowRun: wfRun,
		Jobs:        []*gather.JobData{jobData},
	}

	timeline, err := buildWorkflowRunTimelineData(wfData)
	require.NoError(t, err)
	require.Len(t, timeline.Items, 1)
	assert.Equal(t, int64(40), timeline.Items[0].Cost, "TimelineItem.Cost should be propagated from job data")
	assert.True(t, timeline.Items[0].CostGathered, "TimelineItem.CostGathered should be propagated from job data")
}
