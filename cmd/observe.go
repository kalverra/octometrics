package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/observe"
)

var (
	outputTypes []string
)

var observeCmd = &cobra.Command{
	Use:   "observe",
	Short: "Observe metrics from GitHub",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return os.RemoveAll(observe.OutputDir)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Debug().
			Strs("output-types", outputTypes).
			Msg("observe flags")

		// if workflowRunID != 0 {
		// 	err := observe.WorkflowRun(githubClient, owner, repo, workflowRunID, outputTypes)
		// 	if err != nil {
		// 		return fmt.Errorf("failed to observe workflow run: %w", err)
		// 	}
		// }

		// if pullRequestID != 0 {
		// 	err := observe.PullRequest(githubClient, owner, repo, pullRequestID, outputTypes)
		// 	if err != nil {
		// 		return fmt.Errorf("failed to observe pull request: %w", err)
		// 	}
		// }

		return observe.Interactive(logger, githubClient)
	},
}

func init() {
	rootCmd.AddCommand(observeCmd)

	observeCmd.Flags().StringArrayVar(&outputTypes, "output-types", []string{"html", "md"}, "Output types to generate")
}
