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
		return mcp.Server(logger, githubClient)
	},
}

func init() {
	mcpCmd.Flags().StringP("github-token", "t", "", "GitHub API token (env: GITHUB_TOKEN)")
	rootCmd.AddCommand(mcpCmd)
}
