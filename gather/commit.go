// Package gather provides functions for gathering GitHub Actions data
// including commits, pull requests, and workflow runs.
package gather

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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
	CheckRuns          []*github.CheckRun `json:"check_runs"`
	MergeQueueEvents   []*MergeQueueEvent `json:"merge_queue_events"`
	WorkflowRunIDs     []int64            `json:"workflow_run_ids"`
	WorkflowRuns       []*WorkflowRunData `json:"workflow_runs,omitempty"`
	StartActionsTime   time.Time          `json:"start_actions_time"`
	EndActionsTime     time.Time          `json:"end_actions_time"`
	Conclusion         string             `json:"conclusion"`
	Cost               int64              `json:"cost"`
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

// GetCost returns the total cost of all actions that ran for the commit in tenths of a cent.
func (c *CommitData) GetCost() int64 {
	if c == nil {
		return 0
	}
	return c.Cost
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

// Commit gathers commit data for a given commit SHA and enhances matches it with workflows that ran on that commit.
func Commit(
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
		forceUpdate = options.ForceUpdate

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

	fileExists := cacheFileExists(targetFile)

	startTime := time.Now()

	if !forceUpdate && fileExists {
		log = log.With().
			Str("source", "local file").
			Logger()
		commitData, err := readJSONFile[*CommitData](targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open commit file: %w", err)
		}
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered commit data")
		return commitData, nil
	}

	log = log.With().
		Str("source", "GitHub API").
		Logger()

	if client == nil {
		return nil, fmt.Errorf("github client is nil")
	}

	ctx, cancel := ghCtx()
	commit, resp, err := client.Rest.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	cancel()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	commitData.RepositoryCommit = commit
	checkRuns, err := checkRunsForCommit(client, owner, repo, sha)
	if err != nil {
		return nil, err
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
			parentCheckRuns, err := checkRunsForCommit(client, owner, repo, parentSHA)
			if err != nil {
				log.Warn().
					Str("parent_sha", parentSHA).
					Err(err).
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
	err = setWorkflowRunsForCommit(log, client, owner, repo, commitData.CheckRuns, commitData, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to gather workflow runs for commit '%s': %w", sha, err)
	}

	if err := writeJSONFile(targetFile, commitData); err != nil {
		return nil, fmt.Errorf("failed to write commit data to file '%s': %w", sha, err)
	}

	log.Debug().
		Str("duration", time.Since(startTime).String()).
		Msg("Gathered commit data")
	return commitData, nil
}

func checkRunsForCommit(
	client *GitHubClient,
	owner, repo string,
	sha string,
) ([]*github.CheckRun, error) {
	var (
		allCheckRuns []*github.CheckRun
		listOpts     = &github.ListCheckRunsOptions{
			Filter: new("all"),
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}
	)

	ctx, cancel := ghCtx()
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
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	checkRuns []*github.CheckRun,
	commitData *CommitData,
	opts []Option,
) error {
	var (
		workflowRunIDsSet = map[int64]struct{}{}
		eg                errgroup.Group
	)

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
		id         int64
		conclusion string
		cost       int64
		start      time.Time
		end        time.Time
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
				workflowRun, _, err := WorkflowRun(log, client, owner, repo, workflowRunID, commitOpts...)
				if err != nil {
					return fmt.Errorf("failed to gather workflow run data for commit %s: %w", commitData.GetSHA(), err)
				}
				conclusion := workflowRun.GetConclusion()
				if conclusion == "" {
					conclusion = workflowRun.GetStatus()
				}
				summaries[index] = workflowRunSummary{
					id:         workflowRunID,
					conclusion: conclusion,
					cost:       workflowRun.GetCost(),
					start:      workflowRun.GetRunStartedAt().Time,
					end:        workflowRun.GetRunCompletedAt(),
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
