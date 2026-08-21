package observe

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestGenerateAllObserveData_IgnoresNonJSONAndSkipsExisting(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	dataDir := filepath.Join(tempDir, "data")
	outputDir := filepath.Join(tempDir, "output", "html")
	wfDir := filepath.Join(dataDir, "owner", "repo", "workflow_runs")
	require.NoError(t, os.MkdirAll(wfDir, 0o750))

	// Write valid workflow run JSON
	jsonPath := filepath.Join(wfDir, "555.json")
	sampleJSON := []byte(`{
		"id": 555,
		"name": "all-test-wf",
		"status": "completed",
		"conclusion": "success",
		"html_url": "https://github.com/owner/repo/actions/runs/555",
		"repository": {"name": "repo", "owner": {"login": "owner"}},
		"actor": {"login": "user"},
		"jobs": []
	}`)
	require.NoError(t, os.WriteFile(jsonPath, sampleJSON, 0o600))

	// Write non-JSON files that previously caused errors
	manifestPath := filepath.Join(dataDir, "owner", "repo", "manifest.jsonl")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`{"type":"workflow_run","id":"555"}`), 0o600))

	monitorPath := filepath.Join(wfDir, "octometrics-monitor-555.jsonl")
	require.NoError(t, os.WriteFile(monitorPath, []byte(`{"message":"cpu"}`), 0o600))

	// First run: should ignore non-JSON files and render 555.html
	err := generateAllObserveData(
		t.Context(),
		log,
		nil,
		[]string{"html"},
		dataDir,
		WithCustomOutputDir(outputDir),
		WithGatherOptions(gather.CustomDataFolder(dataDir)),
	)
	require.NoError(t, err, "generateAllObserveData should ignore non-JSON files without error")

	outPath := filepath.Join(outputDir, "owner", "repo", "workflow_runs", "555.html")
	require.FileExists(t, outPath)

	// Modify content of rendered html to sentinel string
	sentinel := []byte("<!-- SENTINEL EDITED HTML -->")
	require.NoError(t, os.WriteFile(outPath, sentinel, 0o600))

	// Set mtime of output HTML to future so it's newer than source JSON
	future := time.Now().Add(10 * time.Minute)
	require.NoError(t, os.Chtimes(outPath, future, future))

	// Second run: should skip rendering 555.html because output exists and is up to date
	err = generateAllObserveData(
		t.Context(),
		log,
		nil,
		[]string{"html"},
		dataDir,
		WithCustomOutputDir(outputDir),
		WithGatherOptions(gather.CustomDataFolder(dataDir)),
	)
	require.NoError(t, err)

	//nolint:gosec
	content, readErr := os.ReadFile(outPath)
	require.NoError(t, readErr)
	assert.Equal(
		t,
		string(sentinel),
		string(content),
		"existing up-to-date output should be skipped and not overwritten",
	)
}
