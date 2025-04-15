package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const CommitsDataDir = "commits"

// CommitData contains the commit data for a given commit SHA.
// It also includes some additional info that makes it easier to map to its associated workflows.
type CommitData struct {
	*github.RepositoryCommit
	Owner            string             `json:"owner"`
	Repo             string             `json:"repo"`
	CheckRuns        []*github.CheckRun `json:"check_runs"`
	MergeQueueEvents []*MergeQueueEvent `json:"merge_queue_events"`
	WorkflowRunIDs   []int64            `json:"workflow_run_ids"`
	StartActionsTime time.Time          `json:"start_actions_time"`
	EndActionsTime   time.Time          `json:"end_actions_time"`
	Conclusion       string             `json:"conclusion"`
	Cost             int64              `json:"cost"`
	comparisonMutex  sync.Mutex         `json:"-"`
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
		fileExists = false
	)

	log = log.With().
		Str("target_file", targetFile).
		Str("commit_sha", sha).
		Logger()

	err := os.MkdirAll(targetDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to make data dir '%s': %w", WorkflowRunsDataDir, err)
	}

	if _, err := os.Stat(targetFile); err == nil {
		fileExists = true
	}

	startTime := time.Now()

	if !forceUpdate && fileExists {
		log = log.With().
			Str("source", "local file").
			Logger()
		commitFileBytes, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open commit file: %w", err)
		}
		err = json.Unmarshal(commitFileBytes, &commitData)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal commit data: %w", err)
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
		return nil, fmt.Errorf("GitHub client is nil")
	}

	ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
	commit, resp, err := client.Rest.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	cancel()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	commitData.RepositoryCommit = commit
	commitData.CheckRuns, err = checkRunsForCommit(client, owner, repo, sha)
	if err != nil {
		return nil, err
	}
	err = setWorkflowRunsForCommit(log, client, owner, repo, commitData.CheckRuns, commitData, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to gather workflow runs for commit '%s': %w", sha, err)
	}

	data, err := json.Marshal(commitData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commit data to json '%s': %w", sha, err)
	}
	err = os.WriteFile(targetFile, data, 0600)
	if err != nil {
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
			Filter: github.Ptr("all"),
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}
		resp *github.Response
		err  error
	)

	for {
		var checkRuns *github.ListCheckRunsResults
		ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
		checkRuns, resp, err = client.Rest.Checks.ListCheckRunsForRef(ctx, owner, repo, sha, listOpts)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to gather check runs from GitHub for commit '%s': %w", sha, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}
		allCheckRuns = append(allCheckRuns, checkRuns.CheckRuns...)

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
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
		workflowRunIDRe   = regexp.MustCompile(`\/actions\/runs\/(\d+)`)
		eg                errgroup.Group
	)

	for _, checkRun := range checkRuns {
		if checkRun.GetStatus() == "completed" {
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
		} else {
			log.Warn().Str("Check Run", checkRun.GetName()).Msg("Check run is not yet completed, skipping")
		}
	}

	for workflowRunID := range workflowRunIDsSet {
		eg.Go(func(workflowRunID int64) func() error {
			return func() error {
				workflowRun, _, err := WorkflowRun(log, client, owner, repo, workflowRunID, opts...)
				if err != nil {
					return fmt.Errorf("failed to gather workflow run data for commit %s: %w", commitData.GetSHA(), err)
				}
				commitData.comparisonMutex.Lock()
				defer commitData.comparisonMutex.Unlock()
				commitData.Conclusion = establishPRChecksConclusion(commitData.Conclusion, workflowRun.GetConclusion())
				commitData.Cost += workflowRun.GetCost()
				if workflowRun.GetRunStartedAt().Before(commitData.StartActionsTime) ||
					commitData.StartActionsTime.IsZero() {
					commitData.StartActionsTime = workflowRun.GetRunStartedAt().Time
				}
				if workflowRun.GetRunCompletedAt().After(commitData.EndActionsTime) {
					commitData.EndActionsTime = workflowRun.GetRunCompletedAt()
				}
				commitData.WorkflowRunIDs = append(commitData.WorkflowRunIDs, workflowRunID)
				return nil
			}
		}(workflowRunID))
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to gather workflow runs for commit %s: %w", commitData.GetSHA(), err)
	}

	return nil
}
