package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kalverra/octometrics/audit"
	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/githuburl"
	"github.com/kalverra/octometrics/observe"
)

var (
	auditFlagOwner        string
	auditFlagRepo         string
	auditFlagWorkflow     string
	auditFlagRuns         int
	auditFlagBranch       string
	auditFlagStatus       string
	auditFlagFormat       string
	auditFlagJSON         bool
	auditFlagAIOutput     bool
	auditFlagOutput       string
	auditFlagNoOpen       bool
	auditFlagNoObserve    bool
	auditFlagPort         int
	auditFlagCPUThreshold float64
	auditFlagRAMThreshold float64
	auditFlagIncludeRuns  []string
	auditFlagExcludeRuns  []string
)

var auditCmd = &cobra.Command{
	Use:     "audit [workflow-url|workflow-name] [runs-count]",
	Aliases: []string{"workflow", "wf"},
	Short:   "Audit a GitHub Actions workflow for performance, runner sizing, and cost optimization",
	Long: `Audit recent runs of a GitHub Actions workflow to sample performance and costs,
extract CPU and Memory utilization from RunsOn logs and octometrics-action, and generate
actionable runner rightsizing and cost optimization recommendations.`,
	Example: `
  # Audit by workflow URL (opens interactive HTML report by default)
  octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml

  # Specify number of runs as positional argument
  octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml 25

  # Terminal-first Markdown mode for AI agents and scripts
  octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml --ai-output

  # Include or exclude specific runs
  octometrics audit https://.../ci.yaml --include-runs 37497090385,37496482422
  octometrics audit https://.../ci.yaml --exclude-runs 37509754611

  # Structured JSON output for agents and automation
  octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml --json

  # Audit by workflow filename with explicit owner and repo
  octometrics audit ci.yaml 20 -o owner -r repo --branch main

  # Save report directly to file
  octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml --output audit.md
`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		if len(args) > 0 {
			if n, err := strconv.Atoi(args[0]); err == nil && n > 0 && !cmd.Flags().Changed("runs") &&
				(auditFlagWorkflow != "" || len(auditFlagIncludeRuns) > 0) {
				auditFlagRuns = n
			} else {
				target = args[0]
			}
		}
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid runs count %q: must be a positive integer", args[1])
			}
			auditFlagRuns = n
		}
		return runAuditFlow(cmd, target)
	},
}

func runAuditFlow(cmd *cobra.Command, target string) error {
	ctx := context.Background()
	if cmd != nil {
		ctx = cmd.Context()
	}

	includeIDs, incOwner, incRepo, err := parseRunIDs(auditFlagIncludeRuns)
	if err != nil {
		return fmt.Errorf("invalid --include-runs: %w", err)
	}
	excludeIDs, _, _, err := parseRunIDs(auditFlagExcludeRuns)
	if err != nil {
		return fmt.Errorf("invalid --exclude-runs: %w", err)
	}

	if auditFlagOwner == "" && incOwner != "" {
		auditFlagOwner = incOwner
	}
	if auditFlagRepo == "" && incRepo != "" {
		auditFlagRepo = incRepo
	}

	owner, repo, workflow, err := resolveAuditTarget(target, len(includeIDs) > 0)
	if err != nil {
		return err
	}

	var client *gather.GitHubClient
	if cfg != nil && cfg.GitHubToken != "" {
		var initErr error
		client, initErr = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
		if initErr != nil {
			logger.Warn().Err(initErr).Msg("failed to initialize GitHub client, falling back to local cache")
		}
	}

	dataDir := ""
	if cfg != nil {
		dataDir = cfg.DataDir
	}

	format := determineAuditFormat(cmd)
	opts := audit.Options{
		Owner:               owner,
		Repo:                repo,
		Workflow:            workflow,
		RunsCount:           auditFlagRuns,
		IncludeRuns:         includeIDs,
		ExcludeRuns:         excludeIDs,
		Branch:              auditFlagBranch,
		Status:              auditFlagStatus,
		DataDir:             dataDir,
		AIOutput:            auditFlagAIOutput,
		Format:              format,
		CPUThresholdPercent: auditFlagCPUThreshold,
		RAMThresholdPercent: auditFlagRAMThreshold,
	}

	auditReport, err := audit.RunAudit(ctx, logger, client, opts)
	if err != nil {
		return fmt.Errorf("audit workflow: %w", err)
	}

	return renderAuditOutput(ctx, auditReport, format, owner, repo, workflow, dataDir)
}

func parseRunIDs(items []string) ([]int64, string, string, error) {
	var (
		runIDs []int64
		owner  string
		repo   string
	)
	for _, item := range items {
		for part := range strings.SplitSeq(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.HasPrefix(part, "http://") || strings.HasPrefix(part, "https://") {
				res, err := githuburl.Parse(part)
				if err != nil {
					return nil, "", "", fmt.Errorf("invalid run URL %q: %w", part, err)
				}
				if res.Owner != "" && owner == "" {
					owner = res.Owner
				}
				if res.Repo != "" && repo == "" {
					repo = res.Repo
				}
				if res.WorkflowRunID != 0 {
					runIDs = append(runIDs, res.WorkflowRunID)
					continue
				}
				return nil, "", "", fmt.Errorf("URL %q does not contain a workflow run ID", part)
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil, "", "", fmt.Errorf("invalid run ID %q: expected integer or GitHub run URL", part)
			}
			runIDs = append(runIDs, id)
		}
	}
	return runIDs, owner, repo, nil
}

func resolveAuditTarget(target string, hasIncludeRuns bool) (string, string, string, error) {
	owner := auditFlagOwner
	repo := auditFlagRepo
	workflow := auditFlagWorkflow

	if owner == "" && cfg != nil {
		owner = cfg.Owner
	}
	if repo == "" && cfg != nil {
		repo = cfg.Repo
	}

	if target != "" {
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			res, err := githuburl.Parse(target)
			if err != nil {
				return "", "", "", err
			}
			if res.Owner != "" {
				owner = res.Owner
			}
			if res.Repo != "" {
				repo = res.Repo
			}
			if res.WorkflowFileName != "" {
				workflow = res.WorkflowFileName
			} else if res.WorkflowID != 0 {
				workflow = strconv.FormatInt(res.WorkflowID, 10)
			}
		} else if workflow == "" {
			workflow = target
		}
	}

	if owner == "" {
		return "", "", "", errors.New("repository owner is required (specify via URL, -o/--owner, or config)")
	}
	if repo == "" {
		return "", "", "", errors.New("repository name is required (specify via URL, -r/--repo, or config)")
	}
	if workflow == "" && !hasIncludeRuns {
		return "", "", "", errors.New(
			"workflow file name or ID is required (specify as positional argument, URL, or --workflow)",
		)
	}

	return owner, repo, workflow, nil
}

func determineAuditFormat(cmd *cobra.Command) string {
	isExplicitFormat := cmd != nil && cmd.Flags().Changed("format")
	switch {
	case auditFlagJSON:
		return "json"
	case auditFlagAIOutput && !isExplicitFormat:
		return "md"
	case !term.IsTerminal(int(os.Stdout.Fd())) && !isExplicitFormat:
		return "md"
	default:
		return strings.ToLower(auditFlagFormat)
	}
}

func renderAuditOutput(
	ctx context.Context,
	auditReport *audit.WorkflowAudit,
	format, owner, repo, workflow, dataDir string,
) error {
	switch format {
	case "json":
		data, err := audit.FormatJSON(auditReport)
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}
		return outputBytesOrStdout(data, auditFlagOutput)
	case "md", "markdown":
		text := audit.FormatMarkdown(auditReport)
		return outputBytesOrStdout([]byte(text), auditFlagOutput)
	case "table":
		text := audit.FormatTable(auditReport)
		return outputBytesOrStdout([]byte(text), auditFlagOutput)
	default:
		return renderHTMLAudit(ctx, auditReport, owner, repo, workflow, dataDir)
	}
}

func outputBytesOrStdout(data []byte, destFile string) error {
	if destFile != "" {
		if err := os.WriteFile(destFile, data, 0o600); err != nil {
			return fmt.Errorf("write output file %q: %w", destFile, err)
		}
		return nil
	}
	fmt.Print(string(data))
	return nil
}

func renderHTMLAudit(
	ctx context.Context,
	auditReport *audit.WorkflowAudit,
	owner, repo, workflow, dataDir string,
) error {
	htmlContent, htmlErr := audit.FormatHTML(auditReport)
	if htmlErr != nil {
		return fmt.Errorf("format HTML: %w", htmlErr)
	}

	if auditFlagOutput != "" {
		if writeErr := os.WriteFile(auditFlagOutput, []byte(htmlContent), 0o600); writeErr != nil {
			return fmt.Errorf("write output file %q: %w", auditFlagOutput, writeErr)
		}
		fmt.Fprintf(os.Stderr, "Wrote HTML audit report to %s\n", auditFlagOutput)
		return nil
	}

	cleanName := strings.TrimSuffix(filepath.Base(workflow), filepath.Ext(workflow))
	htmlOutputDir := filepath.Join("observe_output", "html")
	targetFilePath := filepath.Join(htmlOutputDir, owner, repo, "workflows", cleanName+".html")
	if mkErr := os.MkdirAll(filepath.Dir(targetFilePath), 0o750); mkErr != nil {
		return fmt.Errorf("create html output directory: %w", mkErr)
	}
	if writeErr := os.WriteFile(targetFilePath, []byte(htmlContent), 0o600); writeErr != nil {
		return fmt.Errorf("write audit html file: %w", writeErr)
	}

	_ = observe.WriteStaticAssets(htmlOutputDir)

	if auditFlagAIOutput || auditFlagNoObserve {
		fmt.Printf("HTML report generated at %s\n", targetFilePath)
		return nil
	}

	initialPath := fmt.Sprintf("/%s/%s/workflows/%s.html", owner, repo, cleanName)
	obsOpts := []observe.Option{}
	if auditFlagNoOpen {
		obsOpts = append(obsOpts, observe.WithNoOpen(true))
	}
	if auditFlagPort > 0 {
		obsOpts = append(obsOpts, observe.WithPort(auditFlagPort))
	}

	fmt.Fprintf(os.Stderr, "Audit complete. Opening %s\n", targetFilePath)
	handler := observe.NewOnDemandHandler(logger, nil, dataDir, htmlOutputDir)
	return observe.ServeHTMLWithHandler(ctx, logger, initialPath, handler, obsOpts...)
}

func init() {
	rootCmd.AddCommand(auditCmd)

	auditCmd.Flags().StringVar(&auditFlagOwner, "owner", "", "GitHub repository owner")
	auditCmd.Flags().StringVarP(&auditFlagRepo, "repo", "r", "", "GitHub repository name")
	auditCmd.Flags().StringVarP(&auditFlagWorkflow, "workflow", "w", "", "Workflow file name or numeric ID")
	auditCmd.Flags().IntVarP(&auditFlagRuns, "runs", "n", 10, "Number of recent runs to sample")
	auditCmd.Flags().IntVar(&auditFlagRuns, "limit", 10, "Number of recent runs to sample (alias for --runs)")
	auditCmd.Flags().
		StringSliceVar(&auditFlagIncludeRuns, "include-runs", nil, "Specific workflow run IDs or URLs to include in audit (comma-separated or repeated)")
	auditCmd.Flags().
		StringSliceVar(&auditFlagExcludeRuns, "exclude-runs", nil, "Specific workflow run IDs or URLs to exclude from audit (comma-separated or repeated)")
	auditCmd.Flags().StringSliceVar(&auditFlagIncludeRuns, "include-run", nil, "Alias for --include-runs")
	auditCmd.Flags().StringSliceVar(&auditFlagExcludeRuns, "exclude-run", nil, "Alias for --exclude-runs")
	_ = auditCmd.Flags().MarkHidden("include-run")
	_ = auditCmd.Flags().MarkHidden("exclude-run")
	_ = auditCmd.Flags().MarkHidden("limit")
	auditCmd.Flags().StringVarP(&auditFlagBranch, "branch", "b", "", "Filter runs by head branch")
	auditCmd.Flags().
		StringVar(&auditFlagStatus, "status", "completed", "Filter runs by status/conclusion (completed, success, failure, all)")
	auditCmd.Flags().StringVarP(&auditFlagFormat, "format", "f", "html", "Output format: html, md, table, json")
	auditCmd.Flags().BoolVar(&auditFlagJSON, "json", false, "Output report as JSON to stdout")
	auditCmd.Flags().
		BoolVar(&auditFlagAIOutput, "ai-output", false, "AI agent mode: print Markdown/JSON to stdout without web server or browser")
	auditCmd.Flags().StringVarP(&auditFlagOutput, "output", "o", "", "Write report output to specified file path")
	auditCmd.Flags().BoolVar(&auditFlagNoOpen, "no-open", false, "Do not open browser automatically for HTML report")
	auditCmd.Flags().BoolVar(&auditFlagNoObserve, "no-observe", false, "Do not start local web server")
	auditCmd.Flags().IntVar(&auditFlagPort, "port", 8080, "Port for local web server")
	auditCmd.Flags().
		Float64Var(&auditFlagCPUThreshold, "threshold-cpu", 30.0, "CPU utilization percentage threshold for downgrade recommendation")
	auditCmd.Flags().
		Float64Var(&auditFlagRAMThreshold, "threshold-mem", 50.0, "Memory utilization percentage threshold for downgrade recommendation")
}
