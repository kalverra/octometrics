package observe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/kalverra/octometrics/gather"
)

func (h *OnDemandHandler) serveGatherAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "gather" {
		http.NotFound(w, r)
		return
	}

	owner := parts[2]
	repo := parts[3]
	action := parts[4]

	if h.client == nil {
		http.Error(w, `{"error":"GitHub client not initialized"}`, http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	opts := []gather.Option{
		gather.CustomDataFolder(h.dataDir),
		gather.WithCost(),
	}

	switch action {
	case "workflow":
		idStr := r.URL.Query().Get("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"Invalid ID"}`, http.StatusBadRequest)
			return
		}
		_, _, err = gather.WorkflowRun(ctx, h.log, h.client, owner, repo, id, opts...)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "url": fmt.Sprintf("/%s/%s/workflow_runs/%d.html", owner, repo, id)})

	case "pull":
		numStr := r.URL.Query().Get("number")
		num, err := strconv.Atoi(numStr)
		if err != nil {
			http.Error(w, `{"error":"Invalid number"}`, http.StatusBadRequest)
			return
		}
		_, err = gather.PullRequest(ctx, h.log, h.client, owner, repo, num, opts...)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "url": fmt.Sprintf("/%s/%s/pull_requests/%d.html", owner, repo, num)})

	case "commit":
		sha := r.URL.Query().Get("sha")
		if sha == "" {
			http.Error(w, `{"error":"Missing SHA"}`, http.StatusBadRequest)
			return
		}
		_, err := gather.Commit(ctx, h.log, h.client, owner, repo, sha, opts...)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "url": fmt.Sprintf("/%s/%s/commits/%s.html", owner, repo, sha)})

	default:
		http.NotFound(w, r)
	}
}
