// Package uistate manages persistent UI state such as favorites and recents.
package uistate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RepoRef represents a GitHub repository reference.
type RepoRef struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

// State manages persistent favorites and recents.
type State struct {
	mu        sync.Mutex
	dataDir   string
	Favorites []RepoRef `json:"favorites"`
	Recents   []RepoRef `json:"recents"`
}

// Load loads the UI state from dataDir/ui_state.json.
func Load(dataDir string) (*State, error) {
	st := &State{
		dataDir:   dataDir,
		Favorites: []RepoRef{},
		Recents:   []RepoRef{},
	}
	if dataDir == "" {
		return st, nil
	}
	filePath := filepath.Clean(filepath.Join(dataDir, "ui_state.json"))
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, nil
	}

	var raw struct {
		Favorites []RepoRef `json:"favorites"`
		Recents   []RepoRef `json:"recents"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return st, nil
	}

	if raw.Favorites != nil {
		st.Favorites = raw.Favorites
	}
	if raw.Recents != nil {
		st.Recents = raw.Recents
	}
	return st, nil
}

func (s *State) saveLocked() error {
	if s.dataDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return fmt.Errorf("mkdir data dir: %w", err)
	}

	raw := struct {
		Favorites []RepoRef `json:"favorites"`
		Recents   []RepoRef `json:"recents"`
	}{
		Favorites: s.Favorites,
		Recents:   s.Recents,
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	targetFile := filepath.Join(s.dataDir, "ui_state.json")
	return writeFileAtomic(targetFile, data, 0o600)
}

func writeFileAtomic(targetFile string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetFile)
	tmpFile, err := os.CreateTemp(dir, ".octometrics-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, targetFile)
}

// TouchRecent updates recents by moving (owner, repo) to front and capping at 20.
func (s *State) TouchRecent(owner, repo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ref := RepoRef{Owner: owner, Repo: repo}
	filtered := make([]RepoRef, 0, len(s.Recents)+1)
	filtered = append(filtered, ref)

	for _, r := range s.Recents {
		if r.Owner == owner && r.Repo == repo {
			continue
		}
		filtered = append(filtered, r)
	}

	if len(filtered) > 20 {
		filtered = filtered[:20]
	}
	s.Recents = filtered
	return s.saveLocked()
}

// ToggleFavorite adds or removes (owner, repo) from favorites.
func (s *State) ToggleFavorite(owner, repo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	foundIndex := -1
	for i, r := range s.Favorites {
		if r.Owner == owner && r.Repo == repo {
			foundIndex = i
			break
		}
	}

	if foundIndex >= 0 {
		s.Favorites = append(s.Favorites[:foundIndex], s.Favorites[foundIndex+1:]...)
	} else {
		s.Favorites = append(s.Favorites, RepoRef{Owner: owner, Repo: repo})
	}

	return s.saveLocked()
}

// IsFavorite returns true if (owner, repo) is in favorites.
func (s *State) IsFavorite(owner, repo string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.Favorites {
		if r.Owner == owner && r.Repo == repo {
			return true
		}
	}
	return false
}
