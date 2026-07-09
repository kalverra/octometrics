package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the Octometrics MCP server (Model Context Protocol) via stdio",
	Long: `Start the Octometrics MCP server over stdio.

Exposes tools for workflow summaries, job timelines, performance metrics, run comparison, and workflow listing.`,
	Example: `
octometrics mcp -t $GITHUB_TOKEN
`,
	PreRunE: func(_ *cobra.Command, _ []string) error {
		if cfg.GitHubToken == "" {
			return fmt.Errorf("github-token is required for the MCP server")
		}
		var err error
		githubClient, err = gather.NewGitHubClient(logger, cfg.GitHubToken, nil)
		if err != nil {
			return fmt.Errorf("failed to create GitHub client: %w", err)
		}
		return nil
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		return mcp.Server(logger, githubClient, &mcp.DefaultObserver{}, version)
	},
}

func init() {
	mcpCmd.Flags().StringP("github-token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	rootCmd.AddCommand(mcpCmd)
}
