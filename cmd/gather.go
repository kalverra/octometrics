package cmd

import (
	"github.com/kalverra/workflow-metrics/gather"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const githubTokenEnvVar = "GITHUB_TOKEN"

var (
	githubToken string
	forceUpdate bool
)

var gatherCmd = &cobra.Command{
	Use:   "gather",
	Short: "Gather metrics from GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Debug().
			Bool("force-update", forceUpdate).
			Msg("gather flags")

		if workflowRunID != 0 {
			_, err := gather.WorkflowRun(githubClient, owner, repo, workflowRunID, forceUpdate)
			return err
		}

		if pullRequestID != 0 {
			_, err := gather.PullRequest(githubClient, owner, repo, pullRequestID, forceUpdate)
			return err
		}

		return nil
	},
}

func init() {
	gatherCmd.Flags().BoolVarP(&forceUpdate, "force-update", "u", false, "Force update of existing data")

	rootCmd.AddCommand(gatherCmd)
}
