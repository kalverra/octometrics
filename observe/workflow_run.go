package observe

import (
	"fmt"
	"path/filepath"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// WorkflowRun gathers and processes a workflow run into an observation to be rendered.
func WorkflowRun(
	log zerolog.Logger,
	client *github.Client,
	owner, repo string,
	workflowRunID int64,
	opts ...Option,
) (*Observation, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	workflowRun, _, err := gather.WorkflowRun(log, client, owner, repo, workflowRunID, options.gatherOptions...)
	if err != nil {
		return nil, err
	}

	var (
		observationData = &Observation{
			ID:         fmt.Sprint(workflowRunID),
			Name:       workflowRun.GetName(),
			GitHubLink: workflowRun.GetHTMLURL(),
			Owner:      owner,
			Repo:       repo,
			State:      workflowRun.GetConclusion(),
			Actor:      workflowRun.GetActor().GetLogin(),
			DataType:   "workflow_run",
		}
	)

	workflowRunTimelineData, err := buildWorkflowRunTimelineData(workflowRun)
	if err != nil {
		return nil, fmt.Errorf("failed to generate timeline: %w", err)
	}
	observationData.TimelineData = workflowRunTimelineData

	return observationData, nil
}

func buildWorkflowRunTimelineData(workflowRun *gather.WorkflowRunData) (*timelineData, error) {
	var (
		items        = make([]timelineItem, 0, len(workflowRun.Jobs))
		skippedItems = []string{}
		owner        = workflowRun.GetRepository().GetOwner().GetLogin()
		repo         = workflowRun.GetRepository().GetName()
	)

	for _, job := range workflowRun.Jobs {
		var (
			startedAt = job.GetStartedAt().Time
			duration  = job.GetCompletedAt().Sub(startedAt)
		)

		newTask := timelineItem{
			Name:       job.GetName(),
			ID:         fmt.Sprint(job.GetID()),
			StartTime:  job.GetStartedAt().Time,
			Conclusion: conclusionToGanttStatus(job.GetConclusion()),
			Duration:   duration,
			Link:       jobRunLink(owner, repo, job.GetID()) + ".html",
		}
		if job.GetConclusion() == "skipped" || duration.Seconds() == 0 {
			skippedItems = append(skippedItems, job.GetName())
			continue
		}
		if job.GetConclusion() == "cancelled" {
			newTask.Name = fmt.Sprintf("%s (cancelled)", job.GetName())
		}
		if job.GetRunAttempt() > 1 {
			newTask.Name = fmt.Sprintf("%s (attempt %d)", job.GetName(), job.GetRunAttempt())
		}
		items = append(items, newTask)
	}

	templateData := &timelineData{
		Items:        items,
		SkippedItems: skippedItems,
	}

	return templateData, nil
}

// https://mermaid.js.org/syntax/gantt.html#syntax
func conclusionToGanttStatus(conclusion string) string {
	status := ""
	switch conclusion {
	case "failure":
		status = "crit"
	case "cancelled":
		status = "done"
	}
	return status
}

// jobRunLink returns the link to a specific job run's rendering.
// You need to add on the extension (.html, .md) to this path.
func jobRunLink(owner, repo string, jobRunID int64) string {
	return filepath.Join("/", owner, repo, jobRunOutputDir, fmt.Sprint(jobRunID))
}

func workflowRunLink(owner, repo string, workflowRunID int64) string {
	return filepath.Join("/", owner, repo, gather.WorkflowRunsDataDir, fmt.Sprint(workflowRunID))
}
