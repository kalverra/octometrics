package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const PullRequestsDataDir = "pull_requests"

type PullRequestData struct {
	*github.PullRequest
	CommitData []*CommitData `json:"commit_data"`
}

func (p *PullRequestData) GetCommitData() []*CommitData {
	return p.CommitData
}

func PullRequest(
	log zerolog.Logger,
	client *github.Client,
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

	startTime := time.Now()

	log.Debug().Int("pull_request_number", pullRequestNumber).Msg("Gathering pull request data")

	if !forceUpdate && fileExists {
		//nolint:gosec // I don't care
		prFileBytes, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open workflow run file: %w", err)
		}
		err = json.Unmarshal(prFileBytes, &pullRequestData)
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Int("pull_request_number", pullRequestNumber).
			Msg("Gathered pull request data")
		return pullRequestData, err
	}

	if client == nil {
		return nil, fmt.Errorf("GitHub client is nil")
	}

	log.Debug().Msg("Gathering pull request data from GitHub")

	ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
	pr, resp, err := client.PullRequests.Get(ctx, owner, repo, pullRequestNumber)
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

	pullRequestData.PullRequest = pr
	// Get the commits associated with the pull request
	prCommits, err := prCommits(client, owner, repo, pullRequestNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to gather commits for pull request %d: %w", pullRequestNumber, err)
	}

	pullRequestData.CommitData, err = prCommitData(log, client, owner, repo, prCommits)
	if err != nil {
		return nil, fmt.Errorf("failed to gather commit data for pull request %d: %w", pullRequestNumber, err)
	}

	data, err := json.Marshal(pullRequestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pull request data to json for pull request %d: %w", pullRequestNumber, err)
	}
	err = os.WriteFile(targetFile, data, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to write pull request data to file for pull request %d: %w", pullRequestNumber, err)
	}
	log.Debug().
		Str("duration", time.Since(startTime).String()).
		Int("pull_request_number", pullRequestNumber).
		Msg("Gathered pull request data")
	return pullRequestData, nil
}

func prCommits(
	client *github.Client,
	owner, repo string,
	pullRequestNumber int,
) ([]*github.RepositoryCommit, error) {
	var (
		commits  []*github.RepositoryCommit
		listOpts = &github.ListOptions{
			PerPage: 100,
		}
	)

	for {
		ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
		commitsPage, resp, err := client.PullRequests.ListCommits(ctx, owner, repo, pullRequestNumber, listOpts)
		cancel()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}

		commits = append(commits, commitsPage...)

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return commits, nil
}

func prCommitData(
	log zerolog.Logger,
	client *github.Client,
	owner, repo string,
	prCommits []*github.RepositoryCommit,
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
