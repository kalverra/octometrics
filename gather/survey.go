package gather

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/internal/config"
)

// SurveysDataDir is the directory name for storing survey result files.
const SurveysDataDir = "surveys"

// SurveyResult holds the outcome of a CI suite survey across a time period.
type SurveyResult struct {
	Owner       string                   `json:"owner"`
	Repo        string                   `json:"repo"`
	Event       string                   `json:"event"`
	Since       time.Time                `json:"since"`
	Until       time.Time                `json:"until"`
	TotalRuns   int                      `json:"total_runs"`
	Commits     []*CommitSurvey          `json:"commits"`
	Percentiles map[string]*CommitSurvey `json:"percentiles"`
}

// CommitSurvey holds lightweight timing data for all workflow runs on a single commit.
type CommitSurvey struct {
	SHA          string                `json:"sha"`
	Event        string                `json:"event"`
	StartTime    time.Time             `json:"start_time"`
	EndTime      time.Time             `json:"end_time"`
	Duration     time.Duration         `json:"duration"`
	WorkflowRuns []*WorkflowRunSummary `json:"workflow_runs"`
	Conclusion   string                `json:"conclusion"`
}

// WorkflowRunSummary is a lightweight representation of a workflow run for survey purposes.
type WorkflowRunSummary struct {
	ID         int64         `json:"id"`
	WorkflowID int64         `json:"workflow_id"`
	Name       string        `json:"name"`
	Event      string        `json:"event"`
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
	Duration   time.Duration `json:"duration"`
	Conclusion string        `json:"conclusion"`
	RunAttempt int           `json:"run_attempt"`
}

// Survey lists all completed workflow runs in a time period, groups them by commit,
// computes per-commit CI suite duration, and identifies p50/p75/p95 representative commits.
func Survey(
	log zerolog.Logger,
	client *GitHubClient,
	cfg *config.Config,
) (*SurveyResult, error) {
	log = log.With().
		Str("owner", cfg.Owner).
		Str("repo", cfg.Repo).
		Str("event", cfg.Event).
		Time("since", cfg.Since).
		Time("until", cfg.Until).
		Logger()

	targetDir := filepath.Join(cfg.DataDir, cfg.Owner, cfg.Repo, SurveysDataDir)
	targetFile := filepath.Join(targetDir, surveyFileName(cfg.Event, cfg.Since, cfg.Until))

	if !cfg.ForceUpdate {
		if _, err := os.Stat(targetFile); err == nil {
			log.Debug().Str("source", "local file").Msg("Loading cached survey result")
			data, err := os.ReadFile(filepath.Clean(targetFile))
			if err != nil {
				return nil, fmt.Errorf("failed to read survey file: %w", err)
			}
			var result SurveyResult
			if err := json.Unmarshal(data, &result); err != nil {
				return nil, fmt.Errorf("failed to unmarshal survey data: %w", err)
			}
			return &result, nil
		}
	}

	if client == nil {
		return nil, fmt.Errorf("GitHub client is nil")
	}

	log.Info().Msg("Surveying workflow runs")
	startTime := time.Now()

	runs, err := listAllWorkflowRuns(log, client, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}

	log.Info().Int("total_runs", len(runs)).Str("duration", time.Since(startTime).String()).Msg("Listed workflow runs")

	commits := groupAndComputeDurations(runs)
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Duration < commits[j].Duration
	})

	percentiles := computePercentiles(commits)

	result := &SurveyResult{
		Owner:       cfg.Owner,
		Repo:        cfg.Repo,
		Event:       cfg.Event,
		Since:       cfg.Since,
		Until:       cfg.Until,
		TotalRuns:   len(runs),
		Commits:     commits,
		Percentiles: percentiles,
	}

	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create survey dir: %w", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal survey result: %w", err)
	}
	if err := os.WriteFile(targetFile, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write survey file: %w", err)
	}

	log.Info().
		Int("commits_analyzed", len(commits)).
		Str("duration", time.Since(startTime).String()).
		Msg("Survey complete")

	return result, nil
}

func surveyFileName(event string, since, until time.Time) string {
	return SurveyFileBaseName(event, since, until) + ".json"
}

// SurveyFileBaseName returns the base name (without extension) for a survey file.
func SurveyFileBaseName(event string, since, until time.Time) string {
	e := event
	if e == "" {
		e = "all"
	}
	return fmt.Sprintf("%s_%s_%s", e, since.Format("2006-01-02"), until.Format("2006-01-02"))
}

// listAllWorkflowRuns paginates through ListRepositoryWorkflowRuns for a date range.
func listAllWorkflowRuns(
	log zerolog.Logger,
	client *GitHubClient,
	cfg *config.Config,
) ([]*github.WorkflowRun, error) {
	var allRuns []*github.WorkflowRun

	listOpts := &github.ListWorkflowRunsOptions{
		Status:  "completed",
		Created: fmt.Sprintf("%s..%s", cfg.Since.Format("2006-01-02"), cfg.Until.Format("2006-01-02")),
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	if cfg.Event != "" {
		listOpts.Event = cfg.Event
	}

	for {
		ctx, cancel := ghCtx()
		result, resp, err := client.Rest.Actions.ListRepositoryWorkflowRuns(ctx, cfg.Owner, cfg.Repo, listOpts)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to list workflow runs: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}

		allRuns = append(allRuns, result.WorkflowRuns...)

		log.Debug().
			Int("fetched", len(allRuns)).
			Int("total_count", result.GetTotalCount()).
			Int("page", listOpts.Page).
			Msg("Fetching workflow runs")

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return allRuns, nil
}

type workflowRunKey struct {
	HeadSHA    string
	WorkflowID int64
}

// groupAndComputeDurations groups workflow runs by commit SHA, deduplicates by workflow
// (keeping latest attempt), and computes the total CI wall-clock duration per commit.
// TODO: Enable event filtering
func groupAndComputeDurations(runs []*github.WorkflowRun) []*CommitSurvey {
	// First, deduplicate: for each (HeadSHA, WorkflowID), keep only the latest RunAttempt
	best := make(map[workflowRunKey]*github.WorkflowRun)
	for _, run := range runs {
		if run.GetStatus() != "completed" {
			continue
		}
		key := workflowRunKey{
			HeadSHA:    run.GetHeadSHA(),
			WorkflowID: run.GetWorkflowID(),
		}
		existing, ok := best[key]
		if !ok || run.GetRunAttempt() > existing.GetRunAttempt() {
			best[key] = run
		}
	}

	// Group deduplicated runs by HeadSHA
	groups := make(map[string][]*github.WorkflowRun)
	for _, run := range best {
		sha := run.GetHeadSHA()
		groups[sha] = append(groups[sha], run)
	}

	var commits []*CommitSurvey
	for sha, shaRuns := range groups {
		if len(shaRuns) == 0 {
			continue
		}

		// Skip commits where any run is not completed
		allCompleted := true
		for _, run := range shaRuns {
			if run.GetConclusion() == "" {
				allCompleted = false
				break
			}
		}
		if !allCompleted {
			continue
		}

		var (
			earliest    time.Time
			latest      time.Time
			conclusion  = "success"
			summaries   []*WorkflowRunSummary
			commitEvent = shaRuns[0].GetEvent()
		)

		for _, run := range shaRuns {
			runStart := run.GetRunStartedAt().Time
			runEnd := run.GetUpdatedAt().Time

			if earliest.IsZero() || runStart.Before(earliest) {
				earliest = runStart
			}
			if runEnd.After(latest) {
				latest = runEnd
			}

			runConclusion := run.GetConclusion()
			if runConclusion == "failure" || runConclusion == "timed_out" {
				conclusion = "failure"
			}

			summaries = append(summaries, &WorkflowRunSummary{
				ID:         run.GetID(),
				WorkflowID: run.GetWorkflowID(),
				Name:       run.GetName(),
				Event:      run.GetEvent(),
				StartTime:  runStart,
				EndTime:    runEnd,
				Duration:   runEnd.Sub(runStart),
				Conclusion: runConclusion,
				RunAttempt: run.GetRunAttempt(),
			})
		}

		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].StartTime.Before(summaries[j].StartTime)
		})

		duration := latest.Sub(earliest)
		if duration <= 0 {
			continue
		}

		commits = append(commits, &CommitSurvey{
			SHA:          sha,
			Event:        commitEvent,
			StartTime:    earliest,
			EndTime:      latest,
			Duration:     duration,
			WorkflowRuns: summaries,
			Conclusion:   conclusion,
		})
	}

	return commits
}

// computePercentiles picks the commits at the p50, p75, and p95 positions from a sorted slice.
func computePercentiles(sortedCommits []*CommitSurvey) map[string]*CommitSurvey {
	result := make(map[string]*CommitSurvey)
	n := len(sortedCommits)
	if n == 0 {
		return result
	}

	percentiles := map[string]float64{
		"p50": 0.50,
		"p75": 0.75,
		"p95": 0.95,
	}

	for label, p := range percentiles {
		idx := max(int(math.Ceil(p*float64(n)))-1, 0)
		if idx >= n {
			idx = n - 1
		}
		result[label] = sortedCommits[idx]
	}

	return result
}
