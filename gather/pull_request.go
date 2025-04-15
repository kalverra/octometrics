package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

const PullRequestsDataDir = "pull_requests"

type PullRequestData struct {
	*github.PullRequest
	CommitData []*CommitData `json:"commit_data"`
}

// GetCommitData returns the commit data for the pull request.
func (p *PullRequestData) GetCommitData() []*CommitData {
	return p.CommitData
}

// PullRequest gathers the pull request data for a given pull request number.
func PullRequest(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	pullRequestNumber int,
	opts ...Option,
) (*PullRequestData, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	var (
		forceUpdate = options.ForceUpdate

		pullRequestData = &PullRequestData{}
		targetDir       = filepath.Join(options.DataDir, owner, repo, PullRequestsDataDir)
		targetFile      = filepath.Join(targetDir, fmt.Sprintf("%d.json", pullRequestNumber))
		fileExists      = false
	)

	err := os.MkdirAll(targetDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to make data dir '%s': %w", WorkflowRunsDataDir, err)
	}

	if _, err := os.Stat(targetFile); err == nil {
		fileExists = true
	}

	log = log.With().
		Int("pull_request_number", pullRequestNumber).
		Logger()

	startTime := time.Now()

	if !forceUpdate && fileExists {
		log = log.With().
			Str("source", "local file").
			Logger()
		prFileBytes, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open workflow run file: %w", err)
		}
		err = json.Unmarshal(prFileBytes, &pullRequestData)
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered pull request data")
		return pullRequestData, err
	}

	log = log.With().
		Str("source", "GitHub API").
		Logger()

	if client == nil {
		return nil, fmt.Errorf("GitHub client is nil")
	}

	ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
	pr, resp, err := client.Rest.PullRequests.Get(ctx, owner, repo, pullRequestNumber)
	cancel()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	if pr == nil {
		return nil, fmt.Errorf("pull request '%d' not found on GitHub", pullRequestNumber)
	}

	mergeQueueEvents, err := prMergeQueueEvents(client, owner, repo, pullRequestNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to gather merge queue events for pull request %d: %w", pullRequestNumber, err)
	}

	pullRequestData.PullRequest = pr
	// Get the commits associated with the pull request
	prCommits, err := prCommits(client, owner, repo, pullRequestNumber, mergeQueueEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to gather commits for pull request %d: %w", pullRequestNumber, err)
	}

	pullRequestData.CommitData, err = prCommitData(log, client, owner, repo, prCommits, mergeQueueEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to gather commit data for pull request %d: %w", pullRequestNumber, err)
	}

	data, err := json.Marshal(pullRequestData)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to marshal pull request data to json for pull request %d: %w",
			pullRequestNumber,
			err,
		)
	}
	err = os.WriteFile(targetFile, data, 0600)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to write pull request data to file for pull request %d: %w",
			pullRequestNumber,
			err,
		)
	}
	log.Debug().
		Str("duration", time.Since(startTime).String()).
		Msg("Gathered pull request data")
	return pullRequestData, nil
}

func prCommits(
	client *GitHubClient,
	owner, repo string,
	pullRequestNumber int,
	mergeQueueEvents []*MergeQueueEvent,
) ([]*github.RepositoryCommit, error) {
	// Collect all the commits we can get through REST for a pull request
	var (
		commitsMap = make(map[string]*github.RepositoryCommit)
		listOpts   = &github.ListOptions{
			PerPage: 100,
		}
	)

	for {
		ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
		commitsPage, resp, err := client.Rest.PullRequests.ListCommits(ctx, owner, repo, pullRequestNumber, listOpts)
		cancel()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf(
				"unexpected status code getting pull request commits %d: %d",
				pullRequestNumber,
				resp.StatusCode,
			)
		}

		for _, commit := range commitsPage {
			commitsMap[commit.GetSHA()] = commit
		}

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	// Get all commits that are only available through Merge Queue events
	for _, event := range mergeQueueEvents {
		if event.Commit == "" {
			continue
		}

		if _, ok := commitsMap[event.Commit]; !ok {
			ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
			commit, resp, err := client.Rest.Repositories.GetCommit(
				ctx,
				owner,
				repo,
				event.Commit,
				&github.ListOptions{
					PerPage: 1,
				},
			)
			cancel()
			if err != nil {
				return nil, fmt.Errorf("failed to search for merge queue commit %s: %w", event.Commit, err)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf(
					"unexpected status code getting merge queue commit %s: %d",
					event.Commit,
					resp.StatusCode,
				)
			}

			commitsMap[event.Commit] = commit
		}
	}

	commits := make([]*github.RepositoryCommit, 0, len(commitsMap))
	for _, commit := range commitsMap {
		commits = append(commits, commit)
	}

	return commits, nil
}

func prCommitData(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	prCommits []*github.RepositoryCommit,
	mergeQueueEvents []*MergeQueueEvent,
	opts ...Option,
) ([]*CommitData, error) {
	var (
		commitData     []*CommitData
		commitDataChan = make(chan *CommitData, len(prCommits))
		eg             errgroup.Group
	)

	for _, commit := range prCommits {
		eg.Go(func() error {
			data, err := Commit(log, client, owner, repo, commit.GetSHA(), opts...)
			if err != nil {
				return fmt.Errorf("failed to gather data for commit '%s': %w", commit.GetSHA(), err)
			}
			for _, mergeQueueEvent := range mergeQueueEvents {
				if mergeQueueEvent.Commit == commit.GetSHA() {
					data.MergeQueueEvents = append(data.MergeQueueEvents, mergeQueueEvent)
				}
			}
			commitDataChan <- data
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	close(commitDataChan)

	for data := range commitDataChan {
		commitData = append(commitData, data)
	}

	// Sort the commit data by commit date
	sort.Slice(commitData, func(i, j int) bool {
		return commitData[i].GetCommit().GetAuthor().GetDate().Before(
			commitData[j].GetCommit().GetAuthor().GetDate().Time)
	})

	return commitData, nil
}

// prMergeQueueEvents queries the GitHub GraphQL API for merge queue events for a given pull request.
func prMergeQueueEvents(
	client *GitHubClient,
	owner, repo string,
	pullRequestNumber int,
) ([]*MergeQueueEvent, error) {
	// https://docs.github.com/en/graphql/reference/objects#addedtomergequeueevent
	var addedQuery struct {
		Repository struct {
			PullRequest struct {
				TimelineItems struct {
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
				} `graphql:"timelineItems(itemTypes: [ADDED_TO_MERGE_QUEUE_EVENT], first: 100)"`
			} `graphql:"pullRequest(number: $prNumber)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	// check for pr number overflow
	if pullRequestNumber > math.MaxInt32 {
		return nil, fmt.Errorf(
			"pull request number %d is too large for GitHub GraphQL API, will cause overflow",
			pullRequestNumber,
		)
	}

	//nolint:gosec // explicitly checking MaxInt32 bound before
	variables := map[string]any{
		"owner":    githubv4.String(owner),
		"repo":     githubv4.String(repo),
		"prNumber": githubv4.Int(pullRequestNumber),
	}

	err := client.GraphQL.Query(context.Background(), &addedQuery, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query for added to merge queue events: %w", err)
	}

	// https://docs.github.com/en/graphql/reference/objects#removedfrommergequeueevent
	var removedQuery struct {
		Repository struct {
			PullRequest struct {
				TimelineItems struct {
					Nodes []struct {
						RemovedFromMergeQueueEvent struct {
							Actor struct {
								Login githubv4.String
							}
							BeforeCommit struct {
								CommitURL githubv4.String
								OID       githubv4.String
							}
							CreatedAt githubv4.DateTime
							Reason    githubv4.String
							ID        githubv4.String
						} `graphql:"... on RemovedFromMergeQueueEvent"`
					}
				} `graphql:"timelineItems(itemTypes: [REMOVED_FROM_MERGE_QUEUE_EVENT], first: 100)"`
			} `graphql:"pullRequest(number: $prNumber)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	err = client.GraphQL.Query(context.Background(), &removedQuery, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query for removed from merge queue events: %w", err)
	}

	// Sort the added and removed events by timestamp to ensure we can match them correctly
	sort.Slice(addedQuery.Repository.PullRequest.TimelineItems.Nodes, func(i, j int) bool {
		return addedQuery.Repository.PullRequest.TimelineItems.Nodes[i].AddedToMergeQueueEvent.CreatedAt.Before(
			addedQuery.Repository.PullRequest.TimelineItems.Nodes[j].AddedToMergeQueueEvent.CreatedAt.Time,
		)
	})

	sort.Slice(removedQuery.Repository.PullRequest.TimelineItems.Nodes, func(i, j int) bool {
		return removedQuery.Repository.PullRequest.TimelineItems.Nodes[i].RemovedFromMergeQueueEvent.CreatedAt.Before(
			removedQuery.Repository.PullRequest.TimelineItems.Nodes[j].RemovedFromMergeQueueEvent.CreatedAt.Time,
		)
	})

	// Merge corresponding added and removed events into a single event based on timestamps (added events don't have an associated commit)
	mergeEvents := make([]*MergeQueueEvent, 0, len(addedQuery.Repository.PullRequest.TimelineItems.Nodes))
	for index := range addedQuery.Repository.PullRequest.TimelineItems.Nodes {
		mergeEvent := &MergeQueueEvent{
			AddedTime: addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.CreatedAt.Time,
			AddedActor: string(
				addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.Actor.Login,
			),
			AddedEnqueuer: string(
				addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.Enqueuer.Login,
			),
			AddedID: string(
				addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.ID,
			),
		}

		if index >= len(removedQuery.Repository.PullRequest.TimelineItems.Nodes) {
			mergeEvents = append(mergeEvents, mergeEvent)
			break // No corresponding removed event
		}
		if addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.CreatedAt.After(
			removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.CreatedAt.Time,
		) {
			return nil, fmt.Errorf(
				"'added' merge queue event %s at %s is after the corresponding 'removed' merge queue event %s at %s for pull request %d",
				addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.ID,
				addedQuery.Repository.PullRequest.TimelineItems.Nodes[index].AddedToMergeQueueEvent.CreatedAt.Time,
				removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.ID,
				removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.CreatedAt.Time,
				pullRequestNumber,
			)
		}

		mergeEvent.RemovedTime = removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.CreatedAt.Time
		mergeEvent.RemovedActor = string(
			removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.Actor.Login,
		)
		mergeEvent.RemovedID = string(
			removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.ID,
		)
		mergeEvent.RemovedReason = string(
			removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.Reason,
		)
		mergeEvent.Commit = string(
			removedQuery.Repository.PullRequest.TimelineItems.Nodes[index].RemovedFromMergeQueueEvent.BeforeCommit.OID,
		)

		mergeEvents = append(mergeEvents, mergeEvent)
	}

	return mergeEvents, nil
}

// establishPRChecksStatus determines the status of the pull request checks
// based on the status of the individual workflow run conclusions.
// https://docs.github.com/en/rest/actions/workflow-runs?apiVersion=2022-11-28#list-workflow-runs-for-a-repository
func establishPRChecksConclusion(baseStatus, newStatus string) string {
	switch newStatus {
	case "failure", "in_progress", "timed_out":
		return newStatus
	}
	return newStatus
}
