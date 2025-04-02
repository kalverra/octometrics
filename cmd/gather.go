package cmd

import (
	"errors"
	"time"

	"github.com/kalverra/octometrics/gather"
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
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if owner == "" {
			return errors.New("owner must be provided")
		}
		if repo == "" {
			return errors.New("repo must be provided")
		}

		if workflowRunID == 0 && pullRequestNumber == 0 {
			return errors.New("either workflow run ID or pull request ID must be provided")
		}
		if workflowRunID != 0 && pullRequestNumber != 0 {
			return errors.New("only one of workflow run ID or pull request ID must be provided")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Debug().
			Bool("force-update", forceUpdate).
			Msg("gather flags")

		startTime := time.Now()

		log.Info().
			Str("owner", owner).
			Str("repo", repo).
			Int64("workflow_run_id", workflowRunID).
			Int("pull_request_number", pullRequestNumber).
			Msg("Gathering data from GitHub")

		opts := []gather.Option{}

		if forceUpdate {
			opts = append(opts, gather.ForceUpdate())
		}

		if workflowRunID != 0 {
			_, err := gather.WorkflowRun(logger, githubClient, owner, repo, workflowRunID, opts...)
			return err
		}

		if pullRequestNumber != 0 {
			_, err := gather.PullRequest(logger, githubClient, owner, repo, pullRequestNumber, opts...)
			return err
		}

		log.Info().
			Str("owner", owner).
			Str("repo", repo).
			Int64("workflow_run_id", workflowRunID).
			Int("pull_request_number", pullRequestNumber).
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered data from GitHub")
		return nil
	},
}

func init() {
	gatherCmd.Flags().BoolVarP(&forceUpdate, "force-update", "u", false, "Force update of existing data")

	rootCmd.AddCommand(gatherCmd)
}
