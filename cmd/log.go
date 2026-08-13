package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/githuburl"
)

var logCmd = &cobra.Command{
	Use:     "log [job-id|url]",
	Aliases: []string{"logs"},
	Short:   "Fetch and output clean, ANSI-stripped log lines for a specific job ID or GitHub job URL",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			jobID int64
			owner = cfg.Owner
			repo  = cfg.Repo
			err   error
		)

		urlFlag, _ := cmd.Flags().GetString("url")
		jobIDFlag, _ := cmd.Flags().GetInt64("job-id")

		target := ""
		if len(args) > 0 {
			target = args[0]
		} else if urlFlag != "" {
			target = urlFlag
		}

		if target != "" {
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				res, parseErr := githuburl.Parse(target)
				if parseErr != nil {
					return parseErr
				}
				owner = res.Owner
				repo = res.Repo
				if res.JobID != 0 {
					jobID = res.JobID
				}
			} else {
				parsedID, parseErr := strconv.ParseInt(target, 10, 64)
				if parseErr != nil {
					return fmt.Errorf("invalid job ID or URL %q: %w", target, parseErr)
				}
				jobID = parsedID
			}
		}

		if jobID == 0 && jobIDFlag > 0 {
			jobID = jobIDFlag
		}

		flagOwner, _ := cmd.Flags().GetString("owner")
		flagRepo, _ := cmd.Flags().GetString("repo")
		if flagOwner != "" {
			owner = flagOwner
		}
		if flagRepo != "" {
			repo = flagRepo
		}

		if jobID == 0 {
			return fmt.Errorf(
				"job ID or GitHub job URL is required (usage: octometrics log [job-id|url] or --job-id <id>)",
			)
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
			owner,
			repo,
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
	logCmd.Flags().Int64P("job-id", "j", 0, "Workflow job ID")
	logCmd.Flags().String("url", "", "Full GitHub job or workflow run URL")
	logCmd.Flags().IntP("gaps", "g", 0, "Show top N intra-step silent gaps with surrounding lines")
	rootCmd.AddCommand(logCmd)
}
