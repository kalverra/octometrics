package uistate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_NonExistent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	st, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, st.Favorites)
	assert.Empty(t, st.Recents)
}

func TestLoad_CorruptFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "ui_state.json")
	require.NoError(t, os.WriteFile(filePath, []byte("{invalid json"), 0o600))

	st, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, st.Favorites)
	assert.Empty(t, st.Recents)
}

func TestTouchRecent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	st, err := Load(dir)
	require.NoError(t, err)

	require.NoError(t, st.TouchRecent("owner1", "repo1"))
	assert.Len(t, st.Recents, 1)
	assert.Equal(t, RepoRef{Owner: "owner1", Repo: "repo1"}, st.Recents[0])

	// Touch again: moves to front / dedupes
	require.NoError(t, st.TouchRecent("owner2", "repo2"))
	require.NoError(t, st.TouchRecent("owner1", "repo1"))
	assert.Len(t, st.Recents, 2)
	assert.Equal(t, RepoRef{Owner: "owner1", Repo: "repo1"}, st.Recents[0])
	assert.Equal(t, RepoRef{Owner: "owner2", Repo: "repo2"}, st.Recents[1])

	// Reload from disk to check persistence
	reloaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, st.Recents, reloaded.Recents)

	// Test cap at 20
	for i := range 25 {
		require.NoError(t, st.TouchRecent("owner", "repo"+string(rune('a'+i))))
	}
	assert.Len(t, st.Recents, 20)
}

func TestToggleFavorite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	st, err := Load(dir)
	require.NoError(t, err)

	assert.False(t, st.IsFavorite("owner1", "repo1"))

	require.NoError(t, st.ToggleFavorite("owner1", "repo1"))
	assert.True(t, st.IsFavorite("owner1", "repo1"))

	require.NoError(t, st.ToggleFavorite("owner1", "repo1"))
	assert.False(t, st.IsFavorite("owner1", "repo1"))

	reloaded, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, reloaded.IsFavorite("owner1", "repo1"))
}
