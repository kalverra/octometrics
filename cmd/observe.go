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
	PreRunE: func(_ *cobra.Command, _ []string) error {
		var err error
		githubClient, err = gather.NewGitHubClient(logger, githubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return os.RemoveAll(observe.OutputDir)
	},
	RunE: func(_ *cobra.Command, _ []string) error {
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

		return observe.Interactive(logger, githubClient, "")
	},
}

func init() {
	rootCmd.AddCommand(observeCmd)
}
