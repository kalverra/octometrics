package observe

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/workflow-metrics/gather"
	"github.com/rs/zerolog/log"
)

func WorkflowRun(client *github.Client, owner, repo string, workflowRunID int64, outputTypes []string) error {
	workflowRun, err := gather.WorkflowRun(client, owner, repo, workflowRunID, false)
	if err != nil {
		return err
	}

	startTime := time.Now()

	err = JobRuns(client, owner, repo, workflowRunID, outputTypes)
	if err != nil {
		return fmt.Errorf("failed to observe job runs: %w", err)
	}

	workflowRunTemplateData, err := buildWorkflowRunGanttData(workflowRun)
	if err != nil {
		return fmt.Errorf("failed to generate mermaid chart: %w", err)
	}

	err = renderGantt(workflowRunTemplateData, outputTypes)
	if err != nil {
		return fmt.Errorf("failed to render gantt chart: %w", err)
	}

	log.Debug().
		Int64("workflow_run_id", workflowRunID).
		Str("duration", time.Since(startTime).String()).
		Msg("Observed workflow run")
	return nil
}

func buildWorkflowRunGanttData(workflowRun *gather.WorkflowRunData) (*ganttData, error) {
	tasks := make([]ganttItem, 0, len(workflowRun.Jobs))
	owner := workflowRun.GetRepository().GetOwner().GetLogin()
	repo := workflowRun.GetRepository().GetName()
	workflowRunID := workflowRun.GetID()
	for _, job := range workflowRun.Jobs {
		startedAt := job.GetStartedAt().Time
		duration := job.GetCompletedAt().Sub(startedAt)

		newTask := ganttItem{
			Name:       job.GetName(),
			StartTime:  job.GetStartedAt().Time,
			Conclusion: conclusionToGanntStatus(job.GetConclusion()),
			Duration:   duration,
			Link:       jobRunLink(owner, repo, job.GetID()) + ".html",
		}
		if job.GetConclusion() == "skipped" {
			newTask.Name = fmt.Sprintf("%s (skipped)", job.GetName())
		}
		tasks = append(tasks, newTask)
	}

	templateData := &ganttData{
		ID:       fmt.Sprint(workflowRunID),
		Name:     fmt.Sprintf("Workflow Run %s, Run ID: %d", workflowRun.GetName(), workflowRun.GetID()),
		Link:     workflowRun.GetHTMLURL(),
		Items:    tasks,
		Owner:    owner,
		Repo:     repo,
		DataType: "workflow_run",
	}

	return templateData, nil
}

func conclusionToGanntStatus(conclusion string) string {
	switch conclusion {
	case "failure":
		return "crit"
	}

	return ""
}

func workflowRunLink(owner, repo string, workflowRunID int64) string {
	return filepath.Join("/", owner, repo, gather.WorkflowRunsDataDir, fmt.Sprint(workflowRunID))
}
