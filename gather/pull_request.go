package gather

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
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

// IsInProgress returns true if the pull request is open or has in-progress commits/workflows.
func (p *PullRequestData) IsInProgress() bool {
	if p == nil || p.PullRequest == nil {
		return false
	}
	if p.GetState() == "open" {
		return true
	}
	for _, c := range p.CommitData {
		if c != nil && c.IsInProgress() {
			return true
		}
	}
	return false
}

var (
	pullRequestCache sync.Map
	pullRequestGroup inFlightGroup
)

func tryLoadPullRequestFromCache(
	log zerolog.Logger,
	client *GitHubClient,
	options *options,
	cacheKey, targetFile string,
) (*PullRequestData, bool) {
	if options.ForceUpdate || !cacheFileExists(targetFile) {
		return nil, false
	}
	if !options.SkipMemoryCache {
		if cached, ok := pullRequestCache.Load(cacheKey); ok {
			prData := cached.(*PullRequestData)
			if client == nil || !prData.IsInProgress() {
				return prData, true
			}
		}
	}
	prData, loadErr := readJSONFile[*PullRequestData](targetFile)
	if loadErr != nil {
		log.Warn().
			Err(loadErr).
			Str("target_file", targetFile).
			Msg("Corrupted local cache file encountered; re-fetching from GitHub")
		_ = os.Remove(targetFile)
		return nil, false
	}
	if client == nil || !prData.IsInProgress() {
		pullRequestCache.Store(cacheKey, prData)
		return prData, true
	}
	return nil, false
}

// PullRequest gathers the pull request data for a given pull request number.
func PullRequest(
	parentCtx context.Context,
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
	cacheKey := targetFile

	if cached, ok := tryLoadPullRequestFromCache(log, client, options, cacheKey, targetFile); ok {
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Str("source", "cache").
			Msg("Gathered pull request data")
		return cached, nil
	}

	val, err := pullRequestGroup.Do(cacheKey, func() (any, error) {
		if cached, ok := tryLoadPullRequestFromCache(log, client, options, cacheKey, targetFile); ok {
			return cached, nil
		}

		log = log.With().
			Str("source", "GitHub API").
			Logger()

		if client == nil {
			return nil, fmt.Errorf("github client is nil")
		}

		ctx, cancel := ghCtx(parentCtx)
		pr, resp, getErr := client.Rest.PullRequests.Get(ctx, owner, repo, pullRequestNumber)
		cancel()
		if getErr != nil {
			return nil, getErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}
		if pr == nil {
			return nil, fmt.Errorf("pull request '%d' not found on GitHub", pullRequestNumber)
		}

		mergeQueueEvents, mqErr := prMergeQueueEvents(parentCtx, client, owner, repo, pullRequestNumber)
		if mqErr != nil {
			return nil, fmt.Errorf(
				"failed to gather merge queue events for pull request %d: %w",
				pullRequestNumber,
				mqErr,
			)
		}

		pullRequestData.PullRequest = pr
		prCommitsList, commitsErr := prCommits(parentCtx, client, owner, repo, pullRequestNumber, mergeQueueEvents)
		if commitsErr != nil {
			return nil, fmt.Errorf("failed to gather commits for pull request %d: %w", pullRequestNumber, commitsErr)
		}

		optsWithPR := append(slices.Clone(opts), withPullRequestData(pullRequestData))
		var commitDataErr error
		pullRequestData.CommitData, commitDataErr = prCommitData(
			parentCtx,
			log,
			client,
			owner,
			repo,
			prCommitsList,
			mergeQueueEvents,
			optsWithPR...,
		)
		if commitDataErr != nil {
			return nil, fmt.Errorf(
				"failed to gather commit data for pull request %d: %w",
				pullRequestNumber,
				commitDataErr,
			)
		}

		if writeErr := writeJSONFile(targetFile, pullRequestData); writeErr != nil {
			return nil, fmt.Errorf(
				"failed to write pull request data to file for pull request %d: %w",
				pullRequestNumber,
				writeErr,
			)
		}

		_ = AppendManifestRecord(options.DataDir, owner, repo, ManifestRecord{
			Type:      "pull_request",
			ID:        fmt.Sprint(pullRequestNumber),
			Name:      pullRequestData.GetTitle(),
			State:     pullRequestData.GetState(),
			Actor:     pullRequestData.GetUser().GetLogin(),
			CreatedAt: pullRequestData.GetCreatedAt().Time,
		})

		pullRequestCache.Store(cacheKey, pullRequestData)
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered pull request data")
		return pullRequestData, nil
	})
	if err != nil {
		return nil, err
	}

	return val.(*PullRequestData), nil
}

func prCommits(
	parentCtx context.Context,
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

	ctx, cancel := ghCtx(parentCtx)
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
			ctx, cancel := ghCtx(parentCtx)
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
	parentCtx context.Context,
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
	)

	ghCtxInst, cancel := ghCtx(parentCtx)
	defer cancel()
	eg, egCtx := errgroup.WithContext(ghCtxInst)

	eg.SetLimit(defaultGatherConcurrency)
	for _, commit := range prCommits {
		eg.Go(func() error {
			commitOpts := append(slices.Clone(opts), withRepositoryCommit(commit))
			data, err := Commit(egCtx, log, client, owner, repo, commit.GetSHA(), commitOpts...)
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
