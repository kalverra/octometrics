package observe

import (
	"fmt"
	"strings"
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
	mermaidDateFormat, mermaidAxisFormat, goDateFormat := ganttDetermineDateFormat(
		workflowRun.GetRunStartedAt().Time,
		workflowRun.GetUpdatedAt().Time, // TODO: UpdatedAt is probably inaccurate
	)

	tasks := make([]ganttItem, 0, len(workflowRun.Jobs))
	owner := workflowRun.GetRepository().GetOwner().GetLogin()
	repo := workflowRun.GetRepository().GetName()
	workflowRunID := workflowRun.GetID()
	for _, job := range workflowRun.Jobs {
		if job.GetConclusion() == "skipped" {
			continue
		}

		startedAt := job.GetStartedAt().Time
		duration := job.GetCompletedAt().Sub(startedAt)

		jobName := job.GetName()
		saniName := strings.ReplaceAll(jobName, " ", "_")
		// Colons in names break mermaid rendering https://github.com/mermaid-js/mermaid/issues/742
		jobName = strings.ReplaceAll(jobName, ":", "#colon;")
		tasks = append(tasks, ganttItem{
			Name:       jobName,
			MermaidID:  saniName,
			StartTime:  job.GetStartedAt().Time,
			Conclusion: conclusionToGanntStatus(job.GetConclusion()),
			Duration:   duration,
			Link:       jobRunLink(owner, repo, job.GetID()) + ".html",
		})
	}

	templateData := &ganttData{
		ID:           fmt.Sprint(workflowRunID),
		Name:         fmt.Sprintf("Workflow Run %s, Run ID: %d", workflowRun.GetName(), workflowRun.GetID()),
		Link:         workflowRun.GetHTMLURL(),
		DateFormat:   mermaidDateFormat,
		AxisFormat:   mermaidAxisFormat,
		GoDateFormat: goDateFormat,
		Items:        tasks,
		Owner:        owner,
		Repo:         repo,
		DataType:     "workflow_run",
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
