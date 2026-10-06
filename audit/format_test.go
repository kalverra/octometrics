package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
)

func sampleAudit() *WorkflowAudit {
	return &WorkflowAudit{
		Workflow: WorkflowInfo{
			Name:  "monitoring-alert-firedrill-pr",
			Path:  ".github/workflows/monitoring-alert-firedrill-pr.yaml",
			Owner: "smartcontractkit",
			Repo:  "chainlink-data-feeds",
		},
		SampleSize: 5,
		DateRange: DateRange{
			Start: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		},
		Summary: RunSummaryStats{
			TotalRuns:        5,
			SuccessRuns:      5,
			FailedRuns:       0,
			SuccessRate:      100.0,
			AvgDuration:      10 * time.Minute,
			P95Duration:      11 * time.Minute,
			TotalCostUSD:     1.40,
			AvgCostPerRunUSD: 0.28,
		},
		Jobs: []JobAudit{
			{
				Name:             "firedrill",
				RunCount:         5,
				AvgDuration:      10 * time.Minute,
				P95Duration:      11 * time.Minute,
				MinDuration:      9 * time.Minute,
				MaxDuration:      11 * time.Minute,
				TotalCostUSD:     1.40,
				AvgCostPerRunUSD: 0.28,
				Runner:           "runs-on:m6i.4xlarge",
				RunnerSpecs: gather.RunnerSpecs{
					ProvisionedCPU:   16,
					ProvisionedRAMGB: 64,
					Family:           "m6i",
					Spot:             false,
					Architecture:     "x64",
					IsRunsOn:         true,
				},
				Utilization: ResourceUtilization{
					CPUCoresProvisioned: 16,
					CPUAvgPercent:       17.5,
					CPUPeakPercent:      99.8,
					CPULoad1mAvg:        4.4,
					RAMProvisionedGB:    64,
					MemoryAvgPercent:    17.9,
					MemoryPeakPercent:   23.8,
					MemoryAvgGB:         11.5,
					MemoryPeakGB:        15.2,
					Source:              "runs-on",
				},
			},
		},
		Recommendations: []Recommendation{
			{
				Type:                      RecommendationTypeRunnerRightsizing,
				Severity:                  SeverityHigh,
				JobName:                   "firedrill",
				Title:                     "Downgrade runner for \"firedrill\": cpu=16/ram=64/family=m6i -> cpu=8/ram=32/family=m6i",
				Reason:                    "Peak memory was 15.2 GB (23.8% of 64 GB provisioned). Downgrading to 32 GB RAM leaves 52% safety headroom. Average CPU utilization was 17.5% (1m load average 4.40 on 16 vCPUs).",
				CurrentConfig:             "cpu=16/ram=64/family=m6i",
				RecommendedConfig:         "cpu=8/ram=32/family=m6i",
				ActionableYAML:            "runs-on: [\"runs-on=.../cpu=8/ram=32/family=m6i\"]",
				EstimatedSavingsPercent:   47.0,
				EstimatedSavingsPerRunUSD: 0.13,
			},
		},
		SampledRuns: []SampledRun{
			{
				ID:         37497090385,
				Status:     "completed",
				Conclusion: "success",
				Branch:     "main",
				Duration:   10 * time.Minute,
				CostUSD:    0.28,
				CreatedAt:  time.Date(2026, 10, 6, 17, 0, 0, 0, time.UTC),
				HTMLURL:    "https://github.com/smartcontractkit/chainlink-data-feeds/actions/runs/37497090385",
			},
		},
	}
}

func TestFormatMarkdown(t *testing.T) {
	t.Parallel()

	audit := sampleAudit()
	md := FormatMarkdown(audit)

	assert.Contains(t, md, "# 🔍 Octometrics Workflow Audit: monitoring-alert-firedrill-pr")
	assert.Contains(t, md, "smartcontractkit/chainlink-data-feeds")
	assert.Contains(t, md, "## 📊 Executive Summary")
	assert.Contains(t, md, "100.0% (5 passed, 0 failed)")
	assert.Contains(t, md, "## 💡 Optimization & Rightsizing Recommendations")
	assert.Contains(t, md, "Downgrade runner for \"firedrill\"")
	assert.Contains(t, md, "runs-on: [\"runs-on=.../cpu=8/ram=32/family=m6i\"]")
	assert.Contains(t, md, "## 📈 Resource Utilization Breakdown")
	assert.Contains(t, md, "15.2 GB")
	assert.Contains(t, md, "23.8%")
	assert.Contains(t, md, "## 📋 Sampled Workflow Runs")
	assert.Contains(t, md, "37497090385")
}

func TestFormatTable(t *testing.T) {
	t.Parallel()

	audit := sampleAudit()
	tbl := FormatTable(audit)

	assert.Contains(t, tbl, "OCTOMETRICS WORKFLOW AUDIT: monitoring-alert-firedrill-pr")
	assert.Contains(t, tbl, "RECOMMENDATIONS")
	assert.Contains(t, tbl, "RESOURCE UTILIZATION & RUNNER SIZING")
	assert.Contains(t, tbl, "firedrill")
	assert.Contains(t, tbl, "17.5%")
	assert.Contains(t, tbl, "15.2 GB")
}

func TestFormatJSON(t *testing.T) {
	t.Parallel()

	audit := sampleAudit()
	rawJSON, err := FormatJSON(audit)
	require.NoError(t, err)

	var decoded WorkflowAudit
	require.NoError(t, json.Unmarshal(rawJSON, &decoded))
	assert.Equal(t, audit.Workflow.Name, decoded.Workflow.Name)
	assert.Equal(t, audit.SampleSize, decoded.SampleSize)
	require.Len(t, decoded.Jobs, 1)
	assert.Equal(t, "firedrill", decoded.Jobs[0].Name)
	require.Len(t, decoded.Recommendations, 1)
}

func TestFormatHTML(t *testing.T) {
	t.Parallel()

	audit := sampleAudit()
	html, err := FormatHTML(audit)
	require.NoError(t, err)

	assert.Contains(t, html, "<title>monitoring-alert-firedrill-pr Audit | Octometrics</title>")
	assert.Contains(t, html, "Optimization & Rightsizing Recommendations")
	assert.Contains(t, html, "Downgrade runner for &#34;firedrill&#34;")
	assert.Contains(t, html, "Resource Utilization & Runner Sizing")
	assert.Contains(t, html, "15.2 GB")
	assert.Contains(t, html, "37497090385")
}
