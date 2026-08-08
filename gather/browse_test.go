package gather

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestBrowse_CacheHit(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)
	var requestCount atomic.Int32
	owner := "owner"
	repo := fmt.Sprintf("repo-%d", time.Now().UnixNano())
	expectedPath := fmt.Sprintf("/repos/%s/%s", owner, repo)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				requestCount.Add(1)
				if req.URL.Path == expectedPath {
					respJSON := fmt.Sprintf(`{
						"id": 123,
						"name": "%s",
						"full_name": "%s/%s",
						"description": "Test Repo",
						"default_branch": "main",
						"html_url": "https://github.com/%s/%s",
						"owner": {"login": "%s"}
					}`, repo, owner, repo, owner, repo, owner)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	// First call -> HTTP request
	summary1, err := RepoInfo(t.Context(), log, client, owner, repo)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%s/%s", owner, repo), summary1.FullName)
	assert.Equal(t, int32(1), requestCount.Load())

	// Second call -> cached, no HTTP request
	summary2, err := RepoInfo(t.Context(), log, client, owner, repo)
	require.NoError(t, err)
	assert.Equal(t, summary1, summary2)
	assert.Equal(t, int32(1), requestCount.Load())
}

func TestBrowse_SearchRepos(t *testing.T) {
	t.Parallel()
	log, _ := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/search/repositories" {
					respJSON := `{
						"total_count": 1,
						"items": [{
							"name": "octometrics",
							"full_name": "kalverra/octometrics",
							"description": "Profiler",
							"default_branch": "main",
							"html_url": "https://github.com/kalverra/octometrics",
							"owner": {"login": "kalverra"}
						}]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	results, err := SearchRepos(t.Context(), log, client, "octometrics", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "kalverra", results[0].Owner)
	assert.Equal(t, "octometrics", results[0].Name)
}

func TestBrowse_ListWorkflows(t *testing.T) {
	t.Parallel()
	log, _ := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/repos/o/r/actions/workflows" {
					respJSON := `{
						"total_count": 1,
						"workflows": [{
							"id": 42,
							"name": "CI",
							"path": ".github/workflows/ci.yml",
							"state": "active",
							"html_url": "https://github.com/o/r/actions/workflows/ci.yml"
						}]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	wfs, err := ListWorkflows(t.Context(), log, client, "o", "r")
	require.NoError(t, err)
	require.Len(t, wfs, 1)
	assert.Equal(t, int64(42), wfs[0].ID)
	assert.Equal(t, "CI", wfs[0].Name)
}

func TestBrowse_ListRuns(t *testing.T) {
	t.Parallel()
	log, _ := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/repos/o/r/actions/runs" || req.URL.Path == "/repos/o/r/actions/workflows/42/runs" {
					respJSON := `{
						"total_count": 1,
						"workflow_runs": [{
							"id": 99,
							"name": "CI Run",
							"workflow_id": 42,
							"status": "completed",
							"conclusion": "success",
							"event": "push",
							"actor": {"login": "kalverra"},
							"head_branch": "main",
							"head_sha": "abc1234",
							"created_at": "2026-08-01T12:00:00Z",
							"html_url": "https://github.com/o/r/actions/runs/99"
						}]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	// All repo runs
	runs, err := ListRuns(t.Context(), log, client, "o", "r", 0, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, int64(99), runs[0].ID)

	// Workflow specific runs
	wfRuns, err := ListRuns(t.Context(), log, client, "o", "r", 42, 10)
	require.NoError(t, err)
	require.Len(t, wfRuns, 1)
	assert.Equal(t, int64(99), wfRuns[0].ID)
}

func TestBrowse_ListCommits(t *testing.T) {
	t.Parallel()
	log, _ := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/repos/o/r/commits" {
					respJSON := `[{
						"sha": "1234567890abcdef1234567890abcdef12345678",
						"html_url": "https://github.com/o/r/commit/1234567890abcdef1234567890abcdef12345678",
						"commit": {
							"message": "fix bug",
							"committer": {
								"name": "Adam",
								"date": "2026-08-01T12:00:00Z"
							}
						},
						"author": {"login": "kalverra"}
					}]`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	commits, err := ListCommits(t.Context(), log, client, "o", "r", 10)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "1234567890abcdef1234567890abcdef12345678", commits[0].SHA)
	assert.Equal(t, "kalverra", commits[0].Author)
}

func TestBrowse_ListPullRequests(t *testing.T) {
	t.Parallel()
	log, _ := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/repos/o/r/pulls" {
					respJSON := `[{
						"id": 101,
						"number": 5,
						"title": "Add feature",
						"state": "open",
						"created_at": "2026-08-01T12:00:00Z",
						"html_url": "https://github.com/o/r/pull/5",
						"user": {"login": "kalverra"},
						"base": {"repo": {"owner": {"login": "o"}, "name": "r"}}
					}]`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	prs, err := ListPullRequests(t.Context(), log, client, "o", "r", 10)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 5, prs[0].Number)
	assert.Equal(t, "Add feature", prs[0].Title)
}

func TestBrowse_SearchPullRequests(t *testing.T) {
	t.Parallel()
	log, _ := testhelpers.Setup(t)

	httpClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/search/issues" {
					respJSON := `{
						"total_count": 1,
						"items": [{
							"id": 202,
							"number": 12,
							"title": "Search PR Result",
							"state": "open",
							"created_at": "2026-08-01T12:00:00Z",
							"html_url": "https://github.com/o/r/pull/12",
							"user": {"login": "kalverra"},
							"repository_url": "https://api.github.com/repos/o/r"
						}]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respJSON)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}
				return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
			},
		},
	}

	client, err := NewGitHubClient(log, "mock-token", httpClient.Transport)
	require.NoError(t, err)

	prs, err := SearchPullRequests(t.Context(), log, client, "fix bug", 10)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 12, prs[0].Number)
	assert.Equal(t, "o", prs[0].Owner)
	assert.Equal(t, "r", prs[0].Repo)
}
