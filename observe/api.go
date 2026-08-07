package observe

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/go-github/v89/github"
)

func (h *OnDemandHandler) serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "github" {
		http.NotFound(w, r)
		return
	}

	owner := parts[2]
	repo := parts[3]
	action := ""
	if len(parts) > 4 {
		action = parts[4]
	}

	if h.client == nil || h.client.Rest == nil {
		http.Error(w, `{"error":"GitHub client not initialized"}`, http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	switch action {
	case "workflows":
		runs, _, err := h.client.Rest.Actions.ListRepositoryWorkflowRuns(
			ctx,
			owner,
			repo,
			&github.ListWorkflowRunsOptions{
				ListOptions: github.ListOptions{PerPage: 15},
			},
		)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(runs.WorkflowRuns)
	case "pulls":
		pulls, _, err := h.client.Rest.PullRequests.List(
			ctx,
			owner,
			repo,
			&github.PullRequestListOptions{
				State:       "all",
				Sort:        "updated",
				Direction:   "desc",
				ListOptions: github.ListOptions{PerPage: 15},
			},
		)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(pulls)
	case "commits":
		commits, _, err := h.client.Rest.Repositories.ListCommits(
			ctx,
			owner,
			repo,
			&github.CommitsListOptions{
				ListOptions: github.ListOptions{PerPage: 15},
			},
		)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(commits)
	default:
		http.NotFound(w, r)
	}
}
