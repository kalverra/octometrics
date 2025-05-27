package observe

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// Commit generates an observation for workflow runs triggered by a commit.
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

func buildCommitTimelineData(commitData *gather.CommitData, workflowRuns []*gather.WorkflowRunData) []*timelineData {
	owner := commitData.GetOwner()
	repo := commitData.GetRepo()

	// Group workflow runs by event
	eventGroups := make(map[string][]*gather.WorkflowRunData)
	for _, workflowRun := range workflowRuns {
		event := workflowRun.GetEvent()
		eventGroups[event] = append(eventGroups[event], workflowRun)
	}

	// Build a timelineData for each event group
	var groupedTimelines []*timelineData
	for event, runs := range eventGroups {
		var (
			items             = make([]timelineItem, 0, len(runs))
			skippedItems      = []string{}
			postTimelineItems = []postTimelineItem{}
		)

		for _, workflowRun := range runs {
			startTime := workflowRun.GetRunStartedAt().Time
			duration := workflowRun.GetRunCompletedAt().Sub(startTime)

			if workflowRun.GetConclusion() == "skipped" || duration.Seconds() == 0 {
				skippedItems = append(skippedItems, workflowRun.GetName())
				continue
			}

			// If there's a PR, catch any post-PR workflows that might have run, like on: [pull_request] activity: closed
			if workflowRun.GetEvent() == "pull_request" && !workflowRun.CorrespondingPRCloseTime.IsZero() {
				if startTime.After(workflowRun.CorrespondingPRCloseTime) {
					postTimelineItems = append(postTimelineItems, postTimelineItem{
						Name: workflowRun.GetName(),
						Link: workflowRun.GetHTMLURL(),
						Time: startTime,
					})
					continue
				}
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
		groupedTimelines = append(groupedTimelines, &timelineData{
			Event:             event,
			Items:             items,
			SkippedItems:      skippedItems,
			PostTimelineItems: postTimelineItems,
		})
	}

	return groupedTimelines
}
