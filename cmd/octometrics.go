package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/logging"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// These variables are set at build time and describe the version and build of the application
var (
	version   = "dev"
	commit    = "dev"
	buildTime = time.Now().Format("2006-01-02T15:04:05.000")
	builtBy   = "local"
)

// Persistent base command flags
var (
	logFileName       string
	logLevelInput     string
	disableConsoleLog bool
	owner             string
	repo              string
	workflowRunID     int64
	pullRequestNumber int

	githubClient *github.Client
	logger       zerolog.Logger
)

var rootCmd = &cobra.Command{
	Use:   "octometrics",
	Short: "See metrics and profiling of your GitHub Actions",
	Long: `See metrics and profiling of your GitHub Actions.

GitHub Actions provides surprisingly little metrics to help you optimize things like runtime and profiling data.
Octometrics aims to help you easily visualize what your workflows look like, helping you identify bottlenecks and inefficiencies in your CI/CD pipelines.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error

		loggingOpts := []logging.Option{
			logging.WithFileName(logFileName),
			logging.WithLevel(logLevelInput),
		}
		if disableConsoleLog {
			loggingOpts = append(loggingOpts, logging.DisableConsoleLog())
		}
		logger, err = logging.New(loggingOpts...)
		if err != nil {
			return fmt.Errorf("failed to setup logging: %w", err)
		}

		githubClient, err = getGitHubClient(logger)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}

		logger.Debug().
			Str("version", version).
			Str("commit", commit).
			Str("build_time", buildTime).
			Str("built_by", builtBy).
			Msg("Octometrics Version Info")
		logger.Debug().
			Str("owner", owner).
			Str("repo", repo).
			Int64("workflow_run_id", workflowRunID).
			Int("pull_request_number", pullRequestNumber).
			Str("log_file", logFileName).
			Str("log_level", logLevelInput).
			Bool("disable_console_log", disableConsoleLog).
			Msg("octometrcis flags")
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		err := cmd.Help()
		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to print help message")
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&logFileName, "log-file", "f", "octometrics.log.json", "Log file name")
	rootCmd.PersistentFlags().StringVarP(&logLevelInput, "log-level", "l", "info", "Log level")
	rootCmd.PersistentFlags().BoolVarP(&disableConsoleLog, "silent", "s", false, "Disables console logs. Still logs to file")

	rootCmd.PersistentFlags().StringVarP(&owner, "owner", "o", "", "Repository owner")
	rootCmd.PersistentFlags().StringVarP(&repo, "repo", "r", "", "Repository name")
	rootCmd.PersistentFlags().Int64VarP(&workflowRunID, "workflow-run-id", "w", 0, "Workflow run ID")
	rootCmd.PersistentFlags().IntVarP(&pullRequestNumber, "pull-request-number", "p", 0, "Pull request number")
	rootCmd.PersistentFlags().StringVarP(&githubToken, "github-token", "t", "", fmt.Sprintf("GitHub API token (can also be set via %s)", githubTokenEnvVar))
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to execute command")
		os.Exit(1)
	}
}

func getGitHubClient(logger zerolog.Logger) (*github.Client, error) {
	if githubToken != "" {
		logger.Debug().Msg("Using GitHub token from flag")
	} else if os.Getenv(githubTokenEnvVar) != "" {
		githubToken = os.Getenv(githubTokenEnvVar)
		logger.Debug().Msg("Using GitHub token from environment variable")
	} else {
		logger.Warn().Msg("GitHub token not provided, will likely hit rate limits quickly")
	}

	rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(logging.GitHubClientRoundTripper(logger))
	if err != nil {
		return nil, err
	}
	client := github.NewClient(rateLimiter)
	if githubToken != "" {
		client = client.WithAuthToken(githubToken)
	}
	limits, _, err := client.RateLimit.Get(context.Background())
	if err != nil {
		return nil, err
	}
	rateLimit := limits.GetCore().Limit
	rateRemaining := limits.GetCore().Remaining
	logger.Debug().Int("limit", rateLimit).Int("remaining", rateRemaining).Msg("GitHub rate limits")
	if rateLimit <= 60 {
		logger.Warn().
			Int("limit", rateLimit).
			Int("remaining", rateRemaining).
			Msg("GitHub rate limit is low. You're either not providing a token, or your token isn't valid.")
	}
	return client, nil
}
