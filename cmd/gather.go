package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
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

		setCount := 0
		if commitSHA != "" {
			setCount++
		}
		if workflowRunID != 0 {
			setCount++
		}
		if pullRequestNumber != 0 {
			setCount++
		}
		if setCount > 1 {
			return errors.New("only one of commit SHA, workflow run ID or pull request number can be provided")
		}
		if setCount == 0 {
			return errors.New("one of commit SHA, workflow run ID or pull request number must be provided")
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
			_, _, err = gather.WorkflowRun(logger, githubClient, owner, repo, workflowRunID, opts...)
		} else if pullRequestNumber != 0 {
			_, err = gather.PullRequest(logger, githubClient, owner, repo, pullRequestNumber, opts...)
		} else if commitSHA != "" {
			_, err = gather.Commit(logger, githubClient, owner, repo, commitSHA, opts...)
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
	gatherCmd.Flags().StringVarP(&owner, "owner", "o", "", "Repository owner")
	gatherCmd.Flags().StringVarP(&repo, "repo", "r", "", "Repository name")
	gatherCmd.Flags().StringVarP(&commitSHA, "commit-sha", "c", "", "Commit SHA")
	gatherCmd.Flags().Int64VarP(&workflowRunID, "workflow-run-id", "w", 0, "Workflow run ID")
	gatherCmd.Flags().IntVarP(&pullRequestNumber, "pull-request-number", "p", 0, "Pull request number")
	gatherCmd.Flags().
		StringVarP(&githubToken, "github-token", "t", "", fmt.Sprintf("GitHub API token (can also be set via %s)", gather.GitHubTokenEnvVar))

	if err := gatherCmd.MarkFlagRequired("owner"); err != nil {
		fmt.Printf("ERROR: Failed to mark owner flag as required: %s\n", err.Error())
		os.Exit(1)
	}
	if err := gatherCmd.MarkFlagRequired("repo"); err != nil {
		fmt.Printf("ERROR: Failed to mark repo flag as required: %s\n", err.Error())
		os.Exit(1)
	}

	rootCmd.AddCommand(gatherCmd)
}
