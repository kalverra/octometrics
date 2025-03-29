package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gopkg.in/natefinch/lumberjack.v2"
)

const logTimeFormat = "2006-01-02T15:04:05.000"

// These variables are set at build time and describe the version and build of the application
var (
	version   = "dev"
	commit    = "dev"
	buildTime = time.Now().Format(logTimeFormat)
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
	pullRequestID     int

	githubClient *github.Client
)

var rootCmd = &cobra.Command{
	Use:   "octometrics",
	Short: "", // TODO: Fill out
	Long:  ``, // TODO: Fill out
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if workflowRunID == 0 && pullRequestID == 0 {
			return fmt.Errorf("either workflow run ID or pull request ID must be provided")
		}
		if workflowRunID != 0 && pullRequestID != 0 {
			return fmt.Errorf("only one of workflow run ID or pull request ID must be provided")
		}

		err := setupLogging()
		if err != nil {
			return fmt.Errorf("failed to setup logging: %w", err)
		}
		githubClient, err = getGitHubClient()
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}

		log.Debug().
			Str("version", version).
			Str("commit", commit).
			Str("build_time", buildTime).
			Str("built_by", builtBy).
			Msg("Workflow Metrics Version Info")
		log.Debug().
			Str("owner", owner).
			Str("repo", repo).
			Int64("workflow_run_id", workflowRunID).
			Int("pull_request_id", pullRequestID).
			Str("log_file", logFileName).
			Str("log_level", logLevelInput).
			Bool("disable_console_log", disableConsoleLog).
			Msg("workflow-metrics flags")
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		err := cmd.Help()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to print help message")
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&logFileName, "log-file", "f", "workflow-metrics.log.json", "Log file name")
	rootCmd.PersistentFlags().StringVarP(&logLevelInput, "log-level", "l", "info", "Log level")
	rootCmd.PersistentFlags().BoolVarP(&disableConsoleLog, "silent", "s", false, "Disables console logs. Still logs to file")

	rootCmd.PersistentFlags().StringVarP(&owner, "owner", "o", "", "Repository owner")
	rootCmd.PersistentFlags().StringVarP(&repo, "repo", "r", "", "Repository name")
	rootCmd.PersistentFlags().Int64VarP(&workflowRunID, "workflow-run-id", "w", 0, "Workflow run ID")
	rootCmd.PersistentFlags().IntVarP(&pullRequestID, "pull-request-id", "p", 0, "Pull request ID")
	rootCmd.PersistentFlags().StringVarP(&githubToken, "github-token", "t", "", fmt.Sprintf("GitHub API token (can also be set via %s)", githubTokenEnvVar))

	err := rootCmd.MarkPersistentFlagRequired("owner")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to mark flag as required")
	}
	err = rootCmd.MarkPersistentFlagRequired("repo")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to mark flag as required")
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Failed to execute command")
		os.Exit(1)
	}
}

func getGitHubClient() (*github.Client, error) {
	if githubToken != "" {
		log.Debug().Msg("Using GitHub token from flag")
	} else if os.Getenv(githubTokenEnvVar) != "" {
		githubToken = os.Getenv(githubTokenEnvVar)
		log.Debug().Msg("Using GitHub token from environment variable")
	} else {
		log.Warn().Msg("GitHub token not provided, will likely hit rate limits quickly")
	}

	rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create rate limiter")
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
	log.Debug().Int("limit", rateLimit).Int("remaining", rateRemaining).Msg("GitHub rate limits")
	if rateLimit <= 60 {
		log.Warn().
			Int("limit", rateLimit).
			Int("remaining", rateRemaining).
			Msg("GitHub rate limit is low. You're either not providing a token, or your token isn't valid.")
	}
	return client, nil
}

func setupLogging() error {
	err := os.WriteFile(logFileName, []byte{}, 0644)
	if err != nil {
		return err
	}

	lumberLogger := &lumberjack.Logger{
		Filename:   logFileName,
		MaxSize:    100, // megabytes
		MaxBackups: 10,
		MaxAge:     30,
	}

	writers := []io.Writer{lumberLogger}
	if !disableConsoleLog {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: logTimeFormat})
	}

	logLevel, err := zerolog.ParseLevel(logLevelInput)
	if err != nil {
		return err
	}

	zerolog.TimeFieldFormat = logTimeFormat
	multiWriter := zerolog.MultiLevelWriter(writers...)
	log.Logger = zerolog.New(multiWriter).Level(logLevel).With().Timestamp().Logger()
	return nil
}
