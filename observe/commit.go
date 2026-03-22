// Package observe renders gathered GitHub Actions data into visual observation formats.
package observe

import (
	"fmt"
	"time"

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

	observation.TimelineData = buildCommitTimelineData(log, commit, workflowRuns)

	return observation, nil

}

func buildCommitTimelineData(
	log zerolog.Logger,
	commitData *gather.CommitData,
	workflowRuns []*gather.WorkflowRunData,
) []*Timeline {
	owner := commitData.GetOwner()
	repo := commitData.GetRepo()

	// Group workflow runs by event
	eventGroups := make(map[string][]*gather.WorkflowRunData)
	for _, workflowRun := range workflowRuns {
		event := workflowRun.GetEvent()
		eventGroups[event] = append(eventGroups[event], workflowRun)
	}

	// Build a Timeline for each event group
	var groupedTimelines []*Timeline
	for event, runs := range eventGroups {
		var (
			items             = make([]TimelineItem, 0, len(runs))
			skippedItems      = []string{}
			queuedItems       = []string{}
			postTimelineItems = []PostTimelineItem{}
		)

		for _, workflowRun := range runs {
			startTime := workflowRun.GetRunStartedAt().Time

			if workflowRun.GetStatus() == "queued" && startTime.IsZero() {
				queuedItems = append(queuedItems, workflowRun.GetName())
				continue
			}

			inProgress := workflowRun.GetConclusion() == "" && workflowRun.GetStatus() == "in_progress"

			var duration time.Duration
			completedAt := workflowRun.GetRunCompletedAt()
			if inProgress || completedAt.IsZero() {
				duration = time.Since(startTime)
			} else {
				duration = completedAt.Sub(startTime)
			}

			if workflowRun.GetConclusion() == "skipped" || (!inProgress && duration.Seconds() == 0) {
				skippedItems = append(skippedItems, workflowRun.GetName())
				continue
			}

			if workflowRun.GetEvent() == "pull_request" && !workflowRun.CorrespondingPRCloseTime.IsZero() {
				if startTime.After(workflowRun.CorrespondingPRCloseTime) {
					postTimelineItems = append(postTimelineItems, PostTimelineItem{
						Name: workflowRun.GetName(),
						Link: workflowRun.GetHTMLURL(),
						Time: startTime,
					})
					continue
				}
			}

			conclusion := workflowRun.GetConclusion()
			if inProgress {
				conclusion = "in_progress"
			}

			newItem := TimelineItem{
				Name:       workflowRun.GetName(),
				ID:         fmt.Sprint(workflowRun.GetID()),
				StartTime:  workflowRun.GetRunStartedAt().Time,
				Conclusion: conclusionToGanttStatus(conclusion),
				Duration:   duration,
				Link:       workflowRunLink(owner, repo, workflowRun.GetID()) + ".html",
			}
			if inProgress {
				newItem.Name = fmt.Sprintf("%s (in progress)", workflowRun.GetName())
			} else if workflowRun.GetConclusion() == "cancelled" {
				newItem.Name = fmt.Sprintf("%s (cancelled)", workflowRun.GetName())
			}
			items = append(items, newItem)
		}
		groupedTimelines = append(groupedTimelines, &Timeline{
			Event:             event,
			Items:             items,
			SkippedItems:      skippedItems,
			QueuedItems:       queuedItems,
			PostTimelineItems: postTimelineItems,
		})
	}

	for _, timeline := range groupedTimelines {
		// Log the error but continue so we don't drop the whole commit if one timeline fails
		if err := timeline.normalize(); err != nil {
			// Instead of returning error and failing the whole commit view, just skip normalizing.
			// This might lead to suboptimal rendering for this specific event type but preserves the rest.
			log.Error().Err(err).Str("event", timeline.Event).Msg("Failed to normalize timeline")
			continue
		}
	}

	return groupedTimelines
}
