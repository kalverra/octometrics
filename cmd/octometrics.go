package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/charmbracelet/fang"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/config"
	"github.com/kalverra/octometrics/logging"
)

const (
	logFileName = "octometrics.log.json"
)

// Persistent base command flags
var (
	githubClient *gather.GitHubClient
	logger       zerolog.Logger
)

// These variables are set at build time and describe the version and build of the application
var (
	version   = "dev"
	commit    = "dev"
	buildTime = time.Now().Format("2006-01-02T15:04:05.000")
	builtBy   = "local"
	builtWith = runtime.Version()
)

func versionInfo() string {
	return fmt.Sprintf(
		"octometrics version %s built with %s from commit %s at %s by %s",
		version,
		builtWith,
		commit,
		buildTime,
		builtBy,
	)
}

// Flag values shared between other commands
var (
	owner             string
	repo              string
	commitSHA         string
	workflowRunID     int64
	pullRequestNumber int
)

var rootCmd = &cobra.Command{
	Use:   "octometrics",
	Short: "See metrics and profiling of your GitHub Actions",
	Long: `See metrics and profiling of your GitHub Actions.

GitHub Actions provides surprisingly little metrics to help you optimize things like runtime and profiling data.
Octometrics aims to help you easily visualize what your workflows look like, helping you identify bottlenecks and inefficiencies in your CI/CD pipelines.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		var err error

		cfg, err := config.Load(config.WithFlags(cmd.Flags()))
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		logger, err = logging.New(logging.WithFileName(logFileName), logging.WithLevel(cfg.LogLevel))
		if err != nil {
			return fmt.Errorf("failed to setup logging: %w", err)
		}

		githubClient, err = gather.NewGitHubClient(logger, githubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, _ []string) {
		err := cmd.Help()
		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to print help message")
		}
	},
}

func init() {
	rootCmd.PersistentFlags().String("log-level", config.DefaultLogLevel, "Log level")
}

// Execute runs the root command for octometrics.
func Execute() {
	if err := fang.Execute(context.Background(), rootCmd, fang.WithVersion(versionInfo())); err != nil {
		os.Exit(1)
	}
}
