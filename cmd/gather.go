package cmd

import (
	"errors"
	"time"

	"github.com/kalverra/octometrics/gather"
	"github.com/spf13/cobra"
)

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
		logger.Debug().
			Bool("force-update", forceUpdate).
			Msg("gather flags")

		startTime := time.Now()

		logger = logger.With().Str("owner", owner).Str("repo", repo).Logger()

		if workflowRunID != 0 {
			logger = logger.With().Int64("workflow_run_id", workflowRunID).Logger()
		} else if pullRequestNumber != 0 {
			logger = logger.With().Int("pull_request_number", pullRequestNumber).Logger()
		}

		logger.Info().Msg("Gathering data")

		opts := []gather.Option{}

		if forceUpdate {
			opts = append(opts, gather.ForceUpdate())
		}

		var err error
		if workflowRunID != 0 {
			_, err = gather.WorkflowRun(logger, githubClient, owner, repo, workflowRunID, opts...)
		} else if pullRequestNumber != 0 {
			_, err = gather.PullRequest(logger, githubClient, owner, repo, pullRequestNumber, opts...)
		}
		if err != nil {
			return err
		}

		logger.Info().Str("duration", time.Since(startTime).String()).Msg("Gathered data")
		return nil
	},
}

func init() {
	gatherCmd.Flags().BoolVarP(&forceUpdate, "force-update", "u", false, "Force update of existing data")

	rootCmd.AddCommand(gatherCmd)
}
