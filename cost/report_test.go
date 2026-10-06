package cost

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleTestReport() *Report {
	return &Report{
		DateStart:         time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		DateEnd:           time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
		Organization:      "smartcontractkit",
		TotalMinutes:      100000,
		TotalGHANetCost:   1200.0,
		TotalGHAGrossCost: 1500.0,
		TotalRunsOnCost:   3500.0,
		TotalComputeCost:  4700.0,
		ByRunnerType: map[RunnerType]*RunnerBreakdown{
			RunnerTypeGHANative: {
				RunnerName: string(RunnerTypeGHANative),
				RunnerType: RunnerTypeGHANative,
				Minutes:    60000,
				NetAmount:  1200.0,
			},
			RunnerTypeRunsOn: {
				RunnerName:    string(RunnerTypeRunsOn),
				RunnerType:    RunnerTypeRunsOn,
				Minutes:       40000,
				EstimatedCost: 3500.0,
			},
		},
		ByRunner: []*RunnerBreakdown{
			{RunnerName: "32cpu-linux-x64", RunnerType: RunnerTypeRunsOn, Minutes: 30000, EstimatedCost: 3000.0},
			{
				RunnerName:  "actions_linux",
				RunnerType:  RunnerTypeGHANative,
				Minutes:     50000,
				NetAmount:   1000.0,
				GrossAmount: 1250.0,
			},
		},
		ByRepo: []*RepoBreakdown{
			{Repository: "chainlink", TotalMinutes: 70000, TotalCost: 3800.0, GHANetCost: 800.0, RunsOnCost: 3000.0},
			{Repository: "vm-alerts", TotalMinutes: 30000, TotalCost: 900.0, GHANetCost: 900.0, RunsOnCost: 0.0},
		},
		ByWorkflow: []*WorkflowBreakdown{
			{
				Repository:    "chainlink",
				WorkflowPath:  ".github/workflows/integration.yml",
				Runner:        "32cpu-linux-x64",
				RunnerType:    RunnerTypeRunsOn,
				Minutes:       20000,
				EstimatedCost: 2000.0,
			},
			{
				Repository:   "chainlink",
				WorkflowPath: ".github/workflows/unit.yml",
				Runner:       "actions_linux",
				RunnerType:   RunnerTypeGHANative,
				Minutes:      15000,
				NetAmount:    500.0,
			},
		},
		Savings: []*SavingsEstimate{
			{
				SKU:                 "actions_linux_16_core",
				EquivalentRunner:    "16cpu-linux-x64",
				Minutes:             5000,
				CurrentNetCost:      210.0,
				EstimatedRunsOnCost: 180.0,
				EstimatedSavings:    30.0,
				SavingsPercent:      14.3,
			},
		},
		TotalProjectedSavings: 30.0,
	}
}

func TestFormatTable(t *testing.T) {
	t.Parallel()

	r := sampleTestReport()
	out := FormatTable(r)

	assert.Contains(t, out, "OVERALL CI RUNNER SPENDING")
	assert.Contains(t, out, "smartcontractkit")
	assert.Contains(t, out, "GHA native")
	assert.Contains(t, out, "runs-on")
	assert.Contains(t, out, "chainlink")
	assert.Contains(t, out, "actions_linux_16_core")
}

func TestFormatMarkdown(t *testing.T) {
	t.Parallel()

	r := sampleTestReport()
	md := FormatMarkdown(r)

	assert.Contains(t, md, "# GitHub Actions CI Runner Cost & Savings Report")
	assert.Contains(t, md, "| Runner Type | Total Minutes |")
	assert.Contains(t, md, "| `chainlink` |")
	assert.Contains(t, md, "Potential Savings from Migrating to Runs-On")
}

func TestFormatJSON(t *testing.T) {
	t.Parallel()

	r := sampleTestReport()
	jsonBytes, err := FormatJSON(r)
	require.NoError(t, err)

	var unmarshaled Report
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, r.Organization, unmarshaled.Organization)
	assert.InDelta(t, r.TotalComputeCost, unmarshaled.TotalComputeCost, 0.01)
}
