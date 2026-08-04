package observe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestExternalStaticAssets(t *testing.T) {
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

	assert.Contains(t, htmlStr, `<link rel="stylesheet" href="/styles.css">`)
	assert.Contains(t, htmlStr, `<script type="module" src="/mermaid-init.js"></script>`)
	assert.NotContains(t, htmlStr, `/* Go Template: inlined as a <style> block */`)
	assert.NotContains(t, htmlStr, `Charts unavailable (Mermaid CDN failed to load)`)

	// Test writing static files to output directory
	err = WriteStaticAssets(tempDir)
	require.NoError(t, err)

	//nolint:gosec // test file read
	stylesContent, err := os.ReadFile(filepath.Join(tempDir, "styles.css"))
	require.NoError(t, err)
	assert.Contains(t, string(stylesContent), `box-sizing: border-box;`)

	//nolint:gosec // test file read
	mermaidJsContent, err := os.ReadFile(filepath.Join(tempDir, "mermaid-init.js"))
	require.NoError(t, err)
	assert.Contains(t, string(mermaidJsContent), `import('https://cdn.jsdelivr.net/npm/mermaid`)
}
