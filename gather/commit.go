// Package gather provides functions for gathering GitHub Actions data
// including commits, pull requests, and workflow runs.
package gather

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// CommitsDataDir is the directory name for storing commit data files.
const CommitsDataDir = "commits"

var workflowRunIDRe = regexp.MustCompile(`\/actions\/runs\/(\d+)`)

// CommitData contains the commit data for a given commit SHA.
// It also includes some additional info that makes it easier to map to its associated workflows.
type CommitData struct {
	*github.RepositoryCommit
	Owner              string             `json:"owner"`
	Repo               string             `json:"repo"`
	CheckRuns          []*github.CheckRun `json:"-"`
	MergeQueueEvents   []*MergeQueueEvent `json:"merge_queue_events"`
	WorkflowRunIDs     []int64            `json:"workflow_run_ids"`
	WorkflowRuns       []*WorkflowRunData `json:"-"`
	StartActionsTime   time.Time          `json:"start_actions_time"`
	EndActionsTime     time.Time          `json:"end_actions_time"`
	Conclusion         string             `json:"conclusion"`
	Cost               int64              `json:"cost"`
	CostEstimate       bool               `json:"cost_estimate,omitempty"`
	CostGathered       bool               `json:"cost_gathered,omitempty"`
	CorrespondingPRNum int                `json:"corresponding_pr_number,omitempty"`
}

// GetOwner returns the owner of the repository for the commit.
func (c *CommitData) GetOwner() string {
	if c == nil {
		return ""
	}
	return c.Owner
}

// GetRepo returns the repository name for the commit.
func (c *CommitData) GetRepo() string {
	if c == nil {
		return ""
	}
	return c.Repo
}

// GetMergeQueueEvents returns any merge queue events associated with the commit.
func (c *CommitData) GetMergeQueueEvents() []*MergeQueueEvent {
	if c == nil {
		return []*MergeQueueEvent{}
	}
	return c.MergeQueueEvents
}

// GetCheckRuns returns the check runs associated with the commit.
func (c *CommitData) GetCheckRuns() []*github.CheckRun {
	if c == nil {
		return []*github.CheckRun{}
	}
	return c.CheckRuns
}

// GetWorkflowRunIDs returns the workflow run IDs associated with the commit.
func (c *CommitData) GetWorkflowRunIDs() []int64 {
	if c == nil {
		return []int64{}
	}
	return c.WorkflowRunIDs
}

// GetStartActionsTime returns the earliest start time of all actions that ran for the commit.
func (c *CommitData) GetStartActionsTime() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.StartActionsTime
}

// GetEndActionsTime returns the latest end time of all actions that ran for the commit.
func (c *CommitData) GetEndActionsTime() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.EndActionsTime
}

// GetConclusion returns the overall conclusion of all actions that ran for the commit.
func (c *CommitData) GetConclusion() string {
	if c == nil {
		return ""
	}
	return c.Conclusion
}

// GetDuration returns the total wall clock duration of actions that ran for the commit.
func (c *CommitData) GetDuration() time.Duration {
	if c == nil || c.StartActionsTime.IsZero() || c.EndActionsTime.IsZero() {
		return 0
	}
	if c.EndActionsTime.Before(c.StartActionsTime) {
		return 0
	}
	return c.EndActionsTime.Sub(c.StartActionsTime)
}

// GetCost returns the total cost of all actions that ran for the commit in tenths of a cent.
func (c *CommitData) GetCost() int64 {
	if c == nil {
		return 0
	}
	return c.Cost
}

// GetCostEstimate returns true when Cost includes estimated costs (e.g. runs-on runners).
func (c *CommitData) GetCostEstimate() bool {
	if c == nil {
		return false
	}
	return c.CostEstimate
}

// GetCostGathered returns true when cost data was gathered for this commit.
func (c *CommitData) GetCostGathered() bool {
	if c == nil {
		return false
	}
	return c.CostGathered
}

// MergeQueueEvent details a commit being added or removed from the merge queue.
type MergeQueueEvent struct {
	// Info from removed event
	Commit          string
	RemovedTime     time.Time
	RemovedActor    string
	RemovedReason   string
	RemovedEnqueuer string
	RemovedID       string

	// Info from added event
	AddedTime     time.Time
	AddedActor    string
	AddedEnqueuer string
	AddedID       string
}

// IsInProgress returns true if the commit has incomplete actions or in-progress workflow runs.
func (c *CommitData) IsInProgress() bool {
	if c == nil {
		return false
	}
	if c.Conclusion == "in_progress" || c.Conclusion == "" {
		return true
	}
	for _, wf := range c.WorkflowRuns {
		if wf != nil && wf.IsInProgress() {
			return true
		}
	}
	return false
}

var (
	commitCache sync.Map
	commitGroup inFlightGroup
)

func tryLoadCommitFromCache(
	parentCtx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	sha string,
	opts []Option,
	options *options,
	cacheKey, targetFile string,
) (*CommitData, bool) {
	if options.ForceUpdate || !cacheFileExists(targetFile) {
		return nil, false
	}
	if !options.SkipMemoryCache {
		if cached, ok := commitCache.Load(cacheKey); ok {
			cData := cached.(*CommitData)
			if client == nil || !cData.IsInProgress() {
				return cData, true
			}
		}
	}
	cData, loadErr := readJSONFile[*CommitData](targetFile)
	if loadErr != nil {
		log.Warn().
			Err(loadErr).
			Str("target_file", targetFile).
			Msg("Corrupted local cache file encountered; re-fetching from GitHub")
		_ = os.Remove(targetFile)
		return nil, false
	}

	cData.WorkflowRuns = nil
	for _, runID := range cData.WorkflowRunIDs {
		wf, _, loadWfErr := WorkflowRun(parentCtx, log, client, owner, repo, runID, opts...)
		if loadWfErr == nil && wf != nil {
			cData.WorkflowRuns = append(cData.WorkflowRuns, wf)
		}
	}
	if client == nil || !cData.IsInProgress() {
		commitCache.Store(cacheKey, cData)
		return cData, true
	}
	log.Debug().
		Str("commit_sha", sha).
		Msg("Cached commit was in progress; refreshing from GitHub API")
	return nil, false
}

// Commit gathers commit data for a given commit SHA and enhances matches it with workflows that ran on that commit.
func Commit(
	parentCtx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo,
	sha string,
	opts ...Option,
) (*CommitData, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	var (
		commitData = &CommitData{
			Owner: owner,
			Repo:  repo,
		}
		targetDir  = filepath.Join(options.DataDir, owner, repo, CommitsDataDir)
		targetFile = filepath.Join(targetDir, fmt.Sprintf("%s.json", sha))
	)

	if options.pullRequestData != nil {
		commitData.CorrespondingPRNum = options.pullRequestData.GetNumber()
	}

	log = log.With().
		Str("target_file", targetFile).
		Str("commit_sha", sha).
		Logger()

	err := ensureDataDir(targetDir, CommitsDataDir)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	cacheKey := fmt.Sprintf("%s:pr=%d", targetFile, commitData.CorrespondingPRNum)

	if cached, ok := tryLoadCommitFromCache(
		parentCtx,
		log,
		client,
		owner,
		repo,
		sha,
		opts,
		options,
		cacheKey,
		targetFile,
	); ok {
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Str("source", "cache").
			Msg("Gathered commit data")
		return cached, nil
	}

	val, err := commitGroup.Do(cacheKey, func() (any, error) {
		if cached, ok := tryLoadCommitFromCache(
			parentCtx,
			log,
			client,
			owner,
			repo,
			sha,
			opts,
			options,
			cacheKey,
			targetFile,
		); ok {
			return cached, nil
		}

		log = log.With().
			Str("source", "GitHub API").
			Logger()

		var commit *github.RepositoryCommit
		if options.repositoryCommit != nil {
			commit = options.repositoryCommit
		} else {
			if client == nil {
				return nil, fmt.Errorf("github client is nil")
			}
			ctx, cancel := ghCtx(parentCtx)
			var resp *github.Response
			var fetchErr error
			commit, resp, fetchErr = client.Rest.Repositories.GetCommit(ctx, owner, repo, sha, nil)
			cancel()
			if fetchErr != nil {
				return nil, fetchErr
			}
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
			}
		}

		commitData.RepositoryCommit = commit
		checkRuns, checkErr := checkRunsForCommit(parentCtx, client, owner, repo, sha)
		if checkErr != nil {
			return nil, checkErr
		}

		if len(checkRuns) == 0 && len(commit.Parents) > 1 {
			log.Info().
				Str("commit_sha", sha).
				Msg("No check runs found on merge commit. Checking parents.")
			// Check the second parent first (typical for PR/merge group merge commits)
			for _, v := range slices.Backward(commit.Parents) {
				parentSHA := v.GetSHA()
				if parentSHA == "" {
					continue
				}
				log.Info().
					Str("parent_sha", parentSHA).
					Msg("Checking parent commit for check runs")
				parentCheckRuns, pErr := checkRunsForCommit(parentCtx, client, owner, repo, parentSHA)
				if pErr != nil {
					log.Warn().
						Str("parent_sha", parentSHA).
						Err(pErr).
						Msg("Failed to check runs for parent commit")
					continue
				}
				if len(parentCheckRuns) > 0 {
					log.Info().
						Str("parent_sha", parentSHA).
						Int("count", len(parentCheckRuns)).
						Msg("Found check runs on parent commit")
					checkRuns = parentCheckRuns
					break
				}
			}
		}
		commitData.CheckRuns = checkRuns
		if wfErr := setWorkflowRunsForCommit(
			parentCtx,
			log,
			client,
			owner,
			repo,
			commitData.CheckRuns,
			commitData,
			opts,
		); wfErr != nil {
			return nil, fmt.Errorf("failed to gather workflow runs for commit '%s': %w", sha, wfErr)
		}

		if writeErr := writeJSONFile(targetFile, commitData); writeErr != nil {
			return nil, fmt.Errorf("failed to write commit data to file '%s': %w", sha, writeErr)
		}

		_ = AppendManifestRecord(options.DataDir, owner, repo, ManifestRecord{
			Type:      "commit",
			ID:        sha,
			Name:      "Commit " + sha,
			State:     commitData.Conclusion,
			Actor:     commitData.GetCommit().GetAuthor().GetName(),
			CreatedAt: commitData.GetCommit().GetAuthor().GetDate().Time,
		})

		commitCache.Store(cacheKey, commitData)
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered commit data")
		return commitData, nil
	})
	if err != nil {
		return nil, err
	}

	return val.(*CommitData), nil
}

func checkRunsForCommit(
	parentCtx context.Context,
	client *GitHubClient,
	owner, repo string,
	sha string,
) ([]*github.CheckRun, error) {
	if client == nil {
		return nil, nil
	}
	var (
		allCheckRuns []*github.CheckRun
		listOpts     = &github.ListCheckRunsOptions{
			Filter: new("all"),
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}
	)

	ctx, cancel := ghCtx(parentCtx)
	defer cancel()

	for checkRun, err := range client.Rest.Checks.ListCheckRunsForRefIter(ctx, owner, repo, sha, listOpts) {
		if err != nil {
			return nil, fmt.Errorf("failed to gather check runs from GitHub for commit '%s': %w", sha, err)
		}
		allCheckRuns = append(allCheckRuns, checkRun)
	}

	return allCheckRuns, nil
}

// setWorkflowRunsForCommit gathers all the workflow runs for a commit
// and sets the workflow run IDs in the commit data.
func setWorkflowRunsForCommit(
	parentCtx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	checkRuns []*github.CheckRun,
	commitData *CommitData,
	opts []Option,
) error {
	var (
		workflowRunIDsSet = map[int64]struct{}{}
	)

	ghCtxInst, cancel := ghCtx(parentCtx)
	defer cancel()
	eg, egCtx := errgroup.WithContext(ghCtxInst)

	for _, checkRun := range checkRuns {
		if checkRun.GetStatus() != "completed" {
			log.Warn().Str("check_run", checkRun.GetName()).Msg("Check run is not yet completed")
		}
		match := workflowRunIDRe.FindStringSubmatch(checkRun.GetHTMLURL())
		if len(match) == 0 {
			log.Warn().
				Str("owner", owner).
				Str("repo", repo).
				Str("SHA", commitData.GetSHA()).
				Str("check_run", checkRun.GetName()).
				Str("URL", checkRun.GetHTMLURL()).
				Msg("Failed to parse workflow run ID from check run URL")
			continue
		}
		workflowRunID, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse workflow run ID from check run URL: %w", err)
		}
		workflowRunIDsSet[workflowRunID] = struct{}{}
	}

	type workflowRunSummary struct {
		id           int64
		conclusion   string
		cost         int64
		costEstimate bool
		costGathered bool
		start        time.Time
		end          time.Time
	}

	workflowRunIDs := make([]int64, 0, len(workflowRunIDsSet))
	for workflowRunID := range workflowRunIDsSet {
		workflowRunIDs = append(workflowRunIDs, workflowRunID)
	}
	slices.Sort(workflowRunIDs)

	summaries := make([]workflowRunSummary, len(workflowRunIDs))
	workflowRuns := make([]*WorkflowRunData, len(workflowRunIDs))
	commitOpts := append(slices.Clone(opts), withCommitData(commitData))
	eg.SetLimit(defaultGatherConcurrency)
	for index, workflowRunID := range workflowRunIDs {
		eg.Go(func(index int, workflowRunID int64) func() error {
			return func() error {
				workflowRun, _, err := WorkflowRun(egCtx, log, client, owner, repo, workflowRunID, commitOpts...)
				if err != nil {
					return fmt.Errorf("failed to gather workflow run data for commit %s: %w", commitData.GetSHA(), err)
				}
				conclusion := workflowRun.GetConclusion()
				if conclusion == "" {
					conclusion = workflowRun.GetStatus()
				}
				summaries[index] = workflowRunSummary{
					id:           workflowRunID,
					conclusion:   conclusion,
					cost:         workflowRun.GetCost(),
					costEstimate: workflowRun.GetCostEstimate(),
					costGathered: workflowRun.GetCostGathered(),
					start:        workflowRun.GetRunStartedAt().Time,
					end:          workflowRun.GetRunCompletedAt(),
				}
				workflowRuns[index] = workflowRun
				return nil
			}
		}(index, workflowRunID))
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to gather workflow runs for commit %s: %w", commitData.GetSHA(), err)
	}

	for _, summary := range summaries {
		commitData.Conclusion = establishPRChecksConclusion(commitData.Conclusion, summary.conclusion)
		commitData.Cost += summary.cost
		if summary.costEstimate {
			commitData.CostEstimate = true
		}
		if summary.costGathered {
			commitData.CostGathered = true
		}
		if summary.start.Before(commitData.StartActionsTime) || commitData.StartActionsTime.IsZero() {
			commitData.StartActionsTime = summary.start
		}
		if summary.end.After(commitData.EndActionsTime) {
			commitData.EndActionsTime = summary.end
		}
	}
	commitData.WorkflowRunIDs = workflowRunIDs
	commitData.WorkflowRuns = workflowRuns

	return nil
}
