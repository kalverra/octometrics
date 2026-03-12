// Package cmd implements the CLI commands for octometrics.
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/observe"
)

var (
	githubClient *gather.GitHubClient
)

var gatherCmd = &cobra.Command{
	Use:   "gather",
	Short: "Gather metrics from GitHub",
	Long: `Gather metrics from GitHub.

Read workflow runtime data from GitHub to display in the browser.

It can be used to gather data for a specific workflow run, pull request, or commit.
In-progress workflows are supported and will be displayed with an active status indicator.
`,
	Example: `
# To see all workflows run on all commits a part of this PR (including merge queue runs): https://github.com/kalverra/octometrics/pull/33
octometrics gather -o kalverra -r octometrics -p 33

# To see all workflows run on a specific commit: https://github.com/kalverra/octometrics/pull/33/changes/94ad3f7e2f45852a99791326847ea12c94b964dc
octometrics gather -o kalverra -r octometrics -c 94ad3f7e2f45852a99791326847ea12c94b964dc

# To see a specific workflow run: https://github.com/kalverra/octometrics/actions/runs/22918636165
octometrics gather -o kalverra -r octometrics -w 22918636165

# Use '-u' to force update local data if it already exists
octometrics gather -o kalverra -r octometrics -p 33 -u
`,
	PreRunE: func(_ *cobra.Command, _ []string) error {
		if err := cfg.ValidateGather(); err != nil {
			return err
		}
		var err error
		githubClient, err = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return nil
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		startTime := time.Now()

		logger = logger.With().Str("owner", cfg.Owner).Str("repo", cfg.Repo).Logger()

		if cfg.WorkflowRunID != 0 {
			logger = logger.With().Int64("workflow_run_id", cfg.WorkflowRunID).Logger()
		} else if cfg.PullRequestNumber != 0 {
			logger = logger.With().Int("pull_request_number", cfg.PullRequestNumber).Logger()
		}

		logger.Info().Msg("Gathering data")
		fmt.Println("Gathering data...")

		opts := []gather.Option{}

		if cfg.ForceUpdate {
			opts = append(opts, gather.ForceUpdate())
		}

		var err error
		if cfg.WorkflowRunID != 0 {
			_, _, err = gather.WorkflowRun(logger, githubClient, cfg.Owner, cfg.Repo, cfg.WorkflowRunID, opts...)
		} else if cfg.PullRequestNumber != 0 {
			_, err = gather.PullRequest(logger, githubClient, cfg.Owner, cfg.Repo, cfg.PullRequestNumber, opts...)
		} else if cfg.CommitSHA != "" {
			_, err = gather.Commit(logger, githubClient, cfg.Owner, cfg.Repo, cfg.CommitSHA, opts...)
		}
		if err != nil {
			return err
		}

		logger.Info().Str("duration", time.Since(startTime).String()).Msg("Gathered data")
		fmt.Println("Gathered data")

		if cfg.NoObserve {
			return nil
		}

		var pagePath string
		if cfg.WorkflowRunID != 0 {
			pagePath = fmt.Sprintf("/%s/%s/workflow_runs/%d.html", cfg.Owner, cfg.Repo, cfg.WorkflowRunID)
		} else if cfg.PullRequestNumber != 0 {
			pagePath = fmt.Sprintf("/%s/%s/pull_requests/%d.html", cfg.Owner, cfg.Repo, cfg.PullRequestNumber)
		} else if cfg.CommitSHA != "" {
			pagePath = fmt.Sprintf("/%s/%s/commits/%s.html", cfg.Owner, cfg.Repo, cfg.CommitSHA)
		}

		if err := os.RemoveAll(observe.OutputDir); err != nil {
			return fmt.Errorf("failed to clean observe output: %w", err)
		}
		return observe.Interactive(logger, githubClient, pagePath)
	},
}

func init() {
	gatherCmd.Flags().
		Bool("no-observe", false, "Skip launching the interactive observer after gathering data")
	gatherCmd.Flags().BoolP("force-update", "u", false, "Force update of existing data")
	gatherCmd.Flags().StringP("owner", "o", "", "Repository owner")
	gatherCmd.Flags().StringP("repo", "r", "", "Repository name")
	gatherCmd.Flags().StringP("commit-sha", "c", "", "Commit SHA")
	gatherCmd.Flags().Int64P("workflow-run-id", "w", 0, "Workflow run ID")
	gatherCmd.Flags().IntP("pull-request-number", "p", 0, "Pull request number")
	gatherCmd.Flags().StringP("github-token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	gatherCmd.Flags().
		Bool("gather-cost", false, "Gather cost data for workflow runs (can significantly increase runtime)")

	rootCmd.AddCommand(gatherCmd)
}
