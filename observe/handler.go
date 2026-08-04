package observe

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"

	"github.com/kalverra/octometrics/gather"
)

// OnDemandHandler serves observations, rendering them lazily if missing or stale.
type OnDemandHandler struct {
	log       zerolog.Logger
	client    *gather.GitHubClient
	dataDir   string
	outputDir string
	opts      []Option
	sf        singleflight.Group
}

// NewOnDemandHandler creates a new OnDemandHandler.
func NewOnDemandHandler(
	log zerolog.Logger,
	client *gather.GitHubClient,
	dataDir string,
	outputDir string,
	opts ...Option,
) *OnDemandHandler {
	return &OnDemandHandler{
		log:       log,
		client:    client,
		dataDir:   dataDir,
		outputDir: outputDir,
		opts:      opts,
	}
}

func (h *OnDemandHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqPath := filepath.Clean(r.URL.Path)

	if reqPath == "/styles.css" || reqPath == "/mermaid-init.js" {
		filePath := filepath.Join(h.outputDir, reqPath)
		if _, err := os.Stat(filePath); err != nil {
			_ = WriteStaticAssets(h.outputDir)
		}
		http.ServeFile(w, r, filePath)
		return
	}

	parts := strings.Split(strings.Trim(reqPath, "/"), "/")

	// Handle index paths
	if reqPath == "/" || reqPath == "." || len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		h.serveRootIndex(w, r)
		return
	}
	if len(parts) == 2 {
		h.serveRepoIndex(w, r, parts[0], parts[1])
		return
	}
	if len(parts) == 3 {
		h.serveCategoryIndex(w, r, parts[0], parts[1], parts[2])
		return
	}

	if len(parts) != 4 {
		http.NotFound(w, r)
		return
	}

	owner, repo, category, filename := parts[0], parts[1], parts[2], parts[3]
	ext := filepath.Ext(filename)
	id := strings.TrimSuffix(filename, ext)
	format := strings.TrimPrefix(ext, ".")
	if format == "" {
		format = "html"
	}

	targetOutFile := filepath.Join(h.outputDir, owner, repo, category, filename)

	sourceJSONFile := h.sourceJSONPath(owner, repo, category, id)

	// Check disk cache hit
	if outStat, err := os.Stat(targetOutFile); err == nil {
		if srcStat, sErr := os.Stat(sourceJSONFile); sErr == nil {
			if outStat.ModTime().After(srcStat.ModTime()) {
				http.ServeFile(w, r, targetOutFile)
				return
			}
		} else if category == "job_runs" {
			// job_runs source JSON is the parent workflow run's file, not <jobID>.json.
			// If the output exists, serve it — job data is immutable once completed.
			http.ServeFile(w, r, targetOutFile)
			return
		} else if category == "comparisons" {
			// Comparisons are rendered by the compare command and have no single source JSON.
			// If the output exists, serve it directly.
			http.ServeFile(w, r, targetOutFile)
			return
		}
	}

	// Cache miss: render on demand with singleflight
	sfKey := fmt.Sprintf("%s/%s/%s/%s", owner, repo, category, id)
	_, err, _ := h.sf.Do(sfKey, func() (any, error) {
		return nil, h.renderEntity(owner, repo, category, id, format)
	})
	if err != nil {
		h.log.Error().Err(err).Str("path", reqPath).Msg("Failed to render observation on demand")
		http.Error(w, fmt.Sprintf("failed to render: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := os.Stat(targetOutFile); err == nil {
		http.ServeFile(w, r, targetOutFile)
	} else {
		http.NotFound(w, r)
	}
}

func (h *OnDemandHandler) sourceJSONPath(owner, repo, category, id string) string {
	switch category {
	case "workflow_runs", "job_runs":
		return filepath.Join(h.dataDir, owner, repo, gather.WorkflowRunsDataDir, id+".json")
	case "commits":
		return filepath.Join(h.dataDir, owner, repo, gather.CommitsDataDir, id+".json")
	case "pull_requests":
		return filepath.Join(h.dataDir, owner, repo, gather.PullRequestsDataDir, id+".json")
	default:
		return filepath.Join(h.dataDir, owner, repo, category, id+".json")
	}
}

func (h *OnDemandHandler) renderEntity(owner, repo, category, id, format string) error {
	allOpts := make([]Option, 0, len(h.opts)+2)
	allOpts = append(allOpts, WithCustomOutputDir(h.outputDir))
	allOpts = append(allOpts, h.opts...)
	allOpts = append(allOpts, WithGatherOptions(gather.CustomDataFolder(h.dataDir)))

	switch category {
	case "workflow_runs", "job_runs":
		var workflowRunID int64
		if category == "job_runs" {
			jobID, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid job run ID '%s': %w", id, err)
			}
			workflowRunID, err = gather.FindWorkflowRunIDForJob(h.dataDir, owner, repo, jobID)
			if err != nil {
				return fmt.Errorf("failed to find parent workflow run for job '%s': %w", id, err)
			}
		} else {
			var err error
			workflowRunID, err = strconv.ParseInt(id, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid workflow run ID '%s': %w", id, err)
			}
		}
		wfData, _, err := gather.WorkflowRun(
			h.log,
			h.client,
			owner,
			repo,
			workflowRunID,
			gather.CustomDataFolder(h.dataDir),
			gather.SkipMemoryCache(),
		)
		if err != nil {
			return err
		}
		obs, err := workflowRunObservation(wfData)
		if err != nil {
			return err
		}
		if _, err := obs.Render(h.log, format, WithCustomOutputDir(h.outputDir)); err != nil {
			return err
		}
		jObs, err := jobRunObservations(wfData)
		if err != nil {
			return err
		}
		for _, j := range jObs {
			if _, err := j.Render(h.log, format, WithCustomOutputDir(h.outputDir)); err != nil {
				return err
			}
		}
		return nil

	case "commits":
		obs, err := Commit(h.log, h.client, owner, repo, id, allOpts...)
		if err != nil {
			return err
		}
		_, err = obs.Render(h.log, format, WithCustomOutputDir(h.outputDir))
		return err

	case "pull_requests":
		prNum, err := strconv.Atoi(id)
		if err != nil {
			return fmt.Errorf("invalid pull request number '%s': %w", id, err)
		}
		obs, err := PullRequest(h.log, h.client, owner, repo, prNum, allOpts...)
		if err != nil {
			return err
		}
		_, err = obs.Render(h.log, format, WithCustomOutputDir(h.outputDir))
		return err

	default:
		return fmt.Errorf("unknown observation category: %s", category)
	}
}

func (h *OnDemandHandler) serveRootIndex(w http.ResponseWriter, r *http.Request) {
	targetFile := filepath.Join(h.outputDir, "index.html")
	if _, err := os.Stat(targetFile); err != nil {
		h.rebuildIndexes()
	}
	http.ServeFile(w, r, targetFile)
}

func (h *OnDemandHandler) serveRepoIndex(w http.ResponseWriter, r *http.Request, owner, repo string) {
	targetFile := filepath.Join(h.outputDir, owner, repo, "index.html")
	if _, err := os.Stat(targetFile); err != nil {
		h.rebuildIndexes()
	}
	http.ServeFile(w, r, targetFile)
}

func (h *OnDemandHandler) serveCategoryIndex(w http.ResponseWriter, r *http.Request, owner, repo, category string) {
	targetFile := filepath.Join(h.outputDir, owner, repo, category, "index.html")
	if _, err := os.Stat(targetFile); err != nil {
		h.rebuildIndexes()
	}
	http.ServeFile(w, r, targetFile)
}

func (h *OnDemandHandler) rebuildIndexes() {
	collected := make(map[categoryKey][]IndexItem)

	entries, err := os.ReadDir(h.dataDir)
	if err != nil {
		return
	}
	for _, ownerEntry := range entries {
		if !ownerEntry.IsDir() {
			continue
		}
		owner := ownerEntry.Name()
		repos, rErr := os.ReadDir(filepath.Join(h.dataDir, owner))
		if rErr != nil {
			continue
		}
		for _, repoEntry := range repos {
			if !repoEntry.IsDir() {
				continue
			}
			repo := repoEntry.Name()
			records, mErr := LoadManifest(h.dataDir, owner, repo)
			if mErr != nil || len(records) == 0 {
				_ = RebuildManifest(h.log, h.dataDir)
				records, _ = LoadManifest(h.dataDir, owner, repo)
			}
			for _, rec := range records {
				key := categoryKey{owner, repo, rec.Type + "s"}
				collected[key] = append(collected[key], IndexItem{
					Name:  rec.Name,
					Path:  rec.ID + ".html",
					State: rec.State,
					Actor: rec.Actor,
				})
			}
		}
	}

	_ = generateIndexPages(collected)
}
