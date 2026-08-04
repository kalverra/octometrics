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
