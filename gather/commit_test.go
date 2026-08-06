package gather

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

type mockRoundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

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

func TestCommit_FallbackToParentForMergeCommit(t *testing.T) {
	t.Parallel()

	log, testDataDir := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path

				// 1. GetCommit for merge-sha
				if path == "/repos/o/r/commits/merge-sha" {
					respJSON := `{
						"sha": "merge-sha",
						"parents": [
							{"sha": "parent1-sha"},
							{"sha": "parent2-sha"}
						]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}

				// 2. check runs for merge-sha (empty)
				if path == "/repos/o/r/commits/merge-sha/check-runs" {
					respJSON := `{
						"total_count": 0,
						"check_runs": []
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}

				// 3. check runs for parent2-sha (has check runs)
				if path == "/repos/o/r/commits/parent2-sha/check-runs" {
					respJSON := `{
						"total_count": 1,
						"check_runs": [
							{
								"id": 1,
								"name": "check-run-1",
								"status": "completed",
								"conclusion": "success",
								"html_url": "https://github.com/o/r/actions/runs/123",
								"head_sha": "parent2-sha"
							}
						]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}

				// 4. workflow run 123
				if path == "/repos/o/r/actions/runs/123" {
					respJSON := `{
						"id": 123,
						"name": "workflow-run-123",
						"status": "completed",
						"conclusion": "success",
						"run_started_at": "2026-07-01T00:00:00Z",
						"updated_at": "2026-07-01T01:00:00Z",
						"repository": {
							"id": 1,
							"name": "r",
							"full_name": "o/r"
						}
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}

				// 5. jobs for workflow run 123
				if path == "/repos/o/r/actions/runs/123/jobs" {
					respJSON := `{
						"total_count": 1,
						"jobs": [
							{
								"id": 1,
								"run_id": 123,
								"status": "completed",
								"conclusion": "success",
								"name": "job-1"
							}
						]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}

				// 6. artifacts for workflow run 123
				if path == "/repos/o/r/actions/runs/123/artifacts" {
					respJSON := `{
						"total_count": 0,
						"artifacts": []
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}

				return nil, fmt.Errorf("unexpected request path: %s", path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	commit, err := Commit(
		log,
		client,
		"o",
		"r",
		"merge-sha",
		ForceUpdate(),
		CustomDataFolder(testDataDir),
		WithoutCost(),
	)
	require.NoError(t, err)
	require.NotNil(t, commit)
	require.Equal(t, "merge-sha", commit.GetSHA())
	require.Len(t, commit.WorkflowRunIDs, 1)
	require.Equal(t, int64(123), commit.WorkflowRunIDs[0])
}

func TestCommit_WithRepositoryCommit(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)
	// client is nil to prove GitHub API is NOT called
	repoCommit := &github.RepositoryCommit{
		SHA: new("1234567890abcdef1234567890abcdef12345678"),
		Commit: &github.Commit{
			Message: new("test commit"),
		},
	}

	commitData, err := Commit(
		log,
		nil,
		"owner",
		"repo",
		repoCommit.GetSHA(),
		CustomDataFolder(tempDir),
		withRepositoryCommit(repoCommit),
	)
	require.NoError(t, err)
	require.NotNil(t, commitData)
	assert.Equal(t, repoCommit.GetSHA(), commitData.GetSHA())
}

func TestCommit_CheckRunsNotPersisted(t *testing.T) {
	t.Parallel()

	cd := &CommitData{
		Owner:          "owner",
		Repo:           "repo",
		CheckRuns:      []*github.CheckRun{{ID: new(int64(123))}},
		WorkflowRunIDs: []int64{999},
	}

	data, err := json.Marshal(cd)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.NotContains(t, jsonStr, "check_runs")
	assert.Contains(t, jsonStr, "workflow_run_ids")
}

func TestCommit_WorkflowRunsNotPersistedAndHydrated(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	wfDir := filepath.Join(tempDir, "owner", "repo", WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(wfDir, 0750))
	sampleWFJSON := []byte(
		`{"id":999,"name":"test-wf","status":"completed","conclusion":"success","cost_gathered":true}`,
	)
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "999.json"), sampleWFJSON, 0600))

	cd := &CommitData{
		Owner:          "owner",
		Repo:           "repo",
		WorkflowRunIDs: []int64{999},
		WorkflowRuns:   []*WorkflowRunData{{WorkflowRun: &github.WorkflowRun{ID: new(int64(999))}}},
	}

	data, err := json.Marshal(cd)
	require.NoError(t, err)
	jsonStr := string(data)
	assert.NotContains(t, jsonStr, "workflow_runs", "CommitData JSON should not persist workflow_runs")

	commitDir := filepath.Join(tempDir, "owner", "repo", CommitsDataDir)
	require.NoError(t, os.MkdirAll(commitDir, 0750))
	commitFile := filepath.Join(commitDir, "abc1234.json")
	require.NoError(t, os.WriteFile(commitFile, data, 0600))

	loaded, err := Commit(log, nil, "owner", "repo", "abc1234", CustomDataFolder(tempDir))
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, loaded.WorkflowRuns, 1, "WorkflowRuns should be hydrated from disk cache using WorkflowRunIDs")
	assert.Equal(t, int64(999), loaded.WorkflowRuns[0].GetID())
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
