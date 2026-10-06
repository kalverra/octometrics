package gather

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"
)

// RunnerSpecs represents the provisioned hardware configuration of a runner.
type RunnerSpecs struct {
	ProvisionedCPU   int     `json:"provisioned_cpu"`
	ProvisionedRAMGB float64 `json:"provisioned_ram_gb"`
	Family           string  `json:"family,omitempty"`
	Spot             bool    `json:"spot,omitempty"`
	Architecture     string  `json:"architecture,omitempty"`
	IsRunsOn         bool    `json:"is_runs_on"`
}

// ExtractRunnerSpecs inspects runner labels and runner name to deduce provisioned resources.
func ExtractRunnerSpecs(labels []string, runnerName string) RunnerSpecs {
	specs := RunnerSpecs{
		Architecture: "x64",
	}

	for _, label := range labels {
		if runsOnRealPattern.MatchString(label) {
			specs.IsRunsOn = true
			details := parseRealRunsOnDetails(label)
			if cpu, err := strconv.Atoi(details.CPU); err == nil && cpu > 0 {
				specs.ProvisionedCPU = cpu
			}
			if ram, err := strconv.ParseFloat(details.RAM, 64); err == nil && ram > 0 {
				specs.ProvisionedRAMGB = ram
			}
			if details.Family != "" {
				specs.Family = details.Family
			}
			specs.Spot = details.Spot == "true"
			if strings.Contains(details.Image, "arm64") {
				specs.Architecture = "arm64"
			}
			return specs
		}

		if matches := runsOnDocsPattern.FindStringSubmatch(label); len(matches) > 0 {
			specs.IsRunsOn = true
			if cpu, err := strconv.Atoi(matches[1]); err == nil {
				specs.ProvisionedCPU = cpu
				specs.ProvisionedRAMGB = float64(cpu * 4)
			}
			specs.Architecture = matches[3]
			return specs
		}
	}

	lower := strings.ToLower(runnerName)
	switch {
	case strings.Contains(lower, "64_core") || strings.Contains(lower, "64-core"):
		specs.ProvisionedCPU = 64
		specs.ProvisionedRAMGB = 256
	case strings.Contains(lower, "32_core") || strings.Contains(lower, "32-core"):
		specs.ProvisionedCPU = 32
		specs.ProvisionedRAMGB = 128
	case strings.Contains(lower, "16_core") || strings.Contains(lower, "16-core"):
		specs.ProvisionedCPU = 16
		specs.ProvisionedRAMGB = 64
	case strings.Contains(lower, "8_core") || strings.Contains(lower, "8-core"):
		specs.ProvisionedCPU = 8
		specs.ProvisionedRAMGB = 32
	case strings.Contains(lower, "4_core") || strings.Contains(lower, "4-core"):
		specs.ProvisionedCPU = 4
		specs.ProvisionedRAMGB = 16
	case strings.Contains(lower, "2_core") || strings.Contains(lower, "2-core") ||
		strings.Contains(lower, "ubuntu-latest") || strings.Contains(lower, "ubuntu-24.04") ||
		strings.Contains(lower, "ubuntu-22.04"):
		specs.ProvisionedCPU = 2
		specs.ProvisionedRAMGB = 7
	default:
		specs.ProvisionedCPU = 2
		specs.ProvisionedRAMGB = 7
	}

	if strings.Contains(lower, "arm") {
		specs.Architecture = "arm64"
	}

	return specs
}

// ListWorkflowRuns discovers workflow run IDs for a workflow file or ID.
// Falls back to scanning local disk cache if client is nil or API request fails.
func ListWorkflowRuns(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	workflowTarget string,
	branch string,
	status string,
	limit int,
	dataDir string,
) ([]int64, error) {
	if limit <= 0 {
		limit = 10
	}

	if runIDs := listWorkflowRunsFromAPI(
		ctx,
		client,
		owner,
		repo,
		workflowTarget,
		branch,
		status,
		limit,
	); len(
		runIDs,
	) > 0 {
		return runIDs, nil
	}

	return listWorkflowRunsFromCache(log, owner, repo, workflowTarget, branch, status, limit, dataDir)
}

// workflowRunsMaxPerPage matches GitHub's per-request page size cap.
const workflowRunsMaxPerPage = 100

func listWorkflowRunsFromAPI(
	ctx context.Context,
	client *GitHubClient,
	owner, repo, workflowTarget, branch, status string,
	limit int,
) []int64 {
	if client == nil || client.Rest == nil {
		return nil
	}
	perPage := min(limit, workflowRunsMaxPerPage)
	opts := &github.ListWorkflowRunsOptions{
		PerPage: perPage,
	}
	if branch != "" {
		opts.Branch = branch
	}
	if status != "" && status != "all" {
		opts.Status = status
	}

	fetchPage := func(page int) []*github.WorkflowRun {
		opts.Page = page
		cleanTarget := filepath.Base(workflowTarget)
		if wfID, err := strconv.ParseInt(cleanTarget, 10, 64); err == nil && wfID > 0 {
			res, _, apiErr := client.Rest.Actions.ListWorkflowRunsByID(ctx, owner, repo, wfID, opts)
			if apiErr == nil && res != nil {
				return res.WorkflowRuns
			}
			return nil
		}
		res, _, apiErr := client.Rest.Actions.ListWorkflowRunsByFileName(ctx, owner, repo, cleanTarget, opts)
		if apiErr == nil && res != nil {
			return res.WorkflowRuns
		}
		if workflowTarget != cleanTarget {
			res2, _, apiErr2 := client.Rest.Actions.ListWorkflowRunsByFileName(ctx, owner, repo, workflowTarget, opts)
			if apiErr2 == nil && res2 != nil {
				return res2.WorkflowRuns
			}
		}
		return nil
	}

	var runIDs []int64
	for page := 1; len(runIDs) < limit; page++ {
		pageRuns := fetchPage(page)
		if len(pageRuns) == 0 {
			break
		}
		for _, r := range pageRuns {
			if r != nil && r.GetID() > 0 {
				runIDs = append(runIDs, r.GetID())
			}
		}
		if len(pageRuns) < perPage {
			break
		}
	}
	if len(runIDs) == 0 {
		return nil
	}
	return runIDs
}

func isRunCacheMatch(runData *WorkflowRunData, cleanTarget, workflowTarget, targetName string) bool {
	runPath := filepath.Base(runData.GetPath())
	runName := runData.GetName()
	return runPath == cleanTarget ||
		runName == workflowTarget ||
		runName == targetName ||
		strings.EqualFold(runPath, cleanTarget) ||
		strings.EqualFold(runName, targetName)
}

func matchesRunFilter(runData *WorkflowRunData, branch, status string) bool {
	if branch != "" && runData.GetHeadBranch() != branch {
		return false
	}
	if status != "" && status != "all" {
		if status == "completed" && runData.GetStatus() != "completed" {
			return false
		}
		if status == "success" && runData.GetConclusion() != "success" {
			return false
		}
		if status == "failure" && runData.GetConclusion() != "failure" {
			return false
		}
	}
	return true
}

func listWorkflowRunsFromCache(
	log zerolog.Logger,
	owner, repo, workflowTarget, branch, status string,
	limit int,
	dataDir string,
) ([]int64, error) {
	log.Debug().Str("workflow", workflowTarget).Msg("listing workflow runs from local disk cache")
	wfDir := filepath.Join(dataDir, owner, repo, WorkflowRunsDataDir)
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"no runs found for workflow %q (local cache empty and no API client)",
				workflowTarget,
			)
		}
		return nil, fmt.Errorf("read local workflow runs directory: %w", err)
	}

	type runEntry struct {
		id        int64
		createdAt time.Time
	}
	var matched []runEntry
	cleanTarget := filepath.Base(workflowTarget)
	targetName := strings.TrimSuffix(cleanTarget, filepath.Ext(cleanTarget))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(wfDir, entry.Name())
		runData, readErr := readJSONFile[*WorkflowRunData](filePath)
		if readErr != nil || runData == nil || runData.WorkflowRun == nil {
			continue
		}

		if !isRunCacheMatch(runData, cleanTarget, workflowTarget, targetName) {
			continue
		}
		if !matchesRunFilter(runData, branch, status) {
			continue
		}

		t := runData.GetCreatedAt().Time
		if t.IsZero() {
			t = runData.GetRunStartedAt().Time
		}
		matched = append(matched, runEntry{id: runData.GetID(), createdAt: t})
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].createdAt.After(matched[j].createdAt)
	})

	var runIDs []int64
	for i, m := range matched {
		if i >= limit {
			break
		}
		runIDs = append(runIDs, m.id)
	}

	if len(runIDs) == 0 {
		return nil, fmt.Errorf("no matching runs found for workflow %q in local cache", workflowTarget)
	}
	return runIDs, nil
}

// EnsureJobMetrics ensures JobData has RunsOnMetrics or Analysis populated.
// Best-effort: log read or fetch failures are logged and skipped.
func EnsureJobMetrics(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	job *JobData,
	runID int64,
	dataDir string,
) {
	if job == nil {
		return
	}
	if job.RunsOnMetrics != nil || job.Analysis != nil {
		return
	}

	jobLogPath := filepath.Join(
		dataDir,
		owner,
		repo,
		"logs",
		strconv.FormatInt(runID, 10),
		fmt.Sprintf("%d.log", job.GetID()),
	)
	if !cacheFileExists(jobLogPath) {
		jobLogPath = filepath.Join(dataDir, owner, repo, "logs", fmt.Sprintf("%d.log", job.GetID()))
	}

	if cacheFileExists(jobLogPath) {
		//nolint:gosec // job log path is safe
		content, err := os.ReadFile(jobLogPath)
		if err != nil {
			log.Debug().Err(err).Int64("job_id", job.GetID()).Msg("failed to read cached job log for metrics")
		} else if metrics, ok := ParseRunsOnJobMetrics(string(content)); ok {
			job.RunsOnMetrics = metrics
			return
		}
	}

	if client != nil {
		cleanedLogs, err := GetCleanJobLogs(ctx, log, client, owner, repo, job.GetID(), dataDir)
		if err != nil {
			log.Debug().Err(err).Int64("job_id", job.GetID()).Msg("failed to fetch job logs for metrics")
		} else if cleanedLogs != "" {
			if metrics, ok := ParseRunsOnJobMetrics(cleanedLogs); ok {
				job.RunsOnMetrics = metrics
			}
		}
	}
}
