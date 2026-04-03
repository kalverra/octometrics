package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/observe"
)

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
		return os.RemoveAll(observe.OutputDir)
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		observeOpts := []observe.Option{
			observe.ExcludeWorkflows(cfg.ExcludeWorkflows),
		}
		return observe.Interactive(logger, githubClient, "", cfg.DataDir, observeOpts...)
	},
}

func init() {
	observeCmd.Flags().StringSlice("exclude-workflows", nil,
		"Omit workflow display names from observations (comma-separated or repeat flag)")
	rootCmd.AddCommand(observeCmd)
}
