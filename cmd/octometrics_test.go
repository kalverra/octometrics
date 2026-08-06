package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCmdFlags(t *testing.T) {
	t.Parallel()

	flags := []string{
		"owner",
		"repo",
		"commit-sha",
		"workflow-run-id",
		"pull-request-number",
		"from",
		"to",
		"event",
		"github-token",
		"force-update",
		"no-observe",
		"exclude-costs",
		"exclude-workflows",
		"include-workflows",
		"format",
		"stdout",
		"rebuild-manifest",
	}

	for _, flagName := range flags {
		f := rootCmd.Flags().Lookup(flagName)
		assert.NotNil(t, f, "rootCmd should have flag --%s", flagName)
	}
}
