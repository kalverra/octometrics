package observe

import (
	"fmt"
	"path"
	"time"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// WorkflowRun gathers and processes a workflow run into an observation to be rendered.
func WorkflowRun(
	log zerolog.Logger,
	client *gather.GitHubClient,
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

	state := workflowRun.GetConclusion()
	if state == "" {
		state = workflowRun.GetStatus()
	}

	var (
		observationData = &Observation{
			ID:         fmt.Sprint(workflowRunID),
			Name:       workflowRun.GetName(),
			GitHubLink: workflowRun.GetHTMLURL(),
			Owner:      owner,
			Repo:       repo,
			State:      state,
			Actor:      workflowRun.GetActor().GetLogin(),
			DataType:   "workflow_run",
		}
	)

	workflowRunTimelineData, err := buildWorkflowRunTimelineData(workflowRun)
	if err != nil {
		return nil, fmt.Errorf("failed to generate timeline: %w", err)
	}
	observationData.TimelineData = []*Timeline{workflowRunTimelineData}

	return observationData, nil
}

func buildWorkflowRunTimelineData(workflowRun *gather.WorkflowRunData) (*Timeline, error) {
	var (
		items             = make([]TimelineItem, 0, len(workflowRun.Jobs))
		postTimelineItems = []PostTimelineItem{}
		skippedItems      = []string{}
		queuedItems       = []string{}

		owner = workflowRun.GetRepository().GetOwner().GetLogin()
		repo  = workflowRun.GetRepository().GetName()
	)

	for _, job := range workflowRun.Jobs {
		startedAt := job.GetStartedAt().Time

		if job.GetStatus() == "queued" && startedAt.IsZero() {
			queuedItems = append(queuedItems, job.GetName())
			continue
		}

		inProgress := job.GetConclusion() == "" && job.GetStatus() == "in_progress"

		var duration time.Duration
		if inProgress {
			duration = time.Since(startedAt)
		} else {
			duration = job.GetCompletedAt().Sub(startedAt)
		}

		if job.GetConclusion() == "skipped" || (!inProgress && duration.Seconds() == 0) {
			skippedItems = append(skippedItems, job.GetName())
			continue
		}

		if workflowRun.GetEvent() == "pull_request" && !workflowRun.CorrespondingPRCloseTime.IsZero() {
			if startedAt.After(workflowRun.CorrespondingPRCloseTime) {
				postTimelineItems = append(postTimelineItems, PostTimelineItem{
					Name: job.GetName(),
					Link: job.GetHTMLURL(),
					Time: startedAt,
				})
				continue
			}
		}

		conclusion := job.GetConclusion()
		if inProgress {
			conclusion = "in_progress"
		}

		newTask := TimelineItem{
			Name:       job.GetName(),
			ID:         fmt.Sprint(job.GetID()),
			StartTime:  startedAt,
			Conclusion: conclusionToGanttStatus(conclusion),
			Duration:   duration,
			Link:       jobRunLink(owner, repo, job.GetID()) + ".html",
		}
		if inProgress {
			newTask.Name = fmt.Sprintf("%s (in progress)", job.GetName())
		} else if job.GetConclusion() == "cancelled" {
			newTask.Name = fmt.Sprintf("%s (cancelled)", job.GetName())
		}
		if job.GetRunAttempt() > 1 {
			newTask.Name = fmt.Sprintf("%s (attempt %d)", job.GetName(), job.GetRunAttempt())
		}
		items = append(items, newTask)
	}

	templateData := &Timeline{
		Event:             workflowRun.GetEvent(),
		Items:             items,
		SkippedItems:      skippedItems,
		QueuedItems:       queuedItems,
		PostTimelineItems: postTimelineItems,
	}

	if err := templateData.normalize(); err != nil {
		return nil, fmt.Errorf("failed to normalize timeline: %w", err)
	}

	return templateData, nil
}

// https://mermaid.js.org/syntax/gantt.html#syntax
func conclusionToGanttStatus(conclusion string) string {
	switch conclusion {
	case "failure":
		return "crit"
	case "cancelled":
		return "done"
	case "in_progress":
		return "active"
	}
	return ""
}

// jobRunLink returns the link to a specific job run's rendering.
// You need to add on the extension (.html, .md) to this path.
func jobRunLink(owner, repo string, jobRunID int64) string {
	return path.Join("/", owner, repo, jobRunOutputDir, fmt.Sprint(jobRunID))
}

func workflowRunLink(owner, repo string, workflowRunID int64) string {
	return path.Join("/", owner, repo, gather.WorkflowRunsDataDir, fmt.Sprint(workflowRunID))
}
