package cmd

import (
	"errors"
	"fmt"
	"time"

	"charm.land/huh/v2/spinner"
	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/observe"
)

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare two workflow runs or commits side-by-side",
	Long: `Compare two workflow runs or commits side-by-side.

Shows stacked Gantt charts, a comparison table with duration deltas and status changes,
and highlights items that only appear in one of the two runs.`,
	Example: `
# Compare two workflow runs
octometrics compare -o kalverra -r octometrics --workflow-runs 123,456

# Compare two commits
octometrics compare -o kalverra -r octometrics --commits abc123,def456

# Force re-fetch data from GitHub
octometrics compare -o kalverra -r octometrics --workflow-runs 123,456 -u
`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		if err := cfg.ValidateCompare(); err != nil {
			return err
		}

		workflowRuns, _ := cmd.Flags().GetInt64Slice("workflow-runs")
		commits, _ := cmd.Flags().GetStringSlice("commits")

		hasWorkflowRuns := len(workflowRuns) > 0
		hasCommits := len(commits) > 0

		if hasWorkflowRuns && hasCommits {
			return errors.New("only one of --workflow-runs or --commits can be provided")
		}
		if !hasWorkflowRuns && !hasCommits {
			return errors.New("one of --workflow-runs or --commits must be provided")
		}
		if hasWorkflowRuns && len(workflowRuns) != 2 {
			return fmt.Errorf("--workflow-runs requires exactly 2 IDs, got %d", len(workflowRuns))
		}
		if hasCommits && len(commits) != 2 {
			return fmt.Errorf("--commits requires exactly 2 SHAs, got %d", len(commits))
		}

		var err error
		githubClient, err = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		startTime := time.Now()
		logger = logger.With().Str("owner", cfg.Owner).Str("repo", cfg.Repo).Logger()

		workflowRuns, _ := cmd.Flags().GetInt64Slice("workflow-runs")
		commits, _ := cmd.Flags().GetStringSlice("commits")

		var opts []observe.Option
		if cfg.ForceUpdate {
			opts = append(opts, observe.WithGatherOptions(gather.ForceUpdate()))
		}

		var (
			comparison *observe.Comparison
			err        error
		)

		spinnerErr := spinner.New().
			Title("Building comparison").
			Action(func() {
				if len(workflowRuns) == 2 {
					comparison, err = observe.CompareWorkflowRuns(
						logger, githubClient,
						cfg.Owner, cfg.Repo,
						workflowRuns[0], workflowRuns[1],
						opts...,
					)
				} else {
					comparison, err = observe.CompareCommits(
						logger, githubClient,
						cfg.Owner, cfg.Repo,
						commits[0], commits[1],
						opts...,
					)
				}
			}).
			Run()
		if err != nil {
			return err
		}
		if spinnerErr != nil {
			return spinnerErr
		}

		pagePath, err := comparison.Render(logger)
		if err != nil {
			return fmt.Errorf("failed to render comparison: %w", err)
		}

		if err := observe.EnsureCompareObservationLinks(logger, githubClient, comparison, opts...); err != nil {
			return fmt.Errorf("failed to render pages linked from comparison: %w", err)
		}

		logger.Info().
			Str("duration", time.Since(startTime).String()).
			Str("page", pagePath).
			Msg("Comparison built")
		fmt.Printf("Comparison built (%s)\n", time.Since(startTime).String())

		return observe.ServeHTML(logger, pagePath)
	},
}

func init() {
	compareCmd.Flags().StringP("owner", "o", "", "Repository owner")
	compareCmd.Flags().StringP("repo", "r", "", "Repository name")
	compareCmd.Flags().StringP("github-token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	compareCmd.Flags().BoolP("force-update", "u", false, "Force update of existing data")
	compareCmd.Flags().Int64Slice("workflow-runs", nil, "Two workflow run IDs to compare (comma-separated)")
	compareCmd.Flags().StringSlice("commits", nil, "Two commit SHAs to compare (comma-separated)")

	rootCmd.AddCommand(compareCmd)
}
