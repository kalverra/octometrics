// Package report generates monitoring reports for GitHub Actions workflow runs.
package report

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"
)

type ghaContext struct {
	RunID       int64
	JobName     string
	Owner       string
	Repo        string
	Ref         string
	Token       string
	StepSummary string
	ServerURL   string
	SHA         string
}

func detectGitHubActions() (*ghaContext, error) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return nil, fmt.Errorf("not running in GitHub Actions (GITHUB_ACTIONS != true)")
	}

	repository := os.Getenv("GITHUB_REPOSITORY")
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GITHUB_REPOSITORY format: %q", repository)
	}

	runIDStr := os.Getenv("GITHUB_RUN_ID")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GITHUB_RUN_ID %q: %w", runIDStr, err)
	}

	return &ghaContext{
		RunID:       runID,
		JobName:     os.Getenv("GITHUB_JOB_NAME"),
		Owner:       parts[0],
		Repo:        parts[1],
		Ref:         os.Getenv("GITHUB_REF"),
		Token:       os.Getenv("GITHUB_TOKEN"),
		StepSummary: os.Getenv("GITHUB_STEP_SUMMARY"),
		ServerURL:   os.Getenv("GITHUB_SERVER_URL"),
		SHA:         os.Getenv("GITHUB_SHA"),
	}, nil
}

// pullRequestNumber extracts the PR number from GITHUB_REF (e.g. "refs/pull/42/merge").
func (g *ghaContext) pullRequestNumber() (int, bool) {
	if !strings.HasPrefix(g.Ref, "refs/pull/") {
		return 0, false
	}
	trimmed := strings.TrimPrefix(g.Ref, "refs/pull/")
	numStr, _, found := strings.Cut(trimmed, "/")
	if !found {
		return 0, false
	}
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, false
	}
	return num, true
}

func (g *ghaContext) newGitHubClient() *github.Client {
	client := github.NewClient(nil)
	if g.Token != "" {
		client = client.WithAuthToken(g.Token)
	}
	return client
}

// fetchJobSteps retrieves step timing for the current job from the GitHub Actions API.
func fetchJobSteps(log zerolog.Logger, gha *ghaContext) ([]*github.TaskStep, error) {
	if gha.Token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN not set, cannot fetch job steps")
	}
	if gha.JobName == "" {
		return nil, fmt.Errorf("GITHUB_JOB_NAME not set, cannot match job")
	}

	client := gha.newGitHubClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &github.ListWorkflowJobsOptions{
		Filter:      "latest",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allJobNames []string
	for {
		jobs, resp, err := client.Actions.ListWorkflowJobs(ctx, gha.Owner, gha.Repo, gha.RunID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list workflow jobs: %w", err)
		}

		for _, job := range jobs.Jobs {
			if job.GetName() == gha.JobName {
				log.Debug().
					Str("job_name", gha.JobName).
					Int("step_count", len(job.Steps)).
					Msg("Found matching job")
				return job.Steps, nil
			}
			allJobNames = append(allJobNames, job.GetName())
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return nil, fmt.Errorf("job %q not found among %d workflow jobs", gha.JobName, len(allJobNames))
}
