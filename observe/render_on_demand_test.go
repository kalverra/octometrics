package observe

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestManifestRecord(t *testing.T) {
	t.Parallel()

	_, tempDir := testhelpers.Setup(t)

	rec := ManifestRecord{
		Type:      "workflow_run",
		ID:        "123",
		Name:      "test-workflow",
		State:     "success",
		Actor:     "user1",
		CreatedAt: time.Now(),
	}

	err := AppendManifestRecord(tempDir, "owner", "repo", rec)
	require.NoError(t, err)

	records, err := LoadManifest(tempDir, "owner", "repo")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "workflow_run", records[0].Type)
	assert.Equal(t, "123", records[0].ID)
	assert.Equal(t, "test-workflow", records[0].Name)
}

func TestRenderOnDemandHandler(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	// Create dummy data dir structure
	dataDir := filepath.Join(tempDir, "data")
	outputDir := filepath.Join(tempDir, "output")
	wfDir := filepath.Join(dataDir, "owner", "repo", "workflow_runs")
	require.NoError(t, os.MkdirAll(wfDir, 0750))

	jsonPath := filepath.Join(wfDir, "555.json")
	sampleJSON := []byte(`{
		"id": 555,
		"name": "on-demand-wf",
		"status": "completed",
		"conclusion": "success",
		"html_url": "https://github.com/owner/repo/actions/runs/555",
		"repository": {"name": "repo", "owner": {"login": "owner"}},
		"actor": {"login": "user"},
		"jobs": []
	}`)
	require.NoError(t, os.WriteFile(jsonPath, sampleJSON, 0600))

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	// Request 1: Cold hit (misses disk cache, renders on demand)
	req := httptest.NewRequest("GET", "/owner/repo/workflow_runs/555.html", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "on-demand-wf")

	// Verify file was written to outputDir
	outPath := filepath.Join(outputDir, "owner", "repo", "workflow_runs", "555.html")
	//nolint:gosec // test file read
	outContent, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(outContent), "on-demand-wf")

	// Request 2: Warm hit (serves from disk cache)
	req2 := httptest.NewRequest("GET", "/owner/repo/workflow_runs/555.html", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	assert.Equal(t, http.StatusOK, rr2.Code)
	assert.Equal(t, string(outContent), rr2.Body.String())
}

func TestRenderOnDemandHandler_JobRun(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	dataDir := filepath.Join(tempDir, "data")
	outputDir := filepath.Join(tempDir, "output")
	wfDir := filepath.Join(dataDir, "owner", "repo", "workflow_runs")
	require.NoError(t, os.MkdirAll(wfDir, 0750))

	// Workflow run JSON containing a job with ID 777
	jsonPath := filepath.Join(wfDir, "555.json")
	sampleJSON := []byte(`{
		"id": 555,
		"name": "on-demand-wf",
		"status": "completed",
		"conclusion": "success",
		"html_url": "https://github.com/owner/repo/actions/runs/555",
		"repository": {"name": "repo", "owner": {"login": "owner"}},
		"actor": {"login": "user"},
		"run_started_at": "2025-01-01T00:00:00Z",
		"created_at": "2025-01-01T00:00:00Z",
		"jobs": [
			{
				"id": 777,
				"run_id": 555,
				"name": "test-job",
				"status": "completed",
				"conclusion": "success",
				"html_url": "https://github.com/owner/repo/actions/runs/555/job/777",
				"started_at": "2025-01-01T00:00:00Z",
				"completed_at": "2025-01-01T00:01:00Z",
				"steps": [],
				"labels": ["ubuntu-latest"]
			}
		]
	}`)
	require.NoError(t, os.WriteFile(jsonPath, sampleJSON, 0600))

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	// Request the job run page — should find parent workflow run 555 and render
	req := httptest.NewRequest("GET", "/owner/repo/job_runs/777.html", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "test-job")

	// Verify job run file was written to outputDir
	outPath := filepath.Join(outputDir, "owner", "repo", "job_runs", "777.html")
	//nolint:gosec // test file read
	outContent, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(outContent), "test-job")
}

func TestRenderOnDemandHandler_Comparison(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	dataDir := filepath.Join(tempDir, "data")
	outputDir := filepath.Join(tempDir, "output")

	// Pre-render a comparison HTML file as the compare command does.
	compPath := filepath.Join(outputDir, "owner", "repo", "comparisons", "111_vs_222.html")
	require.NoError(t, os.MkdirAll(filepath.Dir(compPath), 0750))
	require.NoError(t, os.WriteFile(compPath, []byte("<html>Left vs Right</html>"), 0600))

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	req := httptest.NewRequest("GET", "/owner/repo/comparisons/111_vs_222.html", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Left vs Right")
}

func TestRenderOnDemandHandler_StaleHTMLRefreshed(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	dataDir := filepath.Join(tempDir, "data")
	outputDir := filepath.Join(tempDir, "output")
	wfDir := filepath.Join(dataDir, "owner", "repo", "workflow_runs")
	require.NoError(t, os.MkdirAll(wfDir, 0750))

	// Write initial JSON
	jsonPath := filepath.Join(wfDir, "555.json")
	writeJSON := func(content string) {
		require.NoError(t, os.WriteFile(jsonPath, []byte(content), 0600))
	}
	writeJSON(`{
		"id": 555,
		"name": "stale-wf",
		"status": "completed",
		"conclusion": "success",
		"html_url": "https://github.com/owner/repo/actions/runs/555",
		"repository": {"name": "repo", "owner": {"login": "owner"}},
		"actor": {"login": "user"},
		"jobs": []
	}`)

	handler := NewOnDemandHandler(log, nil, dataDir, outputDir)

	// First request renders the page
	req := httptest.NewRequest("GET", "/owner/repo/workflow_runs/555.html", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "stale-wf")

	// Write stale HTML with old content that should be overwritten
	outPath := filepath.Join(outputDir, "owner", "repo", "workflow_runs", "555.html")
	require.NoError(t, os.MkdirAll(filepath.Dir(outPath), 0750))
	require.NoError(t, os.WriteFile(outPath, []byte("<html>STALE CONTENT</html>"), 0600))

	// Touch JSON to be newer than the stale HTML
	time.Sleep(10 * time.Millisecond)
	writeJSON(`{
		"id": 555,
		"name": "fresh-wf",
		"status": "completed",
		"conclusion": "success",
		"html_url": "https://github.com/owner/repo/actions/runs/555",
		"repository": {"name": "repo", "owner": {"login": "owner"}},
		"actor": {"login": "user"},
		"jobs": []
	}`)

	// Second request should detect stale HTML and re-render with new data
	req2 := httptest.NewRequest("GET", "/owner/repo/workflow_runs/555.html", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	assert.Equal(t, http.StatusOK, rr2.Code)
	assert.NotContains(t, rr2.Body.String(), "STALE CONTENT")
	assert.Contains(t, rr2.Body.String(), "fresh-wf")
}
