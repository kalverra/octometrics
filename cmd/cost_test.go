package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostCmdRegistration(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, costCmd)
	assert.Equal(t, "cost", costCmd.Use)
	assert.True(t, costCmd.Hidden)
	assert.Contains(t, costCmd.Long, "rusty and experimental")
	assert.NotContains(t, rootCmd.UsageString(), "cost [command]")

	cmd, _, err := rootCmd.Find([]string{"cost", "report"})
	require.NoError(t, err)
	assert.Equal(t, "report", cmd.Use)
	assert.Contains(t, costReportCmd.Long, "rusty and experimental")

	assert.NotNil(t, costReportCmd.Flags().Lookup("csv"))
	assert.NotNil(t, costReportCmd.Flags().Lookup("org"))
	assert.NotNil(t, costReportCmd.Flags().Lookup("format"))
	assert.NotNil(t, costReportCmd.Flags().Lookup("output"))
}
