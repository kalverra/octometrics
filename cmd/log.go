package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
)

var logCmd = &cobra.Command{
	Use:     "log [job-id]",
	Aliases: []string{"logs"},
	Short:   "Fetch and output clean, ANSI-stripped log lines for a specific job ID",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid job ID %q: %w", args[0], err)
		}

		if cfg.GitHubToken != "" && githubClient == nil {
			var clientErr error
			githubClient, clientErr = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
			if clientErr != nil {
				return fmt.Errorf("failed to create GitHub client: %w", clientErr)
			}
		}

		cleanedLogs, err := gather.GetCleanJobLogs(
			cmd.Context(),
			logger,
			githubClient,
			cfg.Owner,
			cfg.Repo,
			jobID,
			cfg.DataDir,
		)
		if err != nil {
			return fmt.Errorf("failed to get job logs: %w", err)
		}

		gapsCount, _ := cmd.Flags().GetInt("gaps")
		if gapsCount > 0 {
			gaps := gather.ParseLogGaps(cleanedLogs, gapsCount)
			if len(gaps) > 0 {
				fmt.Printf("Top %d silent stretches (gaps) in job %d logs:\n", len(gaps), jobID)
				for i, g := range gaps {
					fmt.Printf("  %d. Gap of %s:\n     Before: %s\n     After:  %s\n",
						i+1, g.Duration.Round(10*time.Millisecond), g.LineBefore, g.LineAfter)
				}
				fmt.Println()
			}
		}

		fmt.Print(cleanedLogs)
		return nil
	},
}

func init() {
	logCmd.Flags().StringP("owner", "o", "", "Repository owner")
	logCmd.Flags().StringP("repo", "r", "", "Repository name")
	logCmd.Flags().IntP("gaps", "g", 0, "Show top N intra-step silent gaps with surrounding lines")
	rootCmd.AddCommand(logCmd)
}
