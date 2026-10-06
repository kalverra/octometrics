package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/cost"
	"github.com/kalverra/octometrics/gather"
)

var costReportOpts cost.Options

var costCmd = &cobra.Command{
	Use:    "cost",
	Short:  "Analyze GitHub Actions usage, costs, and runner savings (experimental)",
	Hidden: true,
	Long: `Analyze GitHub Actions billing usage reports, break down spending by runner type,
repository, and workflow, and project migration savings to Runs-On.

NOTE: This feature is rusty and experimental. It was developed for ad hoc
comparisons using downloaded CSV files for cost reporting.`,
}

var costReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate cost and runner breakdown report from usage CSV (experimental)",
	Long: `Generate cost and runner breakdown report from usage CSV.

NOTE: This feature is rusty and experimental. It was developed for ad hoc
comparisons using downloaded CSV files for cost reporting.`,
	Example: `
# Generate terminal table report from default or specified CSV
octometrics cost report --csv gha-usage-report.csv

# Filter by organization and output as Markdown
octometrics cost report --csv gha-usage-report.csv --org smartcontractkit --format markdown -o report.md

# Output as JSON
octometrics cost report --csv gha-usage-report.csv --format json
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger.Warn().Msg("cost reporting is rusty and experimental")
		fmt.Fprintln(os.Stderr, "WARNING: The cost reporting feature is rusty and experimental.")

		var client *gather.GitHubClient
		if cfg != nil && cfg.GitHubToken != "" {
			var err error
			client, err = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
			if err != nil {
				logger.Warn().
					Err(err).
					Msg("GitHub client initialization failed, proceeding with local cache / heuristic resolution")
			}
		}

		dataDir := ""
		if cfg != nil {
			dataDir = cfg.DataDir
		}
		costReportOpts.DataDir = dataDir

		return cost.RunReport(cmd.Context(), logger, client, costReportOpts)
	},
}

func init() {
	rootCmd.AddCommand(costCmd)
	costCmd.AddCommand(costReportCmd)

	costReportCmd.Flags().StringVarP(
		&costReportOpts.CSVPath, "csv", "c", "gha-usage-report.csv",
		"Path to GitHub Actions billing usage report CSV",
	)
	costReportCmd.Flags().StringVar(
		&costReportOpts.OrgFilter, "org", "",
		"Filter by GitHub organization (e.g. smartcontractkit)",
	)
	costReportCmd.Flags().StringVarP(
		&costReportOpts.Format, "format", "f", "table",
		"Report output format: table, markdown, json",
	)
	costReportCmd.Flags().StringVarP(
		&costReportOpts.CloudzeroCSVPath, "runs-on-csv", "r", "",
		"Path to Cloudzero Runs-On cost export CSV (defaults to runs-on-costs.csv if present)",
	)
	costReportCmd.Flags().StringVarP(
		&costReportOpts.OutputFile, "output", "o", "",
		"Write report output to specified file path",
	)
}
