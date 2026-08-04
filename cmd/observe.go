package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/config"
	"github.com/kalverra/octometrics/observe"
)

func buildObserveOptions(cfg *config.Config) []observe.Option {
	gatherOpts := []gather.Option{
		gather.CustomDataFolder(cfg.DataDir),
	}
	if cfg.ForceUpdate {
		gatherOpts = append(gatherOpts, gather.ForceUpdate())
	}
	if !cfg.ExcludeCosts {
		gatherOpts = append(gatherOpts, gather.WithCost())
	} else {
		gatherOpts = append(gatherOpts, gather.WithoutCost())
	}

	return []observe.Option{
		observe.WithGatherOptions(gatherOpts...),
		observe.ExcludeWorkflows(cfg.ExcludeWorkflows),
		observe.IncludeWorkflows(cfg.IncludeWorkflows),
	}
}

var observeCmd = &cobra.Command{
	Use:   "observe",
	Short: "Observe metrics from GitHub",
	Long: `Observe metrics from GitHub.

Display the gathered Workflow/Job/Step data in your browser.`,
	Example: `octometrics observe # Display all of your gathered Workflow/Job/Step data in your browser`,
	PreRunE: func(_ *cobra.Command, _ []string) error {
		var err error
		githubClient, err = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		rebuild, _ := cmd.Flags().GetBool("rebuild-manifest")
		if rebuild {
			if err := observe.RebuildManifest(logger, cfg.DataDir); err != nil {
				return fmt.Errorf("failed to rebuild manifest: %w", err)
			}
			logger.Info().Msg("Manifest rebuilt successfully")
		}
		return observe.Interactive(logger, githubClient, "", cfg.DataDir, buildObserveOptions(cfg)...)
	},
}

func init() {
	observeCmd.Flags().Bool("rebuild-manifest", false, "Rebuild manifest.jsonl files from local data directory")
	observeCmd.Flags().StringSlice("exclude-workflows", nil,
		"Omit workflow display names from observations (comma-separated or repeat flag)")
	rootCmd.AddCommand(observeCmd)
}
