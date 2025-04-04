package observe

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/gather"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const jobRunOutputDir = "job_runs"

func JobRuns(
	log zerolog.Logger,
	client *github.Client,
	owner, repo string,
	workflowRunID int64,
	opts ...Option,
) error {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	workflowRun, _, err := gather.WorkflowRun(log, client, owner, repo, workflowRunID, options.gatherOptions...)
	if err != nil {
		return err
	}

	var (
		startTime = time.Now()
		eg        errgroup.Group
	)

	for _, job := range workflowRun.Jobs {
		eg.Go(func() error {
			jobRunTemplateData, err := buildJobRunGanttData(owner, repo, job)
			if err != nil {
				return fmt.Errorf("failed to build gantt chart for job '%d': %w", job.GetID(), err)
			}

			err = renderGantt(jobRunTemplateData, options.outputTypes)
			if err != nil {
				return fmt.Errorf("failed to render gantt chart for job '%d': %w", job.GetID(), err)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to observe job runs: %w", err)
	}

	log.Trace().
		Int64("workflow_run_id", workflowRunID).
		Int("job_count", len(workflowRun.Jobs)).
		Str("duration", time.Since(startTime).String()).
		Msg("Observed job runs")
	return nil
}

func buildJobRunGanttData(owner, repo string, job *gather.JobData) (*ganttData, error) {
	tasks := make([]ganttItem, 0, len(job.Steps))
	for _, step := range job.Steps {
		startTime := step.GetStartedAt().Time
		duration := step.GetCompletedAt().Sub(startTime)
		newTask := ganttItem{
			Name:       step.GetName(),
			StartTime:  step.GetStartedAt().Time,
			Conclusion: conclusionToGanntStatus(step.GetConclusion()),
			Duration:   duration,
		}
		if step.GetConclusion() == "skipped" {
			newTask.Name = fmt.Sprintf("%s (skipped)", step.GetName())
		}
		tasks = append(tasks, newTask)
	}

	return &ganttData{
		ID:       fmt.Sprint(job.GetID()),
		Name:     fmt.Sprintf("Job Run %s, ID: %d", job.GetName(), job.GetID()),
		Link:     job.GetHTMLURL(),
		Items:    tasks,
		Owner:    owner,
		Repo:     repo,
		Cost:     job.GetCost(),
		DataType: "job_run",
	}, nil
}

// jobRunLink returns the link to a specific job run's rendering.
// You need to add on the extension (.html, .md) to this path.
func jobRunLink(owner, repo string, jobRunID int64) string {
	return filepath.Join("/", owner, repo, jobRunOutputDir, fmt.Sprint(jobRunID))
}
