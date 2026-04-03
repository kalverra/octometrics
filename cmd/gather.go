// Package cmd implements the CLI commands for octometrics.
package cmd

import (
	"fmt"
	"os"
	"time"

	"charm.land/huh/v2/spinner"
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

It can be used to gather data for a specific workflow run, pull request, or commit,
or all data within a certain time frame using --from and --to.
Filter gathered data by event type (e.g. pull_request, push, merge_group) using --event.
In-progress workflows are supported and will be displayed with an active status indicator.
`,
	Example: `
# To see all workflows run on all commits a part of this PR (including merge queue runs): https://github.com/kalverra/octometrics/pull/33
octometrics gather -o kalverra -r octometrics -p 33

# To see all workflows run on a specific commit: https://github.com/kalverra/octometrics/pull/33/changes/94ad3f7e2f45852a99791326847ea12c94b964dc
octometrics gather -o kalverra -r octometrics -c 94ad3f7e2f45852a99791326847ea12c94b964dc

# To see a specific workflow run: https://github.com/kalverra/octometrics/actions/runs/22918636165
octometrics gather -o kalverra -r octometrics -w 22918636165

# To gather all data in a certain time frame:
octometrics gather -o kalverra -r octometrics --from 2025-01-01 --to 2025-01-07

# Filter gathered data by event type:
octometrics gather -o kalverra -r octometrics --from 2025-01-01 --to 2025-01-07 --event pull_request

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
		} else if !cfg.From.IsZero() && !cfg.To.IsZero() {
			logger = logger.With().Time("from", cfg.From).Time("to", cfg.To).Logger()
		}

		logger.Info().Msg("Gathering data")

		opts := []gather.Option{
			gather.CustomDataFolder(cfg.DataDir),
		}

		if cfg.ForceUpdate {
			opts = append(opts, gather.ForceUpdate())
		}

		if cfg.GatherCost {
			opts = append(opts, gather.WithCost())
		}

		var err error
		action := func() {
			if cfg.WorkflowRunID != 0 {
				_, _, err = gather.WorkflowRun(logger, githubClient, cfg.Owner, cfg.Repo, cfg.WorkflowRunID, opts...)
			} else if cfg.PullRequestNumber != 0 {
				_, err = gather.PullRequest(logger, githubClient, cfg.Owner, cfg.Repo, cfg.PullRequestNumber, opts...)
			} else if cfg.CommitSHA != "" {
				_, err = gather.Commit(logger, githubClient, cfg.Owner, cfg.Repo, cfg.CommitSHA, opts...)
			} else if !cfg.From.IsZero() && !cfg.To.IsZero() {
				err = gather.Range(logger, githubClient, cfg.Owner, cfg.Repo, cfg.From, cfg.To, cfg.Event, opts...)
			}
		}

		spinnerErr := spinner.New().
			Title("Gathering data").
			Action(action).
			Run()

		if err != nil {
			return err
		}
		if spinnerErr != nil {
			return spinnerErr
		}

		logger.Info().Str("duration", time.Since(startTime).String()).Msg("Gathered data")
		fmt.Printf("Gathered data (%s) ✅\n", time.Since(startTime).String())

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
		} else if !cfg.From.IsZero() && !cfg.To.IsZero() {
			pagePath = fmt.Sprintf("/%s/%s/", cfg.Owner, cfg.Repo)
		}

		if err := os.RemoveAll(observe.OutputDir); err != nil {
			return fmt.Errorf("failed to clean observe output: %w", err)
		}
		observeOpts := []observe.Option{
			observe.ExcludeWorkflows(cfg.ExcludeWorkflows),
		}
		return observe.Interactive(logger, githubClient, pagePath, cfg.DataDir, observeOpts...)
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
	gatherCmd.Flags().
		Time("from", time.Time{}, []string{"2006-01-02", "2006-01-02T15:04:05Z"}, "Start date for gathering data (YYYY-MM-DD)")
	gatherCmd.Flags().
		Time("to", time.Time{}, []string{"2006-01-02", "2006-01-02T15:04:05Z"}, "End date for gathering data (YYYY-MM-DD)")
	gatherCmd.Flags().
		String("event", "all", "Filter gathered data by event type (all, pull_request, merge_group, push)")
	gatherCmd.Flags().StringP("github-token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	gatherCmd.Flags().
		Bool("gather-cost", false, "Gather cost data for workflow runs (can significantly increase runtime)")
	gatherCmd.Flags().StringSlice("exclude-workflows", nil,
		"Omit workflow display names from observations after gather (comma-separated or repeat flag)")

	rootCmd.AddCommand(gatherCmd)
}
