package gather

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestGroupAndComputeDurations(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	t.Run("groups runs by HeadSHA", func(t *testing.T) {
		t.Parallel()

		runs := []*github.WorkflowRun{
			makeWorkflowRun(1, 100, "sha-a", "CI", "pull_request", "completed", "success", 1,
				baseTime, baseTime.Add(5*time.Minute), baseTime.Add(10*time.Minute)),
			makeWorkflowRun(2, 200, "sha-a", "Lint", "pull_request", "completed", "success", 1,
				baseTime, baseTime.Add(2*time.Minute), baseTime.Add(8*time.Minute)),
			makeWorkflowRun(3, 100, "sha-b", "CI", "pull_request", "completed", "success", 1,
				baseTime, baseTime.Add(1*time.Minute), baseTime.Add(20*time.Minute)),
		}

		commits := groupAndComputeDurations(runs)
		require.Len(t, commits, 2)

		commitMap := make(map[string]*CommitSurvey)
		for _, c := range commits {
			commitMap[c.SHA] = c
		}

		require.Contains(t, commitMap, "sha-a")
		require.Contains(t, commitMap, "sha-b")

		assert.Len(t, commitMap["sha-a"].WorkflowRuns, 2)
		assert.Len(t, commitMap["sha-b"].WorkflowRuns, 1)
	})

	t.Run("deduplicates by workflow ID keeping latest attempt", func(t *testing.T) {
		t.Parallel()

		runs := []*github.WorkflowRun{
			makeWorkflowRun(1, 100, "sha-a", "CI", "pull_request", "completed", "failure", 1,
				baseTime, baseTime.Add(1*time.Minute), baseTime.Add(5*time.Minute)),
			makeWorkflowRun(2, 100, "sha-a", "CI", "pull_request", "completed", "success", 2,
				baseTime.Add(10*time.Minute), baseTime.Add(11*time.Minute), baseTime.Add(15*time.Minute)),
		}

		commits := groupAndComputeDurations(runs)
		require.Len(t, commits, 1)
		assert.Len(t, commits[0].WorkflowRuns, 1, "should keep only latest attempt")
		assert.Equal(t, 2, commits[0].WorkflowRuns[0].RunAttempt)
		assert.Equal(t, "success", commits[0].Conclusion)
	})

	t.Run("computes correct wall-clock duration", func(t *testing.T) {
		t.Parallel()

		runs := []*github.WorkflowRun{
			makeWorkflowRun(1, 100, "sha-a", "CI", "pull_request", "completed", "success", 1,
				baseTime, baseTime, baseTime.Add(10*time.Minute)),
			makeWorkflowRun(2, 200, "sha-a", "Lint", "pull_request", "completed", "success", 1,
				baseTime, baseTime.Add(1*time.Minute), baseTime.Add(6*time.Minute)),
		}

		commits := groupAndComputeDurations(runs)
		require.Len(t, commits, 1)
		// Wall clock: min(RunStartedAt) = baseTime, max(UpdatedAt) = baseTime+10m
		assert.Equal(t, 10*time.Minute, commits[0].Duration)
	})

	t.Run("skips non-completed runs", func(t *testing.T) {
		t.Parallel()

		runs := []*github.WorkflowRun{
			makeWorkflowRun(1, 100, "sha-a", "CI", "pull_request", "in_progress", "", 1,
				baseTime, baseTime, baseTime.Add(5*time.Minute)),
		}

		commits := groupAndComputeDurations(runs)
		assert.Empty(t, commits)
	})

	t.Run("sets failure conclusion when any run failed", func(t *testing.T) {
		t.Parallel()

		runs := []*github.WorkflowRun{
			makeWorkflowRun(1, 100, "sha-a", "CI", "pull_request", "completed", "success", 1,
				baseTime, baseTime, baseTime.Add(10*time.Minute)),
			makeWorkflowRun(2, 200, "sha-a", "Lint", "pull_request", "completed", "failure", 1,
				baseTime, baseTime.Add(1*time.Minute), baseTime.Add(6*time.Minute)),
		}

		commits := groupAndComputeDurations(runs)
		require.Len(t, commits, 1)
		assert.Equal(t, "failure", commits[0].Conclusion)
	})

	t.Run("handles empty input", func(t *testing.T) {
		t.Parallel()

		commits := groupAndComputeDurations(nil)
		assert.Empty(t, commits)
	})
}

func TestComputePercentiles(t *testing.T) {
	t.Parallel()

	t.Run("correctly identifies p50 p75 p95", func(t *testing.T) {
		t.Parallel()

		commits := make([]*CommitSurvey, 100)
		for i := range 100 {
			commits[i] = &CommitSurvey{
				SHA:      commitSHA(i),
				Duration: time.Duration(i+1) * time.Minute,
			}
		}

		result := computePercentiles(commits)
		require.Contains(t, result, "p50")
		require.Contains(t, result, "p75")
		require.Contains(t, result, "p95")

		assert.Equal(t, 50*time.Minute, result["p50"].Duration)
		assert.Equal(t, 75*time.Minute, result["p75"].Duration)
		assert.Equal(t, 95*time.Minute, result["p95"].Duration)
	})

	t.Run("single commit returns it for all percentiles", func(t *testing.T) {
		t.Parallel()

		commits := []*CommitSurvey{
			{SHA: "only-one", Duration: 5 * time.Minute},
		}
		result := computePercentiles(commits)
		assert.Equal(t, "only-one", result["p50"].SHA)
		assert.Equal(t, "only-one", result["p75"].SHA)
		assert.Equal(t, "only-one", result["p95"].SHA)
	})

	t.Run("empty input returns empty map", func(t *testing.T) {
		t.Parallel()

		result := computePercentiles(nil)
		assert.Empty(t, result)
	})

	t.Run("two commits", func(t *testing.T) {
		t.Parallel()

		commits := []*CommitSurvey{
			{SHA: "fast", Duration: 3 * time.Minute},
			{SHA: "slow", Duration: 10 * time.Minute},
		}
		result := computePercentiles(commits)
		assert.Equal(t, "fast", result["p50"].SHA)
		assert.Equal(t, "slow", result["p75"].SHA)
		assert.Equal(t, "slow", result["p95"].SHA)
	})
}

func TestSurveyFileName(t *testing.T) {
	t.Parallel()

	since := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2025, 6, 7, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, "pull_request_2025-06-01_2025-06-07.json", surveyFileName("pull_request", since, until))
	assert.Equal(t, "all_2025-06-01_2025-06-07.json", surveyFileName("", since, until))
}

func TestSurveyResultSerialization(t *testing.T) {
	t.Parallel()

	log, testDir := testhelpers.Setup(t)
	_ = log

	result := &SurveyResult{
		Owner:     "test-owner",
		Repo:      "test-repo",
		Event:     "pull_request",
		Since:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Until:     time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC),
		TotalRuns: 42,
		Commits: []*CommitSurvey{
			{
				SHA:       "abc123",
				Duration:  10 * time.Minute,
				StartTime: time.Date(2025, 1, 3, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 1, 3, 10, 10, 0, 0, time.UTC),
			},
		},
		Percentiles: map[string]*CommitSurvey{
			"p50": {SHA: "abc123", Duration: 10 * time.Minute},
		},
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	targetFile := filepath.Join(testDir, "survey.json")
	require.NoError(t, os.WriteFile(targetFile, data, 0600))

	//nolint:gosec // Test file
	loaded, err := os.ReadFile(targetFile)
	require.NoError(t, err)

	var restored SurveyResult
	require.NoError(t, json.Unmarshal(loaded, &restored))

	assert.Equal(t, result.Owner, restored.Owner)
	assert.Equal(t, result.Repo, restored.Repo)
	assert.Equal(t, result.TotalRuns, restored.TotalRuns)
	assert.Len(t, restored.Commits, 1)
	assert.Equal(t, "abc123", restored.Commits[0].SHA)
	assert.Equal(t, "abc123", restored.Percentiles["p50"].SHA)
}

// makeWorkflowRun creates a github.WorkflowRun with the given parameters for testing.
func makeWorkflowRun(
	id, workflowID int64,
	headSHA, name, event, status, conclusion string,
	runAttempt int,
	createdAt, runStartedAt, updatedAt time.Time,
) *github.WorkflowRun {
	return &github.WorkflowRun{
		ID:           &id,
		WorkflowID:   &workflowID,
		HeadSHA:      &headSHA,
		Name:         &name,
		Event:        &event,
		Status:       &status,
		Conclusion:   &conclusion,
		RunAttempt:   &runAttempt,
		CreatedAt:    &github.Timestamp{Time: createdAt},
		RunStartedAt: &github.Timestamp{Time: runStartedAt},
		UpdatedAt:    &github.Timestamp{Time: updatedAt},
	}
}

func commitSHA(i int) string {
	//nolint:gosec // Test data
	return "sha-" + string(rune('a'+i))
}
