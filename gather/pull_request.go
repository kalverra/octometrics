package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog/log"
)

const pullRequestsDir = "pull_requests"

type PullRequestData struct {
	*github.PullRequest
}

func PullRequest(client *github.Client, owner, repo string, pullRequestNumber int, forceUpdate bool) (*PullRequestData, error) {
	var (
		pullRequestData = &PullRequestData{}
		targetDir       = filepath.Join(dataDir, owner, repo, pullRequestsDir)
		targetFile      = filepath.Join(targetDir, fmt.Sprintf("%d.json", pullRequestNumber))
		fileExists      = false
	)

	err := os.MkdirAll(targetDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to make data dir '%s': %w", workflowRunsDir, err)
	}

	if _, err := os.Stat(targetFile); err == nil {
		fileExists = true
	}

	var (
		startTime  = time.Now()
		successLog = log.Info().
				Str("duration", time.Since(startTime).String()).
				Int("pull_request_number", pullRequestNumber)
	)

	if !forceUpdate && fileExists {
		prFileBytes, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open workflow run file: %w", err)
		}
		err = json.Unmarshal(prFileBytes, &pullRequestData)
		successLog.Msg("Gathered pull request data")
		return pullRequestData, err
	}

	ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, pullRequestNumber)
	cancel()
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, fmt.Errorf("pull request '%d' not found on GitHub", pullRequestNumber)
	}

	pullRequestData.PullRequest = pr

	data, err := json.Marshal(pullRequestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pull request data to json for pull request '%d': %w", pullRequestNumber, err)
	}
	err = os.WriteFile(targetFile, data, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write pull request data to file for pull request '%d': %w", pullRequestNumber, err)
	}
	successLog.Msg("Gathered pull request data")
	return pullRequestData, nil
}
