package cmd

import (
	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/monitor"
	"github.com/kalverra/octometrics/report"
)

var reportOpts report.Options

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a monitoring report for the current GitHub Actions job",
	Long: `Analyze monitor data, fetch job step timing from the GitHub API, and produce
a Mermaid-based report posted to GITHUB_STEP_SUMMARY and as a PR comment.

Typically invoked by the octometrics-action post step after the monitor process
has been stopped.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return report.Run(logger, reportOpts)
	},
}

func init() {
	rootCmd.AddCommand(reportCmd)

	reportCmd.Flags().StringVarP(
		&reportOpts.MonitorFile, "monitor-file", "f", monitor.DataFile,
		"Path to the monitor JSONL data file",
	)
	reportCmd.Flags().BoolVar(
		&reportOpts.SkipSummary, "skip-summary", false,
		"Skip writing to GITHUB_STEP_SUMMARY",
	)
	reportCmd.Flags().BoolVar(
		&reportOpts.SkipComment, "skip-comment", false,
		"Skip posting a PR or commit comment",
	)
}
