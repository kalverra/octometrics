package observe

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/uistate"
)

type homeViewModel struct {
	Favorites    []uistate.RepoRef
	Recents      []uistate.RepoRef
	NotConnected bool
}

// LocalMatch represents a search result matched against local manifest records.
type LocalMatch struct {
	Type  string
	ID    string
	Name  string
	State string
	Actor string
	Owner string
	Repo  string
}

type searchViewModel struct {
	Query        string
	GitHubRepos  []gather.RepoSummary
	GitHubPRs    []gather.PRSummary
	LocalMatches []LocalMatch
	NotConnected bool
}

type repoViewModel struct {
	Repo         *gather.RepoSummary
	Owner        string
	Name         string
	IsFavorite   bool
	ActiveTab    string
	WorkflowID   int64
	Query        string
	Workflows    []gather.WorkflowSummary
	Runs         []gather.RunSummary
	Commits      []gather.CommitSummary
	PRs          []gather.PRSummary
	NotConnected bool
}

type pendingViewModel struct {
	EntityName string
	TargetURL  string
	ErrorMsg   string
}

func (h *OnDemandHandler) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	vm := homeViewModel{
		Favorites:    h.uiState.Favorites,
		Recents:      h.uiState.Recents,
		NotConnected: h.client == nil || h.client.Rest == nil,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := htmlTemplate.ExecuteTemplate(w, "home", vm); err != nil {
		h.log.Error().Err(err).Msg("failed to render home page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *OnDemandHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	isPartial := r.URL.Query().Get("partial") == "1"

	vm := searchViewModel{
		Query:        query,
		NotConnected: h.client == nil || h.client.Rest == nil,
	}

	if query != "" {
		if !vm.NotConnected {
			repos, err := gather.SearchRepos(r.Context(), h.log, h.client, query, 10)
			if err != nil {
				h.log.Warn().Err(err).Str("query", query).Msg("github search repos failed")
			} else {
				vm.GitHubRepos = repos
			}

			prs, err := gather.SearchPullRequests(r.Context(), h.log, h.client, query, 10)
			if err != nil {
				h.log.Warn().Err(err).Str("query", query).Msg("github search prs failed")
			} else {
				// Mark downloaded PRs
				for i := range prs {
					dlMap := h.loadDownloadedMap(prs[i].Owner, prs[i].Repo)
					prs[i].Downloaded = dlMap[fmt.Sprintf("pull_request:%d", prs[i].Number)]
				}
				vm.GitHubPRs = prs
			}
		}

		vm.LocalMatches = h.searchLocalManifest(query)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmplName := "search"
	if isPartial {
		tmplName = "search_results"
	}
	if err := htmlTemplate.ExecuteTemplate(w, tmplName, vm); err != nil {
		h.log.Error().Err(err).Msg("failed to render search page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *OnDemandHandler) handleRepo(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")

	if owner == "" || repo == "" {
		http.NotFound(w, r)
		return
	}

	_ = h.uiState.TouchRecent(owner, repo)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "workflows"
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var wfID int64
	if wfStr := r.URL.Query().Get("workflow_id"); wfStr != "" {
		if parsed, err := strconv.ParseInt(wfStr, 10, 64); err == nil {
			wfID = parsed
		}
	}

	dlMap := h.loadDownloadedMap(owner, repo)

	vm := repoViewModel{
		Owner:        owner,
		Name:         repo,
		IsFavorite:   h.uiState.IsFavorite(owner, repo),
		ActiveTab:    tab,
		WorkflowID:   wfID,
		Query:        query,
		NotConnected: h.client == nil || h.client.Rest == nil,
	}

	if !vm.NotConnected {
		repoSummary, err := gather.RepoInfo(r.Context(), h.log, h.client, owner, repo)
		if err != nil {
			h.log.Warn().Err(err).Str("owner", owner).Str("repo", repo).Msg("failed to fetch repo info")
		} else {
			vm.Repo = repoSummary
		}
	}

	switch tab {
	case "workflows":
		h.populateWorkflowsTab(r.Context(), &vm, owner, repo, wfID, query, dlMap)
	case "commits":
		h.populateCommitsTab(r.Context(), &vm, owner, repo, query, dlMap)
	case "pulls":
		h.populatePullsTab(r.Context(), &vm, owner, repo, query, dlMap)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := htmlTemplate.ExecuteTemplate(w, "repo", vm); err != nil {
		h.log.Error().Err(err).Msg("failed to render repo page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *OnDemandHandler) populateWorkflowsTab(
	ctx context.Context,
	vm *repoViewModel,
	owner, repo string,
	wfID int64,
	query string,
	dlMap map[string]bool,
) {
	if !vm.NotConnected {
		wfs, err := gather.ListWorkflows(ctx, h.log, h.client, owner, repo)
		if err != nil {
			h.log.Warn().Err(err).Msg("failed to list workflows")
		} else {
			vm.Workflows = wfs
		}

		runs, err := gather.ListRuns(ctx, h.log, h.client, owner, repo, wfID, 20)
		if err != nil {
			h.log.Warn().Err(err).Msg("failed to list runs")
		} else {
			for i := range runs {
				runs[i].Downloaded = dlMap[fmt.Sprintf("workflow_run:%d", runs[i].ID)]
			}
			vm.Runs = runs
		}
	} else {
		records, _ := LoadManifest(h.dataDir, owner, repo)
		for _, rec := range records {
			if rec.Type == "workflow_run" {
				id, _ := strconv.ParseInt(rec.ID, 10, 64)
				vm.Runs = append(vm.Runs, gather.RunSummary{
					ID:         id,
					Name:       rec.Name,
					Status:     rec.State,
					Conclusion: rec.State,
					Actor:      rec.Actor,
					Downloaded: true,
				})
			}
		}
	}
	vm.Runs = filterRuns(vm.Runs, query)
}

func (h *OnDemandHandler) populateCommitsTab(
	ctx context.Context,
	vm *repoViewModel,
	owner, repo string,
	query string,
	dlMap map[string]bool,
) {
	if !vm.NotConnected {
		commits, err := gather.ListCommits(ctx, h.log, h.client, owner, repo, 20)
		if err != nil {
			h.log.Warn().Err(err).Msg("failed to list commits")
		} else {
			for i := range commits {
				commits[i].Downloaded = dlMap[fmt.Sprintf("commit:%s", commits[i].SHA)]
			}
			vm.Commits = commits
		}
	} else {
		records, _ := LoadManifest(h.dataDir, owner, repo)
		for _, rec := range records {
			if rec.Type == "commit" {
				isMQ := strings.Contains(rec.Name, "gh-readonly-queue") ||
					strings.Contains(strings.ToLower(rec.Name), "merge queue")
				branch := ""
				if isMQ {
					branch = "merge-queue"
				}
				vm.Commits = append(vm.Commits, gather.CommitSummary{
					SHA:          rec.ID,
					Message:      rec.Name,
					Author:       rec.Actor,
					Branch:       branch,
					IsMergeQueue: isMQ,
					Downloaded:   true,
				})
			}
		}
	}

	defaultBranch := "main"
	if vm.Repo != nil && vm.Repo.DefaultBranch != "" {
		defaultBranch = vm.Repo.DefaultBranch
	}
	for i := range vm.Commits {
		if vm.Commits[i].Branch == "" {
			if vm.Commits[i].IsMergeQueue {
				vm.Commits[i].Branch = "merge-queue"
			} else {
				vm.Commits[i].Branch = defaultBranch
			}
		}
	}

	vm.Commits = filterCommits(vm.Commits, query)
}

func (h *OnDemandHandler) populatePullsTab(
	ctx context.Context,
	vm *repoViewModel,
	owner, repo string,
	query string,
	dlMap map[string]bool,
) {
	if !vm.NotConnected {
		prs, err := gather.ListPullRequests(ctx, h.log, h.client, owner, repo, 20)
		if err != nil {
			h.log.Warn().Err(err).Msg("failed to list prs")
		} else {
			for i := range prs {
				prs[i].Downloaded = dlMap[fmt.Sprintf("pull_request:%d", prs[i].Number)]
			}
			vm.PRs = prs
		}
	} else {
		records, _ := LoadManifest(h.dataDir, owner, repo)
		for _, rec := range records {
			if rec.Type == "pull_request" {
				num, _ := strconv.Atoi(rec.ID)
				vm.PRs = append(vm.PRs, gather.PRSummary{
					Number:     num,
					Title:      rec.Name,
					State:      rec.State,
					Actor:      rec.Actor,
					Owner:      owner,
					Repo:       repo,
					Downloaded: true,
				})
			}
		}
	}
	vm.PRs = filterPRs(vm.PRs, query)
}

func filterRuns(runs []gather.RunSummary, query string) []gather.RunSummary {

	if query == "" {
		return runs
	}
	qLower := strings.ToLower(query)
	var filtered []gather.RunSummary
	for _, run := range runs {
		if strings.Contains(strings.ToLower(fmt.Sprint(run.ID)), qLower) ||
			strings.Contains(strings.ToLower(run.Name), qLower) ||
			strings.Contains(strings.ToLower(run.HeadSHA), qLower) ||
			strings.Contains(strings.ToLower(run.HeadBranch), qLower) ||
			strings.Contains(strings.ToLower(run.Actor), qLower) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func filterCommits(commits []gather.CommitSummary, query string) []gather.CommitSummary {
	if query == "" {
		return commits
	}
	qLower := strings.ToLower(query)
	var filtered []gather.CommitSummary
	for _, c := range commits {
		if strings.Contains(strings.ToLower(c.SHA), qLower) ||
			strings.Contains(strings.ToLower(c.Message), qLower) ||
			strings.Contains(strings.ToLower(c.Author), qLower) ||
			strings.Contains(strings.ToLower(c.Branch), qLower) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func filterPRs(prs []gather.PRSummary, query string) []gather.PRSummary {
	if query == "" {
		return prs
	}
	qLower := strings.ToLower(query)
	var filtered []gather.PRSummary
	for _, pr := range prs {
		if strings.Contains(strings.ToLower(fmt.Sprint(pr.Number)), qLower) ||
			strings.Contains(strings.ToLower(pr.Title), qLower) ||
			strings.Contains(strings.ToLower(pr.Actor), qLower) {
			filtered = append(filtered, pr)
		}
	}
	return filtered
}

func (h *OnDemandHandler) handleFavorites(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodPost {
		owner := r.FormValue("owner")
		repo := r.FormValue("repo")
		if owner != "" && repo != "" {
			_ = h.uiState.ToggleFavorite(owner, repo)
		}
	}

	referer := r.Header.Get("Referer")
	target := "/"
	if referer != "" {
		if strings.HasPrefix(referer, "/") {
			target = referer
		} else if strings.HasPrefix(referer, "http://") || strings.HasPrefix(referer, "https://") {
			target = referer
		}
	}
	//nolint:gosec // safe redirect to sanitized local or http(s) URL
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *OnDemandHandler) loadDownloadedMap(owner, repo string) map[string]bool {
	m := make(map[string]bool)
	records, err := LoadManifest(h.dataDir, owner, repo)
	if err != nil {
		return m
	}
	for _, r := range records {
		m[fmt.Sprintf("%s:%s", r.Type, r.ID)] = true
	}
	return m
}

func (h *OnDemandHandler) searchLocalManifest(query string) []LocalMatch {
	q := strings.ToLower(query)
	var results []LocalMatch

	entries, err := os.ReadDir(h.dataDir)
	if err != nil {
		return nil
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
			if mErr != nil {
				continue
			}
			for _, rec := range records {
				if strings.Contains(strings.ToLower(rec.Name), q) ||
					strings.Contains(strings.ToLower(rec.ID), q) ||
					strings.Contains(strings.ToLower(rec.Actor), q) ||
					strings.Contains(strings.ToLower(owner), q) ||
					strings.Contains(strings.ToLower(repo), q) {
					results = append(results, LocalMatch{
						Type:  rec.Type,
						ID:    rec.ID,
						Name:  rec.Name,
						State: rec.State,
						Actor: rec.Actor,
						Owner: owner,
						Repo:  repo,
					})
				}
			}
		}
	}

	return results
}
