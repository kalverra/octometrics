package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		"output-file",
		"stdout",
		"rebuild-manifest",
	}

	for _, flagName := range flags {
		f := rootCmd.Flags().Lookup(flagName)
		assert.NotNil(t, f, "rootCmd should have flag --%s", flagName)
	}

	assert.NotNil(t, rootCmd.Flags().Lookup("vs"), "rootCmd should have flag --vs")
}

func TestLogCmdFlags(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, logCmd.Flags().Lookup("owner"), "logCmd should have flag --owner")
	assert.NotNil(t, logCmd.Flags().Lookup("repo"), "logCmd should have flag --repo")
	assert.NotNil(t, logCmd.Flags().Lookup("job-id"), "logCmd should have flag --job-id")
	assert.NotNil(t, logCmd.Flags().Lookup("url"), "logCmd should have flag --url")
	assert.NotNil(t, logCmd.Flags().Lookup("gaps"), "logCmd should have flag --gaps")
}

func TestRootCmdURLArgs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "octometrics [url]", rootCmd.Use)

	t.Run("invalid URL returns error", func(t *testing.T) {
		err := rootCmd.RunE(rootCmd, []string{"https://invalid-domain.com/owner/repo/pull/1"})
		assert.ErrorContains(t, err, "unsupported GitHub URL host")
	})
}

func TestVsTargetParsing(t *testing.T) {
	t.Parallel()

	runID, sha, err := parseVsTarget("30840863008")
	require.NoError(t, err)
	assert.Equal(t, int64(30840863008), runID)
	assert.Empty(t, sha)

	runID, sha, err = parseVsTarget("64bb0b9579d398aa3afcc332f0e8dc729679ddf8")
	require.NoError(t, err)
	assert.Equal(t, int64(0), runID)
	assert.Equal(t, "64bb0b9579d398aa3afcc332f0e8dc729679ddf8", sha)

	runID, sha, err = parseVsTarget("https://github.com/smartcontractkit/chainlink/actions/runs/31831066312")
	require.NoError(t, err)
	assert.Equal(t, int64(31831066312), runID)
	assert.Empty(t, sha)

	runID, sha, err = parseVsTarget(
		"https://github.com/smartcontractkit/chainlink/commit/64bb0b9579d398aa3afcc332f0e8dc729679ddf8",
	)
	require.NoError(t, err)
	assert.Equal(t, int64(0), runID)
	assert.Equal(t, "64bb0b9579d398aa3afcc332f0e8dc729679ddf8", sha)
}
