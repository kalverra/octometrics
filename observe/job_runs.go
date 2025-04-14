package observe

import (
	"fmt"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/kalverra/octometrics/gather"
)

const jobRunOutputDir = "job_runs"

// JobRuns observes all job runs for a given workflow run.
func JobRuns(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	workflowRunID int64,
	opts ...Option,
) ([]*Observation, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	workflowRun, _, err := gather.WorkflowRun(log, client, owner, repo, workflowRunID, options.gatherOptions...)
	if err != nil {
		return nil, err
	}

	var (
		eg               errgroup.Group
		observations     = make([]*Observation, 0, len(workflowRun.Jobs))
		observationsChan = make(chan *Observation, len(workflowRun.Jobs))
	)

	for _, job := range workflowRun.Jobs {
		eg.Go(func() error {
			jobRunTemplateData, err := buildJobRunTimelineData(job)
			if err != nil {
				return fmt.Errorf("failed to build timeline for job '%d': %w", job.GetID(), err)
			}

			observationsChan <- &Observation{
				ID:           fmt.Sprint(job.GetID()),
				Name:         job.GetName(),
				GitHubLink:   job.GetHTMLURL(),
				TimelineData: jobRunTemplateData,
				Owner:        owner,
				Repo:         repo,
				DataType:     "job_run",
				State:        job.GetConclusion(),
				Actor:        workflowRun.GetActor().GetLogin(),
				Cost:         job.GetCost(),
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("failed to observe job runs: %w", err)
	}

	close(observationsChan)
	for observation := range observationsChan {
		observations = append(observations, observation)
	}

	return observations, nil
}

func buildJobRunTimelineData(job *gather.JobData) (*timelineData, error) {
	var (
		items        = make([]timelineItem, 0, len(job.Steps))
		skippedItems = []string{}
	)

	for _, step := range job.Steps {
		var (
			startTime = step.GetStartedAt().Time
			duration  = step.GetCompletedAt().Sub(startTime)
		)

		if step.GetConclusion() == "skipped" || duration.Seconds() == 0 {
			skippedItems = append(skippedItems, step.GetName())
			continue
		}

		newItem := timelineItem{
			Name:       step.GetName(),
			ID:         step.GetName(),
			StartTime:  step.GetStartedAt().Time,
			Conclusion: conclusionToGanttStatus(step.GetConclusion()),
			Duration:   duration,
		}
		if step.GetConclusion() == "cancelled" {
			newItem.Name = fmt.Sprintf("%s (cancelled)", step.GetName())
		}
		items = append(items, newItem)
	}

	return &timelineData{
		Items:        items,
		SkippedItems: skippedItems,
	}, nil
}
