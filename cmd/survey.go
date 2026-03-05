package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/config"
	"github.com/kalverra/octometrics/observe"
)

var surveyCmd = &cobra.Command{
	Use:   "survey",
	Short: "Survey CI suite run times across a time period and identify p50/p75/p95 runs",
	Long: `Survey lists all completed workflow runs for a repository in a given time period,
groups them by commit to compute per-commit CI suite duration, and identifies the
commits at the p50, p75, and p95 percentiles. Detailed data is then gathered only
for those representative commits, keeping API usage minimal.`,
	PreRunE: func(_ *cobra.Command, _ []string) error {
		if err := cfg.ValidateSurvey(); err != nil {
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
		startTime := time.Now()

		logger = logger.With().
			Str("owner", cfg.Owner).
			Str("repo", cfg.Repo).
			Str("event", cfg.Event).
			Time("since", cfg.Since).
			Time("until", cfg.Until).
			Logger()

		fmt.Printf("Surveying %s/%s workflow runs from %s to %s...\n",
			cfg.Owner, cfg.Repo,
			cfg.Since.Format("2006-01-02"), cfg.Until.Format("2006-01-02"),
		)

		opts := []gather.Option{}
		if cfg.ForceUpdate {
			opts = append(opts, gather.ForceUpdate())
		}

		result, err := gather.Survey(logger, githubClient, cfg)
		if err != nil {
			return fmt.Errorf("survey failed: %w", err)
		}

		fmt.Printf("Analyzed %d commits from %d workflow runs\n", len(result.Commits), result.TotalRuns)

		for _, label := range []string{"p50", "p75", "p95"} {
			if cs, ok := result.Percentiles[label]; ok {
				fmt.Printf("  %s: %s (commit %s, %d workflows)\n",
					label, cs.Duration.Round(time.Second), cs.SHA[:7], len(cs.WorkflowRuns),
				)
			}
		}

		// Phase 2: gather detailed data for representative commits
		fmt.Println("\nGathering detailed data for percentile commits...")
		for label, cs := range result.Percentiles {
			fmt.Printf("  Gathering %s commit %s...\n", label, cs.SHA[:7])
			_, err := gather.Commit(logger, githubClient, cfg.Owner, cfg.Repo, cs.SHA, opts...)
			if err != nil {
				logger.Warn().Err(err).Str("label", label).Str("sha", cs.SHA).
					Msg("Failed to gather detailed data for percentile commit")
				fmt.Printf("  Warning: failed to gather details for %s commit %s: %v\n", label, cs.SHA, err)
			}
		}

		logger.Info().Str("duration", time.Since(startTime).String()).Msg("Survey complete")
		fmt.Printf("\nSurvey complete in %s\n", time.Since(startTime).Round(time.Second))

		if cfg.NoObserve {
			return nil
		}

		if err := os.RemoveAll(observe.OutputDir); err != nil {
			return fmt.Errorf("failed to clean observe output: %w", err)
		}

		surveyFile := fmt.Sprintf("/%s/%s/surveys/%s.html",
			cfg.Owner, cfg.Repo,
			gather.SurveyFileBaseName(cfg.Event, cfg.Since, cfg.Until),
		)
		return observe.Interactive(logger, githubClient, surveyFile)
	},
}

func init() {
	surveyCmd.Flags().StringP("owner", "o", "", "Repository owner")
	surveyCmd.Flags().StringP("repo", "r", "", "Repository name")
	surveyCmd.Flags().String("event", "all", "Filter by event type (all, pull_request, merge_group, push)")
	surveyCmd.Flags().Time("since", config.DefaultSince, []string{"2006-01-02"}, "Start analysis date")
	surveyCmd.Flags().Time("until", config.DefaultUntil, []string{"2006-01-02"}, "End analysis date")
	surveyCmd.Flags().StringP("github_token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	surveyCmd.Flags().BoolP("force_update", "u", false, "Force update of existing data")
	surveyCmd.Flags().Bool("no_observe", false, "Skip launching the interactive observer after survey")

	rootCmd.AddCommand(surveyCmd)
}
