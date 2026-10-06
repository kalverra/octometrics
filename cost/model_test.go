package cost

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateCostAndSavings(t *testing.T) {
	t.Parallel()

	records := []*UsageRecord{
		{
			Date:               time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			Product:            "actions",
			SKU:                "actions_linux_16_core",
			Quantity:           1000,
			AppliedCostPerUnit: 0.042,
			GrossAmount:        42.0,
			NetAmount:          42.0,
			Organization:       "smartcontractkit",
			Repository:         "chainlink",
			WorkflowPath:       ".github/workflows/large.yml",
			RunnerType:         RunnerTypeGHANative,
		},
		{
			Date:               time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			Product:            "actions",
			SKU:                "actions_linux",
			Quantity:           10000,
			AppliedCostPerUnit: 0.006,
			GrossAmount:        60.0,
			NetAmount:          50.0,
			Organization:       "smartcontractkit",
			Repository:         "chainlink",
			WorkflowPath:       ".github/workflows/small.yml",
			RunnerType:         RunnerTypeGHANative,
		},
		{
			Date:               time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			Product:            "actions",
			SKU:                "actions_self_hosted_linux",
			Quantity:           5000,
			AppliedCostPerUnit: 0,
			GrossAmount:        0,
			NetAmount:          0,
			Organization:       "smartcontractkit",
			Repository:         "chainlink",
			WorkflowPath:       ".github/workflows/integ.yml",
			RunnerType:         RunnerTypeRunsOn,
		},
	}

	specs := map[string]*WorkflowSpec{
		"chainlink:.github/workflows/integ.yml": {
			Repository:   "chainlink",
			WorkflowPath: ".github/workflows/integ.yml",
			RunnerSpec:   "32cpu-linux-x64",
			RatePerMin:   0.098,
		},
	}

	report := GenerateReport(records, specs, nil)
	require.NotNil(t, report)

	// Verify Overall Totals
	assert.InDelta(t, 16000.0, report.TotalMinutes, 0.01)
	assert.InDelta(t, 92.0, report.TotalGHANetCost, 0.01)
	assert.InDelta(t, 102.0, report.TotalGHAGrossCost, 0.01)
	// 5000 mins * 0.098 = 490.0
	assert.InDelta(t, 490.0, report.TotalRunsOnCost, 0.01)
	assert.InDelta(t, 582.0, report.TotalComputeCost, 0.01)

	// Verify Runner Type breakdown
	ghaBreakdown := report.ByRunnerType[RunnerTypeGHANative]
	require.NotNil(t, ghaBreakdown)
	assert.InDelta(t, 11000.0, ghaBreakdown.Minutes, 0.01)
	assert.InDelta(t, 92.0, ghaBreakdown.NetAmount, 0.01)

	roBreakdown := report.ByRunnerType[RunnerTypeRunsOn]
	require.NotNil(t, roBreakdown)
	assert.InDelta(t, 5000.0, roBreakdown.Minutes, 0.01)
	assert.InDelta(t, 490.0, roBreakdown.EstimatedCost, 0.01)

	// Verify Savings Calculations
	require.NotEmpty(t, report.Savings)
	// actions_linux_16_core (1000 mins @ $0.042 net = $42.0)
	// Equivalent Runs-On: 16cpu-linux-x64 or 16cpu-linux-arm64
	// Blended rate ~ $0.038 -> 1000 * 0.038 = $38.0 -> Savings: $4.00 vs Net
	var savings16 *SavingsEstimate
	for _, s := range report.Savings {
		if s.SKU == "actions_linux_16_core" {
			savings16 = s
			break
		}
	}
	require.NotNil(t, savings16)
	assert.InDelta(t, 42.0, savings16.CurrentNetCost, 0.01)
	assert.Positive(t, savings16.EstimatedSavings)
}

func TestCalculateCostWithCloudzero(t *testing.T) {
	t.Parallel()

	records := []*UsageRecord{
		{
			Date:         time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			Product:      "actions",
			SKU:          "actions_self_hosted_linux",
			Quantity:     1000,
			Organization: "smartcontractkit",
			Repository:   "chainlink",
			WorkflowPath: ".github/workflows/integ.yml",
			RunnerType:   RunnerTypeRunsOn,
		},
	}

	specs := map[string]*WorkflowSpec{
		"chainlink:.github/workflows/integ.yml": {
			Repository:   "chainlink",
			WorkflowPath: ".github/workflows/integ.yml",
			RunnerSpec:   "32cpu-linux-x64",
			RatePerMin:   0.013,
		},
	}

	cz := &CloudzeroResult{
		RepoCosts: map[string]float64{
			"chainlink": 12.50,
		},
		UntaggedCost: -1.00,
		TotalCost:    11.50,
	}

	report := GenerateReport(records, specs, cz)
	require.NotNil(t, report)
	assert.True(t, report.CloudzeroExact)
	assert.InDelta(t, 11.50, report.TotalRunsOnCost, 0.01)
	assert.InDelta(t, 12.50, report.ByRepo[0].RunsOnCost, 0.01)
	assert.InDelta(t, 12.50, report.ByWorkflow[0].EstimatedCost, 0.01)
}
