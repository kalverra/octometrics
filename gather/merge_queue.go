package gather

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

const mergeQueuePageSize = 100

type mergeQueueAddedEvent struct {
	ID        string
	CreatedAt githubv4.DateTime
	Actor     string
	Enqueuer  string
}

type mergeQueueRemovedEvent struct {
	ID        string
	CreatedAt githubv4.DateTime
	Actor     string
	Reason    string
	Commit    string
}

func prMergeQueueEvents(
	client *GitHubClient,
	owner, repo string,
	pullRequestNumber int,
) ([]*MergeQueueEvent, error) {
	if pullRequestNumber > math.MaxInt32 {
		return nil, fmt.Errorf(
			"pull request number %d is too large for GitHub GraphQL API, will cause overflow",
			pullRequestNumber,
		)
	}

	var (
		addedEvents   []mergeQueueAddedEvent
		removedEvents []mergeQueueRemovedEvent
		eg            errgroup.Group
	)

	eg.Go(func() error {
		var err error
		addedEvents, err = fetchMergeQueueAddedEvents(client, owner, repo, pullRequestNumber)
		return err
	})
	eg.Go(func() error {
		var err error
		removedEvents, err = fetchMergeQueueRemovedEvents(client, owner, repo, pullRequestNumber)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	sort.Slice(addedEvents, func(i, j int) bool {
		return addedEvents[i].CreatedAt.Before(addedEvents[j].CreatedAt.Time)
	})
	sort.Slice(removedEvents, func(i, j int) bool {
		return removedEvents[i].CreatedAt.Before(removedEvents[j].CreatedAt.Time)
	})

	mergeEvents := make([]*MergeQueueEvent, 0, len(addedEvents))
	for index, added := range addedEvents {
		mergeEvent := &MergeQueueEvent{
			AddedTime:     added.CreatedAt.Time,
			AddedActor:    added.Actor,
			AddedEnqueuer: added.Enqueuer,
			AddedID:       added.ID,
		}

		if index >= len(removedEvents) {
			mergeEvents = append(mergeEvents, mergeEvent)
			continue
		}

		removed := removedEvents[index]
		if added.CreatedAt.After(removed.CreatedAt.Time) {
			return nil, fmt.Errorf(
				"'added' merge queue event %s at %s is after the corresponding 'removed' merge queue event %s at %s for pull request %d",
				added.ID,
				added.CreatedAt.Time,
				removed.ID,
				removed.CreatedAt.Time,
				pullRequestNumber,
			)
		}

		mergeEvent.RemovedTime = removed.CreatedAt.Time
		mergeEvent.RemovedActor = removed.Actor
		mergeEvent.RemovedID = removed.ID
		mergeEvent.RemovedReason = removed.Reason
		mergeEvent.Commit = removed.Commit
		mergeEvents = append(mergeEvents, mergeEvent)
	}

	return mergeEvents, nil
}

func fetchMergeQueueAddedEvents(
	client *GitHubClient,
	owner, repo string,
	pullRequestNumber int,
) ([]mergeQueueAddedEvent, error) {
	var (
		all   []mergeQueueAddedEvent
		after *githubv4.String
	)

	for {
		var query struct {
			Repository struct {
				PullRequest struct {
					TimelineItems struct {
						PageInfo struct {
							HasNextPage bool
							EndCursor   githubv4.String
						}
						Nodes []struct {
							AddedToMergeQueueEvent struct {
								Actor struct {
									Login githubv4.String
								}
								CreatedAt githubv4.DateTime
								Enqueuer  struct {
									Login githubv4.String
								}
								ID githubv4.String
							} `graphql:"... on AddedToMergeQueueEvent"`
						}
					} `graphql:"timelineItems(itemTypes: [ADDED_TO_MERGE_QUEUE_EVENT], first: $first, after: $after)"`
				} `graphql:"pullRequest(number: $prNumber)"`
			} `graphql:"repository(owner: $owner, name: $repo)"`
		}

		prNumber := githubv4.Int(pullRequestNumber) //nolint:gosec // bounded by MaxInt32 check in prMergeQueueEvents
		variables := map[string]any{
			"owner":    githubv4.String(owner),
			"repo":     githubv4.String(repo),
			"prNumber": prNumber,
			"first":    githubv4.Int(mergeQueuePageSize),
			"after":    after,
		}

		if err := client.GraphQL.Query(context.Background(), &query, variables); err != nil {
			return nil, fmt.Errorf("failed to query for added to merge queue events: %w", err)
		}

		for _, node := range query.Repository.PullRequest.TimelineItems.Nodes {
			event := node.AddedToMergeQueueEvent
			all = append(all, mergeQueueAddedEvent{
				ID:        string(event.ID),
				CreatedAt: event.CreatedAt,
				Actor:     string(event.Actor.Login),
				Enqueuer:  string(event.Enqueuer.Login),
			})
		}

		if !query.Repository.PullRequest.TimelineItems.PageInfo.HasNextPage {
			break
		}
		cursor := query.Repository.PullRequest.TimelineItems.PageInfo.EndCursor
		after = &cursor
	}

	return all, nil
}

func fetchMergeQueueRemovedEvents(
	client *GitHubClient,
	owner, repo string,
	pullRequestNumber int,
) ([]mergeQueueRemovedEvent, error) {
	var (
		all   []mergeQueueRemovedEvent
		after *githubv4.String
	)

	for {
		var query struct {
			Repository struct {
				PullRequest struct {
					TimelineItems struct {
						PageInfo struct {
							HasNextPage bool
							EndCursor   githubv4.String
						}
						Nodes []struct {
							RemovedFromMergeQueueEvent struct {
								Actor struct {
									Login githubv4.String
								}
								BeforeCommit struct {
									OID githubv4.String
								}
								CreatedAt githubv4.DateTime
								Reason    githubv4.String
								ID        githubv4.String
							} `graphql:"... on RemovedFromMergeQueueEvent"`
						}
					} `graphql:"timelineItems(itemTypes: [REMOVED_FROM_MERGE_QUEUE_EVENT], first: $first, after: $after)"`
				} `graphql:"pullRequest(number: $prNumber)"`
			} `graphql:"repository(owner: $owner, name: $repo)"`
		}

		prNumber := githubv4.Int(pullRequestNumber) //nolint:gosec // bounded by MaxInt32 check in prMergeQueueEvents
		variables := map[string]any{
			"owner":    githubv4.String(owner),
			"repo":     githubv4.String(repo),
			"prNumber": prNumber,
			"first":    githubv4.Int(mergeQueuePageSize),
			"after":    after,
		}

		if err := client.GraphQL.Query(context.Background(), &query, variables); err != nil {
			return nil, fmt.Errorf("failed to query for removed from merge queue events: %w", err)
		}

		for _, node := range query.Repository.PullRequest.TimelineItems.Nodes {
			event := node.RemovedFromMergeQueueEvent
			all = append(all, mergeQueueRemovedEvent{
				ID:        string(event.ID),
				CreatedAt: event.CreatedAt,
				Actor:     string(event.Actor.Login),
				Reason:    string(event.Reason),
				Commit:    string(event.BeforeCommit.OID),
			})
		}

		if !query.Repository.PullRequest.TimelineItems.PageInfo.HasNextPage {
			break
		}
		cursor := query.Repository.PullRequest.TimelineItems.PageInfo.EndCursor
		after = &cursor
	}

	return all, nil
}
