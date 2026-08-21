package gather

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadManifest_DedupesByTypeAndID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	owner, repo := "kalverra", "octometrics"
	p := ManifestPath(dir, owner, repo)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))

	//nolint:gosec
	f, err := os.Create(filepath.Clean(p))
	require.NoError(t, err)
	_, err = fmt.Fprintf(
		f,
		"%s\n%s\n",
		`{"type":"workflow_run","id":"1","name":"first","state":"success","actor":"a","created_at":"2025-01-01T00:00:00Z"}`,
		`{"type":"workflow_run","id":"1","name":"second","state":"failure","actor":"b","created_at":"2025-01-02T00:00:00Z"}`,
	)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	recs, err := LoadManifest(dir, owner, repo)
	require.NoError(t, err)
	require.Len(t, recs, 1, "duplicate type+id records should be collapsed to the latest")
	require.Equal(t, "failure", recs[0].State)
	require.Equal(t, "b", recs[0].Actor)
	require.Equal(t, "second", recs[0].Name)
}
