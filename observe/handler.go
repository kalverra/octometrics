package observe

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/uistate"
)

type gatherJob struct {
	done bool
	err  error
}

// OnDemandHandler serves observations, rendering them lazily if missing or stale.
type OnDemandHandler struct {
	log       zerolog.Logger
	client    *gather.GitHubClient
	dataDir   string
	outputDir string
	opts      []Option
	uiState   *uistate.State
	jobs      map[string]*gatherJob
	jobsMu    sync.Mutex
	mux       *http.ServeMux
}

// NewOnDemandHandler creates a new OnDemandHandler.
func NewOnDemandHandler(
	log zerolog.Logger,
	client *gather.GitHubClient,
	dataDir string,
	outputDir string,
	opts ...Option,
) *OnDemandHandler {
	st, err := uistate.Load(dataDir)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load ui state, starting clean")
		st, _ = uistate.Load("")
	}

	h := &OnDemandHandler{
		log:       log,
		client:    client,
		dataDir:   dataDir,
		outputDir: outputDir,
		opts:      opts,
		uiState:   st,
		jobs:      make(map[string]*gatherJob),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.handleHome)
	mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) {
		//nolint:gosec // safe redirect to home
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /search", h.handleSearch)
	mux.HandleFunc("GET /favorites", h.handleFavorites)
	mux.HandleFunc("POST /favorites", h.handleFavorites)
	mux.HandleFunc("GET /styles.css", h.handleStatic)
	mux.HandleFunc("GET /mermaid-init.js", h.handleStatic)
	mux.HandleFunc("GET /export-png.js", h.handleStatic)
	mux.HandleFunc("GET /search.js", h.handleStatic)
	mux.HandleFunc("GET /{owner}/{repo}", h.handleRepo)
	mux.HandleFunc("GET /{owner}/{repo}/index.html", func(w http.ResponseWriter, r *http.Request) {
		owner := r.PathValue("owner")
		repo := r.PathValue("repo")
		//nolint:gosec // safe redirect to repo page
		http.Redirect(w, r, fmt.Sprintf("/%s/%s", owner, repo), http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /{owner}/{repo}/{category}/index.html", func(w http.ResponseWriter, r *http.Request) {
		owner := r.PathValue("owner")
		repo := r.PathValue("repo")
		cat := r.PathValue("category")
		tab := "workflows"
		switch cat {
		case "commits":
			tab = "commits"
		case "pull_requests":
			tab = "pulls"
		}
		//nolint:gosec // safe redirect to repo tab
		http.Redirect(w, r, fmt.Sprintf("/%s/%s?tab=%s", owner, repo, tab), http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /{owner}/{repo}/{category}/{filename}", h.handleEntity)

	h.mux = mux
	return h
}

func (h *OnDemandHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	h.mux.ServeHTTP(w, r)
}

func (h *OnDemandHandler) handleStatic(w http.ResponseWriter, r *http.Request) {
	reqPath := filepath.Clean(r.URL.Path)
	filePath := filepath.Join(h.outputDir, reqPath)
	if _, err := os.Stat(filePath); err != nil {
		_ = WriteStaticAssets(h.outputDir)
	}
	http.ServeFile(w, r, filePath)
}

func (h *OnDemandHandler) handleEntity(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	category := r.PathValue("category")
	filename := r.PathValue("filename")

	ext := filepath.Ext(filename)
	id := strings.TrimSuffix(filename, ext)
	format := strings.TrimPrefix(ext, ".")
	if format == "" {
		format = "html"
	}

	targetOutFile := filepath.Clean(filepath.Join(h.outputDir, owner, repo, category, filename))
	sourceJSONFile := h.sourceJSONPath(owner, repo, category, id)

	// Check disk cache hit
	jobKey := fmt.Sprintf("%s/%s/%s/%s", owner, repo, category, id)
	entityName := fmt.Sprintf("%s/%s %s %s", owner, repo, category, id)

	if outStat, err := os.Stat(targetOutFile); err == nil {
		if srcStat, sErr := os.Stat(sourceJSONFile); sErr == nil {
			if outStat.ModTime().After(srcStat.ModTime()) {
				h.jobsMu.Lock()
				delete(h.jobs, jobKey)
				h.jobsMu.Unlock()
				http.ServeFile(w, r, targetOutFile)
				return
			}
		} else if category == "job_runs" || category == "comparisons" {
			h.jobsMu.Lock()
			delete(h.jobs, jobKey)
			h.jobsMu.Unlock()
			http.ServeFile(w, r, targetOutFile)
			return
		}
	}

	// Cache miss: interstitial job model
	h.jobsMu.Lock()
	job, exists := h.jobs[jobKey]
	if exists && job.done {
		if job.err != nil {
			err := job.err
			delete(h.jobs, jobKey)
			h.jobsMu.Unlock()

			w.WriteHeader(http.StatusAccepted)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = htmlTemplate.ExecuteTemplate(w, "pending", pendingViewModel{
				EntityName: entityName,
				ErrorMsg:   err.Error(),
			})
			return
		}
		// Stale completed job; delete it so a fresh job starts
		delete(h.jobs, jobKey)
		exists = false
	}

	if exists && !job.done {
		h.jobsMu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = htmlTemplate.ExecuteTemplate(w, "pending", pendingViewModel{
			EntityName: entityName,
		})
		return
	}

	// No job -> start rendering in background with context.Background()
	newJob := &gatherJob{done: false}
	h.jobs[jobKey] = newJob
	h.jobsMu.Unlock()

	go func() {
		err := h.renderEntity(context.Background(), owner, repo, category, id, format)
		h.jobsMu.Lock()
		newJob.err = err
		newJob.done = true
		h.jobsMu.Unlock()
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = htmlTemplate.ExecuteTemplate(w, "pending", pendingViewModel{
		EntityName: entityName,
	})
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

func (h *OnDemandHandler) renderEntity(ctx context.Context, owner, repo, category, id, format string) error {
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
			ctx,
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
		obs, err := Commit(ctx, h.log, h.client, owner, repo, id, allOpts...)
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
		obs, err := PullRequest(ctx, h.log, h.client, owner, repo, prNum, allOpts...)
		if err != nil {
			return err
		}
		_, err = obs.Render(h.log, format, WithCustomOutputDir(h.outputDir))
		return err

	default:
		return fmt.Errorf("unknown observation category: %s", category)
	}
}
