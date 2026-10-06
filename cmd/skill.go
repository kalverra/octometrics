package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var skillContent string

// SetSkillContent configures the Markdown content for the skill command.
func SetSkillContent(content string) {
	skillContent = content
}

var skillCmd = &cobra.Command{
	Use:     "skill",
	Aliases: []string{"instructions", "agent"},
	Short:   "Output agent instructions and skill guide (Markdown)",
	Long: `Output full agent instructions and operational guidelines in Markdown format.

Designed for AI agents and LLM-assisted workflows to understand how to use octometrics,
supported input formats, triage procedures, and optimization workflows.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprint(cmd.OutOrStdout(), skillContent)
		return err
	},
}

func init() {
	rootCmd.AddCommand(skillCmd)
}
