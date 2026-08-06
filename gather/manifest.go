package gather

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ManifestRecord represents a single index entry stored in manifest.jsonl.
type ManifestRecord struct {
	Type      string    `json:"type"` // "workflow_run", "job_run", "commit", "pull_request"
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

var manifestMu sync.Mutex

// ManifestPath returns the path to manifest.jsonl for an owner/repo.
func ManifestPath(dataDir, owner, repo string) string {
	return filepath.Join(dataDir, owner, repo, "manifest.jsonl")
}

// AppendManifestRecord appends a record to data/<owner>/<repo>/manifest.jsonl.
func AppendManifestRecord(dataDir, owner, repo string, rec ManifestRecord) error {
	manifestMu.Lock()
	defer manifestMu.Unlock()

	p := ManifestPath(dataDir, owner, repo)
	p = filepath.Clean(p)
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	//nolint:gosec // ManifestPath constructs path within data directory
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open manifest file: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest record: %w", err)
	}

	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write manifest record: %w", err)
	}

	return nil
}

// LoadManifest loads all records from data/<owner>/<repo>/manifest.jsonl.
func LoadManifest(dataDir, owner, repo string) ([]ManifestRecord, error) {
	manifestMu.Lock()
	defer manifestMu.Unlock()

	p := filepath.Clean(ManifestPath(dataDir, owner, repo))
	//nolint:gosec // ManifestPath constructs path within data directory
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open manifest file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var records []ManifestRecord
	indexMap := make(map[string]int)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ManifestRecord
		if err := json.Unmarshal(line, &rec); err == nil {
			key := rec.Type + ":" + rec.ID
			if idx, ok := indexMap[key]; ok {
				records[idx] = rec
			} else {
				indexMap[key] = len(records)
				records = append(records, rec)
			}
		}
	}

	return records, scanner.Err()
}

// RebuildManifest walks dataDir and reconstructs manifest.jsonl files.
func RebuildManifest(ctx context.Context, log zerolog.Logger, dataDir string) error {
	byRepo := make(map[string][]ManifestRecord)

	err := filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		relPath, relErr := filepath.Rel(dataDir, path)
		if relErr != nil {
			return nil
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) != 4 {
			return nil
		}
		owner, repo, cat, name := parts[0], parts[1], parts[2], strings.TrimSuffix(parts[3], ".json")
		repoKey := owner + "/" + repo

		switch cat {
		case WorkflowRunsDataDir:
			runID, pErr := strconv.ParseInt(name, 10, 64)
			if pErr == nil {
				wfData, _, loadErr := WorkflowRun(ctx, log, nil, owner, repo, runID, CustomDataFolder(dataDir))
				if loadErr == nil && wfData != nil {
					state := wfData.GetConclusion()
					if state == "" {
						state = wfData.GetStatus()
					}
					byRepo[repoKey] = append(byRepo[repoKey], ManifestRecord{
						Type:      "workflow_run",
						ID:        name,
						Name:      wfData.GetName(),
						State:     state,
						Actor:     wfData.GetActor().GetLogin(),
						CreatedAt: wfData.GetCreatedAt().Time,
					})
					for _, job := range wfData.Jobs {
						jState := job.GetConclusion()
						if jState == "" {
							jState = job.GetStatus()
						}
						byRepo[repoKey] = append(byRepo[repoKey], ManifestRecord{
							Type:      "job_run",
							ID:        fmt.Sprint(job.GetID()),
							Name:      job.GetName(),
							State:     jState,
							Actor:     wfData.GetActor().GetLogin(),
							CreatedAt: job.GetStartedAt().Time,
						})
					}
				}
			}
		case CommitsDataDir:
			cData, cErr := Commit(ctx, log, nil, owner, repo, name, CustomDataFolder(dataDir))
			if cErr == nil && cData != nil {
				byRepo[repoKey] = append(byRepo[repoKey], ManifestRecord{
					Type:      "commit",
					ID:        name,
					Name:      "Commit " + name,
					State:     cData.Conclusion,
					Actor:     cData.GetCommit().GetAuthor().GetName(),
					CreatedAt: cData.GetCommit().GetAuthor().GetDate().Time,
				})
			}
		case PullRequestsDataDir:
			prNum, pErr := strconv.Atoi(name)
			if pErr == nil {
				prData, prErr := PullRequest(ctx, log, nil, owner, repo, prNum, CustomDataFolder(dataDir))
				if prErr == nil && prData != nil {
					byRepo[repoKey] = append(byRepo[repoKey], ManifestRecord{
						Type:      "pull_request",
						ID:        name,
						Name:      prData.GetTitle(),
						State:     prData.GetState(),
						Actor:     prData.GetUser().GetLogin(),
						CreatedAt: prData.GetCreatedAt().Time,
					})
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for repoKey, records := range byRepo {
		parts := strings.SplitN(repoKey, "/", 2)
		owner, repo := parts[0], parts[1]
		p := ManifestPath(dataDir, owner, repo)
		_ = os.Remove(p)
		for _, rec := range records {
			if err := AppendManifestRecord(dataDir, owner, repo, rec); err != nil {
				return err
			}
		}
	}

	return nil
}
