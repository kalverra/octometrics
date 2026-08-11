package observe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestHandler_Home(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Octometrics")
	assert.Contains(t, body, "Not connected to GitHub")
	assert.Contains(t, body, "Favorites")
	assert.Contains(t, body, "Recents")
}

func TestHandler_Search(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	// Full search page
	req := httptest.NewRequest("GET", "/search?q=test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "<html")
	assert.Contains(t, body, "Search")

	// Fragment search response (partial=1)
	reqPartial := httptest.NewRequest("GET", "/search?q=test&partial=1", nil)
	recPartial := httptest.NewRecorder()
	handler.ServeHTTP(recPartial, reqPartial)

	require.Equal(t, http.StatusOK, recPartial.Code)
	bodyPartial := recPartial.Body.String()
	assert.NotContains(t, bodyPartial, "<html")
}

func TestHandler_RepoPage(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	req := httptest.NewRequest("GET", "/kalverra/octometrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "kalverra/octometrics")
	assert.Contains(t, body, "Workflows")
	assert.Contains(t, body, "Commits")
	assert.Contains(t, body, "Pull Requests")
}

func TestHandler_RepoPage_CommitsTab(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	_ = gather.AppendManifestRecord(dataDir, "kalverra", "octometrics", gather.ManifestRecord{
		Type:  "commit",
		ID:    "abc123456789",
		Name:  "Merge branch 'main' into gh-readonly-queue/main/pr-1",
		Actor: "kalverra",
	})

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	req := httptest.NewRequest("GET", "/kalverra/octometrics?tab=commits", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Recent Commits")
	assert.Contains(t, body, "Branch")
	assert.NotContains(t, body, "git-tree-svg")
}

func TestHandler_RepoPage_SearchFilter(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	_ = gather.AppendManifestRecord(dataDir, "kalverra", "octometrics", gather.ManifestRecord{
		Type:  "commit",
		ID:    "abc123456789",
		Name:  "Fix critical bug",
		Actor: "kalverra",
	})
	_ = gather.AppendManifestRecord(dataDir, "kalverra", "octometrics", gather.ManifestRecord{
		Type:  "commit",
		ID:    "def987654321",
		Name:  "Update docs",
		Actor: "octocat",
	})
	_ = gather.AppendManifestRecord(dataDir, "kalverra", "octometrics", gather.ManifestRecord{
		Type:  "workflow_run",
		ID:    "12345",
		Name:  "CI Build",
		State: "success",
		Actor: "kalverra",
	})
	_ = gather.AppendManifestRecord(dataDir, "kalverra", "octometrics", gather.ManifestRecord{
		Type:  "pull_request",
		ID:    "42",
		Name:  "Add feature X",
		State: "open",
		Actor: "kalverra",
	})

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	// Filter commits by SHA/query
	reqCommit := httptest.NewRequest("GET", "/kalverra/octometrics?tab=commits&q=abc1234", nil)
	recCommit := httptest.NewRecorder()
	handler.ServeHTTP(recCommit, reqCommit)
	require.Equal(t, http.StatusOK, recCommit.Code)
	bodyCommit := recCommit.Body.String()
	assert.Contains(t, bodyCommit, "abc123456789")
	assert.NotContains(t, bodyCommit, "def987654321")

	// Filter workflow runs by run ID
	reqRun := httptest.NewRequest("GET", "/kalverra/octometrics?tab=workflows&q=12345", nil)
	recRun := httptest.NewRecorder()
	handler.ServeHTTP(recRun, reqRun)
	require.Equal(t, http.StatusOK, recRun.Code)
	bodyRun := recRun.Body.String()
	assert.Contains(t, bodyRun, "#12345")

	// Filter PRs by PR number
	reqPR := httptest.NewRequest("GET", "/kalverra/octometrics?tab=pulls&q=42", nil)
	recPR := httptest.NewRecorder()
	handler.ServeHTTP(recPR, reqPR)
	require.Equal(t, http.StatusOK, recPR.Code)
	bodyPR := recPR.Body.String()
	assert.Contains(t, bodyPR, "#42")
}

func TestHandler_FavoriteToggle(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	req := httptest.NewRequest("POST", "/favorites", nil)
	req.Form = map[string][]string{
		"owner": {"kalverra"},
		"repo":  {"octometrics"},
	}
	req.Header.Set("Referer", "http://localhost:8080/kalverra/octometrics")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "http://localhost:8080/kalverra/octometrics", rec.Header().Get("Location"))
}

func TestHandler_PendingInterstitial(t *testing.T) {
	t.Parallel()

	log, dataDir := testhelpers.Setup(t)
	outputDir := t.TempDir()

	// Handler with nil client so rendering entity fails or stays pending
	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	req := httptest.NewRequest("GET", "/owner/repo/workflow_runs/999.html", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "http-equiv=\"refresh\"")
	assert.Contains(t, body, "Gathering")

	// Wait for background job to finish before test cleanup
	time.Sleep(50 * time.Millisecond)
}
