package gather

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

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
	if err := os.Rename(tmpPath, targetFile); err != nil {
		return err
	}
	return nil
}

func ensureDataDir(dir, dirName string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to make data dir '%s': %w", dirName, err)
	}
	return nil
}

func readJSONFile[T any](path string) (T, error) {
	var value T
	bytes, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(bytes, &value); err != nil {
		return value, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}
	return value, nil
}

func writeJSONFile(path string, value any) error {
	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal data to json: %w", err)
	}
	if err := writeFileAtomic(path, bytes, 0o600); err != nil {
		return fmt.Errorf("failed to write data to file: %w", err)
	}
	return nil
}

func cacheFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type inFlightCall struct {
	wg  sync.WaitGroup
	val any
	err error
}

type inFlightGroup struct {
	mu sync.Mutex
	m  map[string]*inFlightCall
}

func (g *inFlightGroup) Do(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*inFlightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(inFlightCall)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}
