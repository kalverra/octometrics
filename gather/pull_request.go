package gather

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// PullRequestsDataDir is the directory name for storing pull request data files.
const PullRequestsDataDir = "pull_requests"

// PullRequestData wraps a GitHub PullRequest with additional commit data.
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
	)

	log = log.With().
		Int("pull_request_number", pullRequestNumber).
		Logger()

	err := ensureDataDir(targetDir, PullRequestsDataDir)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()

	if !forceUpdate && cacheFileExists(targetFile) {
		log = log.With().
			Str("source", "local file").
			Logger()
		pullRequestData, err := readJSONFile[*PullRequestData](targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open pull request file: %w", err)
		}
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered pull request data")
		return pullRequestData, err
	}

	log = log.With().
		Str("source", "GitHub API").
		Logger()

	if client == nil {
		return nil, fmt.Errorf("github client is nil")
	}

	ctx, cancel := ghCtx()
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

	opts = append(opts, withPullRequestData(pullRequestData))
	pullRequestData.CommitData, err = prCommitData(
		log,
		client,
		owner,
		repo,
		prCommits,
		mergeQueueEvents,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to gather commit data for pull request %d: %w", pullRequestNumber, err)
	}

	if err := writeJSONFile(targetFile, pullRequestData); err != nil {
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

	ctx, cancel := ghCtx()
	defer cancel()

	for commit, err := range client.Rest.PullRequests.ListCommitsIter(ctx, owner, repo, pullRequestNumber, listOpts) {
		if err != nil {
			return nil, err
		}
		commitsMap[commit.GetSHA()] = commit
	}

	// Get all commits that are only available through Merge Queue events
	for _, event := range mergeQueueEvents {
		if event.Commit == "" {
			continue
		}

		if _, ok := commitsMap[event.Commit]; !ok {
			ctx, cancel := ghCtx()
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

	eg.SetLimit(defaultGatherConcurrency)
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
