package observe

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

func Commit(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	commitSHA string,
	opts ...Option,
) (*Observation, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	commit, err := gather.Commit(log, client, owner, repo, commitSHA, options.gatherOptions...)
	if err != nil {
		return nil, err
	}

	var (
		workflowRuns = make([]*gather.WorkflowRunData, 0, len(commit.WorkflowRunIDs))
		observation  = &Observation{
			ID:         commitSHA,
			Name:       "Commit " + commitSHA,
			Owner:      owner,
			Repo:       repo,
			GitHubLink: commit.GetHTMLURL(),
			DataType:   "commit",
			State:      commit.GetConclusion(),
			Actor:      commit.GetAuthor().GetLogin(),
			Cost:       commit.GetCost(),
		}
	)

	for _, workflowRunID := range commit.WorkflowRunIDs {
		workflowRun, _, err := gather.WorkflowRun(log, client, owner, repo, workflowRunID, options.gatherOptions...)
		if err != nil {
			return nil, err
		}
		workflowRuns = append(workflowRuns, workflowRun)
	}

	observation.TimelineData = buildCommitTimelineData(commit, workflowRuns)

	return observation, nil

}

func buildCommitTimelineData(commitData *gather.CommitData, workflowRuns []*gather.WorkflowRunData) *timelineData {
	var (
		items        = make([]timelineItem, 0, len(workflowRuns))
		skippedItems = []string{}
		owner        = commitData.GetOwner()
		repo         = commitData.GetRepo()
	)

	for _, workflowRun := range workflowRuns {
		var (
			startTime = workflowRun.GetRunStartedAt().Time
			duration  = workflowRun.GetRunCompletedAt().Sub(startTime)
		)

		if workflowRun.GetConclusion() == "skipped" || duration.Seconds() == 0 {
			skippedItems = append(skippedItems, workflowRun.GetName())
			continue
		}

		newItem := timelineItem{
			Name:       workflowRun.GetName(),
			ID:         fmt.Sprint(workflowRun.GetID()),
			StartTime:  workflowRun.GetRunStartedAt().Time,
			Conclusion: conclusionToGanttStatus(workflowRun.GetConclusion()),
			Duration:   duration,
			Link:       workflowRunLink(owner, repo, workflowRun.GetID()) + ".html",
		}
		if workflowRun.GetConclusion() == "cancelled" {
			newItem.Name = fmt.Sprintf("%s (cancelled)", workflowRun.GetName())
		}
		items = append(items, newItem)
	}

	return &timelineData{
		Items:        items,
		SkippedItems: skippedItems,
	}
}
