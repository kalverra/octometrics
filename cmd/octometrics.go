// Package cmd implements the CLI commands for octometrics.
package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/charmbracelet/fang"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/config"
	"github.com/kalverra/octometrics/internal/githuburl"
	"github.com/kalverra/octometrics/internal/logging"
	"github.com/kalverra/octometrics/observe"
)

const (
	logFileName = "octometrics.log.jsonl"
)

var (
	cfg          *config.Config
	logger       zerolog.Logger
	cpuFile      *os.File
	githubClient *gather.GitHubClient
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

func commandNeedsGitHubToken(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "monitor", "report", "help", "completion":
			return false
		}
	}
	return true
}

func buildObserveOptions(cfg *config.Config) []observe.Option {
	gatherOpts := []gather.Option{
		gather.CustomDataFolder(cfg.DataDir),
	}
	if cfg.ForceUpdate {
		gatherOpts = append(gatherOpts, gather.ForceUpdate())
	}
	if !cfg.ExcludeCosts {
		gatherOpts = append(gatherOpts, gather.WithCost())
	} else {
		gatherOpts = append(gatherOpts, gather.WithoutCost())
	}

	return []observe.Option{
		observe.WithGatherOptions(gatherOpts...),
		observe.ExcludeWorkflows(cfg.ExcludeWorkflows),
		observe.IncludeWorkflows(cfg.IncludeWorkflows),
	}
}

var rootCmd = &cobra.Command{
	Use:   "octometrics [url]",
	Args:  cobra.MaximumNArgs(1),
	Short: "See metrics and profiling of your GitHub Actions",
	Long: `See metrics and profiling of your GitHub Actions.

GitHub Actions provides surprisingly little metrics to help you optimize things like runtime and profiling data.
Octometrics aims to help you easily visualize what your workflows look like, helping you identify bottlenecks and inefficiencies in your CI/CD pipelines.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		var err error

		cfg, err = config.Load(config.WithFlags(cmd.Flags()))
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		logger, err = logging.New(logging.WithFileName(logFileName), logging.WithLevel(cfg.LogLevel))
		if err != nil {
			return fmt.Errorf("failed to setup logging: %w", err)
		}

		if cfg.CPUProfile != "" {
			f, err := os.Create(cfg.CPUProfile)
			if err != nil {
				return fmt.Errorf("could not create CPU profile: %w", err)
			}
			cpuFile = f
			if err := pprof.StartCPUProfile(f); err != nil {
				_ = f.Close()
				return fmt.Errorf("could not start CPU profile: %w", err)
			}
		}

		if cfg.GitHubToken == "" && commandNeedsGitHubToken(cmd) {
			logger.Warn().Msg("GitHub token not provided, will likely hit rate limits quickly")
			fmt.Fprintln(os.Stderr, "WARNING: GitHub token not provided, will likely hit rate limits quickly")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		rebuild, _ := cmd.Flags().GetBool("rebuild-manifest")
		if rebuild {
			if err := observe.RebuildManifest(cmd.Context(), logger, cfg.DataDir); err != nil {
				return fmt.Errorf("failed to rebuild manifest: %w", err)
			}
			logger.Info().Msg("Manifest rebuilt successfully")
		}

		format, toStdout, err := determineFormat(cmd)
		if err != nil {
			return err
		}

		if len(args) > 0 {
			res, err := githuburl.Parse(args[0])
			if err != nil {
				return err
			}
			cfg.Owner = res.Owner
			cfg.Repo = res.Repo
			if res.WorkflowRunID != 0 {
				cfg.WorkflowRunID = res.WorkflowRunID
			}
			if res.CommitSHA != "" {
				cfg.CommitSHA = res.CommitSHA
			}
			if res.PullRequestNumber != 0 {
				cfg.PullRequestNumber = res.PullRequestNumber
			}
		}

		hasTarget := cfg.WorkflowRunID != 0 || cfg.PullRequestNumber != 0 || cfg.CommitSHA != "" ||
			(!cfg.From.IsZero() && !cfg.To.IsZero())

		if cfg.GitHubToken != "" {
			var clientErr error
			githubClient, clientErr = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
			if clientErr != nil {
				return fmt.Errorf("failed to create GitHub client: %w", clientErr)
			}
		}

		if hasTarget {
			if err := cfg.ValidateGather(); err != nil {
				return err
			}

			ctx := cmd.Context()
			opts := []gather.Option{
				gather.CustomDataFolder(cfg.DataDir),
			}
			if cfg.ForceUpdate {
				opts = append(opts, gather.ForceUpdate())
			}
			if !cfg.ExcludeCosts {
				opts = append(opts, gather.WithCost())
			} else {
				opts = append(opts, gather.WithoutCost())
			}

			var rangeFailures int
			if cfg.WorkflowRunID != 0 {
				_, _, err = gather.WorkflowRun(
					ctx,
					logger,
					githubClient,
					cfg.Owner,
					cfg.Repo,
					cfg.WorkflowRunID,
					opts...)
			} else if cfg.PullRequestNumber != 0 {
				_, err = gather.PullRequest(
					ctx,
					logger,
					githubClient,
					cfg.Owner,
					cfg.Repo,
					cfg.PullRequestNumber,
					opts...)
			} else if cfg.CommitSHA != "" {
				_, err = gather.Commit(ctx, logger, githubClient, cfg.Owner, cfg.Repo, cfg.CommitSHA, opts...)
			} else if !cfg.From.IsZero() && !cfg.To.IsZero() {
				rangeFailures, err = gather.Range(
					ctx,
					logger,
					githubClient,
					cfg.Owner,
					cfg.Repo,
					cfg.From,
					cfg.To,
					cfg.Event,
					opts...)
			}

			if err != nil {
				return err
			}
			if rangeFailures > 0 {
				return fmt.Errorf("%d workflow run(s) failed to gather", rangeFailures)
			}
		}

		if cfg.NoObserve {
			return nil
		}

		obsOpts := buildObserveOptions(cfg)

		if toStdout || format == "md" {
			var obs *observe.Observation
			if cfg.WorkflowRunID != 0 {
				obs, err = observe.WorkflowRun(
					cmd.Context(),
					logger,
					githubClient,
					cfg.Owner,
					cfg.Repo,
					cfg.WorkflowRunID,
					obsOpts...)
			} else if cfg.PullRequestNumber != 0 {
				obs, err = observe.PullRequest(
					cmd.Context(),
					logger,
					githubClient,
					cfg.Owner,
					cfg.Repo,
					cfg.PullRequestNumber,
					obsOpts...)
			} else if cfg.CommitSHA != "" {
				obs, err = observe.Commit(
					cmd.Context(),
					logger,
					githubClient,
					cfg.Owner,
					cfg.Repo,
					cfg.CommitSHA,
					obsOpts...)
			}

			if err != nil {
				return err
			}

			if obs != nil {
				outStr, err := obs.RenderString(logger, format)
				if err != nil {
					return fmt.Errorf("failed to render observation: %w", err)
				}
				fmt.Print(outStr)
				return nil
			}

			return observe.All(cmd.Context(), logger, githubClient, []string{format}, cfg.DataDir, obsOpts...)
		}

		var pagePath string
		if cfg.WorkflowRunID != 0 {
			pagePath = fmt.Sprintf("/%s/%s/workflow_runs/%d.html", cfg.Owner, cfg.Repo, cfg.WorkflowRunID)
		} else if cfg.PullRequestNumber != 0 {
			pagePath = fmt.Sprintf("/%s/%s/pull_requests/%d.html", cfg.Owner, cfg.Repo, cfg.PullRequestNumber)
		} else if cfg.CommitSHA != "" {
			pagePath = fmt.Sprintf("/%s/%s/commits/%s.html", cfg.Owner, cfg.Repo, cfg.CommitSHA)
		} else if cfg.Owner != "" && cfg.Repo != "" {
			pagePath = fmt.Sprintf("/%s/%s", cfg.Owner, cfg.Repo)
		}

		return observe.Interactive(cmd.Context(), logger, githubClient, pagePath, cfg.DataDir, obsOpts...)
	},
	PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			if err := cpuFile.Close(); err != nil {
				return fmt.Errorf("failed to close CPU profile file: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().String("log-level", config.DefaultLogLevel, "Level for detailed logging")
	rootCmd.PersistentFlags().
		String("data-dir", config.DefaultDataDir(), "Directory for cached GitHub data (env: DATA_DIR)")
	rootCmd.PersistentFlags().String("cpu-profile", "", "Write CPU profile to file")

	rootCmd.Flags().StringP("owner", "o", "", "Repository owner")
	rootCmd.Flags().StringP("repo", "r", "", "Repository name")
	rootCmd.Flags().StringP("commit-sha", "c", "", "Commit SHA")
	rootCmd.Flags().Int64P("workflow-run-id", "w", 0, "Workflow run ID")
	rootCmd.Flags().IntP("pull-request-number", "p", 0, "Pull request number")
	rootCmd.Flags().
		Time("from", time.Time{}, []string{"2006-01-02", "2006-01-02T15:04:05Z"}, "Start date for gathering data (YYYY-MM-DD)")
	rootCmd.Flags().
		Time("to", time.Time{}, []string{"2006-01-02", "2006-01-02T15:04:05Z"}, "End date for gathering data (YYYY-MM-DD)")
	rootCmd.Flags().String("event", "all", "Filter gathered data by event type (all, pull_request, merge_group, push)")
	rootCmd.Flags().StringP("github-token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	rootCmd.Flags().BoolP("force-update", "u", false, "Force update of existing data")
	rootCmd.Flags().Bool("no-observe", false, "Skip launching the interactive observer after gathering data")
	rootCmd.Flags().Bool("exclude-costs", false, "Skip gathering cost data for workflow runs")
	rootCmd.Flags().StringSlice("exclude-workflows", nil, "Omit workflow display names from observations")
	rootCmd.Flags().StringSlice("include-workflows", nil, "Include only specific workflow display names")
	rootCmd.Flags().String("format", "html", "Output format: html or md")
	rootCmd.Flags().Bool("stdout", false, "Output raw result to stdout without starting web server")
	rootCmd.Flags().Bool("rebuild-manifest", false, "Rebuild manifest.jsonl files from local data directory")
}

// Execute runs the root command for octometrics.
func Execute() {
	if err := fang.Execute(context.Background(), rootCmd, fang.WithVersion(versionInfo())); err != nil {
		os.Exit(1)
	}
}

func determineFormat(cmd *cobra.Command) (format string, toStdout bool, err error) {
	fmtFlag, _ := cmd.Flags().GetString("format")
	stdoutFlag, _ := cmd.Flags().GetBool("stdout")

	if fmtFlag != "html" && fmtFlag != "md" && fmtFlag != "markdown" {
		return "", false, fmt.Errorf("invalid format %q: must be 'html' or 'md'", fmtFlag)
	}
	if fmtFlag == "markdown" {
		fmtFlag = "md"
	}

	nonInteractive := !term.IsTerminal(int(os.Stdout.Fd()))
	isExplicitFormat := cmd.Flags().Changed("format")

	if stdoutFlag || fmtFlag == "md" || (nonInteractive && !isExplicitFormat) {
		if !isExplicitFormat && (stdoutFlag || nonInteractive) {
			fmtFlag = "md"
		}
		toStdout = true
	}

	return fmtFlag, toStdout, nil
}
