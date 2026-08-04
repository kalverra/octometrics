package observe

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestExportStaticAssetWritten(t *testing.T) {
	t.Parallel()

	_, tempDir := testhelpers.Setup(t)

	err := WriteStaticAssets(tempDir)
	require.NoError(t, err)

	//nolint:gosec // test file read
	exportJsContent, err := os.ReadFile(filepath.Join(tempDir, "export-png.js"))
	require.NoError(t, err)
	assert.Contains(t, string(exportJsContent), "html-to-image")
	assert.Contains(t, string(exportJsContent), "Export to PNG")
}

func TestObservationHTMLIncludesExportScript(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	obs := &Observation{
		ID:         "12345",
		Name:       "test-run",
		Owner:      "owner",
		Repo:       "repo",
		DataType:   "workflow_run",
		State:      "success",
		Actor:      "user",
		GitHubLink: "https://github.com/owner/repo/actions/runs/12345",
		TimelineData: []*Timeline{
			{Event: "push"},
		},
	}

	setActiveHTMLOutputDir(tempDir)
	renderedFile, err := obs.Render(log, "html")
	require.NoError(t, err)

	//nolint:gosec // test file read
	content, err := os.ReadFile(renderedFile)
	require.NoError(t, err)
	htmlStr := string(content)

	assert.Contains(t, htmlStr, `<script type="module" src="/export-png.js"></script>`)
	assert.Contains(t, htmlStr, `class="section event-section"`)
}

func TestCompareHTMLIncludesExportScript(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	comp := &Comparison{
		Owner: "owner",
		Repo:  "repo",
		Left:  &Observation{ID: "111", Name: "left"},
		Right: &Observation{ID: "222", Name: "right"},
		EventPairs: []EventPair{
			{
				Event: "push",
				Left:  &Timeline{Event: "push"},
				Right: &Timeline{Event: "push"},
			},
		},
	}

	setActiveHTMLOutputDir(tempDir)
	renderedRelPath, err := comp.Render(log, "html")
	require.NoError(t, err)

	outPath := filepath.Join(tempDir, renderedRelPath)
	//nolint:gosec // test file read
	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	htmlStr := string(content)

	assert.Contains(t, htmlStr, `<script type="module" src="/export-png.js"></script>`)
	assert.Contains(t, htmlStr, `class="section event-section"`)
}

func TestHandlerServesExportJS(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	handler := NewOnDemandHandler(log, nil, tempDir, tempDir)

	req := httptest.NewRequest("GET", "/export-png.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "javascript")
	assert.Contains(t, rr.Body.String(), "html-to-image")
}
