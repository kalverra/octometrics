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
	githubToken  string
	githubClient *gather.GitHubClient
	forceUpdate  bool
	noObserve    bool
)

var gatherCmd = &cobra.Command{
	Use:   "gather",
	Short: "Gather metrics from GitHub",
	PreRunE: func(_ *cobra.Command, _ []string) error {
		if err := cfg.ValidateGather(); err != nil {
			return err
		}
		var err error
		githubClient, err = gather.NewGitHubClient(logger, githubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return nil
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		logger.Debug().
			Bool("force-update", forceUpdate).
			Msg("gather flags")

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

		if forceUpdate {
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

		if noObserve {
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
		BoolVar(&noObserve, "no-observe", false, "Skip launching the interactive observer after gathering")
	gatherCmd.Flags().BoolVarP(&forceUpdate, "force-update", "u", false, "Force update of existing data")
	gatherCmd.Flags().StringP("owner", "o", "", "Repository owner")
	gatherCmd.Flags().StringP("repo", "r", "", "Repository name")
	gatherCmd.Flags().StringP("commit-sha", "c", "", "Commit SHA")
	gatherCmd.Flags().Int64P("workflow_run_id", "w", 0, "Workflow run ID")
	gatherCmd.Flags().IntP("pull_request_number", "p", 0, "Pull request number")
	gatherCmd.Flags().StringP("github_token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")

	rootCmd.AddCommand(gatherCmd)
}
