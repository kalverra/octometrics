package observe

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/workflow-metrics/gather"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const jobRunOutputDir = "job_runs"

func JobRuns(client *github.Client, owner, repo string, workflowRunID int64, outputTypes []string) error {
	workflowRun, err := gather.WorkflowRun(client, owner, repo, workflowRunID, false)
	if err != nil {
		return err
	}

	var (
		startTime = time.Now()
		eg        errgroup.Group
	)

	for _, job := range workflowRun.Jobs {
		eg.Go(func() error {
			jobRunTemplateData, err := buildJobRunGanttData(owner, repo, workflowRunID, job)
			if err != nil {
				return fmt.Errorf("failed to build gantt chart for job '%d': %w", job.GetID(), err)
			}

			err = renderGantt(jobRunTemplateData, outputTypes)
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

func buildJobRunGanttData(owner, repo string, workflowRunID int64, job *gather.JobData) (*ganttData, error) {
	mermaidDateFormat, mermaidAxisFormat, goDateFormat := ganttDetermineDateFormat(
		job.GetStartedAt().Time,
		job.GetCompletedAt().Time,
	)

	tasks := make([]ganttItem, 0, len(job.Steps))
	for _, step := range job.Steps {
		if step.GetConclusion() == "skipped" {
			continue
		}

		saniName := strings.ReplaceAll(step.GetName(), " ", "_")
		startTime := step.GetStartedAt().Time
		duration := step.GetCompletedAt().Sub(startTime)
		tasks = append(tasks, ganttItem{
			Name:       step.GetName(),
			MermaidID:  saniName,
			StartTime:  step.GetStartedAt().Time,
			Conclusion: conclusionToGanntStatus(step.GetConclusion()),
			Duration:   duration,
		})
	}

	return &ganttData{
		ID:           fmt.Sprint(job.GetID()),
		Name:         fmt.Sprintf("Job Run %s, ID: %d", job.GetName(), job.GetID()),
		Link:         job.GetHTMLURL(),
		DateFormat:   mermaidDateFormat,
		AxisFormat:   mermaidAxisFormat,
		GoDateFormat: goDateFormat,
		Items:        tasks,
		Owner:        owner,
		Repo:         repo,
		DataType:     "job_run",
	}, nil
}

// jobRunLink returns the link to a specific job run's rendering.
// You need to add on the extension (.html, .md) to this path.
func jobRunLink(owner, repo string, jobRunID int64) string {
	return filepath.Join("/", owner, repo, jobRunOutputDir, fmt.Sprint(jobRunID))
}
