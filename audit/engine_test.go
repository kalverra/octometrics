package audit

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
)

func TestBuildAuditReport_Rightsizing(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Construct 5 sample runs for firedrill workflow matching investigation notes data
	runs := make([]*gather.WorkflowRunData, 5)
	for i := range 5 {
		runID := int64(37497090000 + i)
		jobID := int64(112384300000 + i)

		runs[i] = &gather.WorkflowRunData{
			WorkflowRun: &github.WorkflowRun{
				ID:         new(runID),
				Name:       new("monitoring-alert-firedrill-pr"),
				Path:       new(".github/workflows/monitoring-alert-firedrill-pr.yaml"),
				Status:     new("completed"),
				Conclusion: new("success"),
				CreatedAt:  &github.Timestamp{Time: now.Add(time.Duration(-i) * time.Hour)},
				HeadBranch: new("main"),
				HTMLURL: new(
					fmt.Sprintf("https://github.com/smartcontractkit/chainlink-data-feeds/actions/runs/%d", runID),
				),
			},
			RunCompletedAt: now.Add(time.Duration(-i)*time.Hour + 10*time.Minute),
			Cost:           280, // $0.28
			Jobs: []*gather.JobData{
				{
					WorkflowJob: &github.WorkflowJob{
						ID:          new(jobID),
						Name:        new("firedrill"),
						StartedAt:   &github.Timestamp{Time: now.Add(time.Duration(-i) * time.Hour)},
						CompletedAt: &github.Timestamp{Time: now.Add(time.Duration(-i)*time.Hour + 10*time.Minute)},
						Labels: []string{
							"runs-on=test-id/cpu=16/ram=64/family=m6i/spot=false/image=ubuntu24-full-x64",
						},
					},
					Runner: "runs-on:m6i.4xlarge (on-demand)",
					Cost:   280,
					RunsOnMetrics: &gather.RunsOnJobMetrics{
						CPULoad1m: &gather.MinMaxAvg{
							Min: 0,
							Max: 30.63,
							Avg: 4.37,
						},
						CPUUtilCorePct: &gather.MinMaxAvg{
							Min: 0,
							Max: 99.80,
							Avg: 17.47,
						},
						MemoryUtilPct: &gather.MinMaxAvg{
							Min: 0.80,
							Max: 23.83,
							Avg: 17.94,
						},
						NetworkIOMB: &gather.MinMaxAvg{
							Min: 0.23,
							Max: 6800.0,
							Avg: 3000.0,
						},
					},
				},
			},
		}
	}

	opts := Options{
		Owner:               "smartcontractkit",
		Repo:                "chainlink-data-feeds",
		Workflow:            "monitoring-alert-firedrill-pr.yaml",
		RunsCount:           5,
		CPUThresholdPercent: 30.0,
		RAMThresholdPercent: 50.0,
	}

	auditReport := BuildAuditReport(runs, opts)
	require.NotNil(t, auditReport)

	assert.Equal(t, 5, auditReport.SampleSize)
	assert.Equal(t, 5, auditReport.Summary.TotalRuns)
	assert.Equal(t, 5, auditReport.Summary.SuccessRuns)
	assert.InDelta(t, 100.0, auditReport.Summary.SuccessRate, 0.001)

	require.Len(t, auditReport.Jobs, 1)
	job := auditReport.Jobs[0]
	assert.Equal(t, "firedrill", job.Name)
	assert.Equal(t, 16, job.RunnerSpecs.ProvisionedCPU)
	assert.InDelta(t, 64.0, job.RunnerSpecs.ProvisionedRAMGB, 0.001)
	assert.Equal(t, "m6i", job.RunnerSpecs.Family)
	assert.False(t, job.RunnerSpecs.Spot)

	// Resource utilization verification
	assert.InDelta(t, 17.47, job.Utilization.CPUAvgPercent, 0.01)
	assert.InDelta(t, 99.80, job.Utilization.CPUPeakPercent, 0.01)
	assert.InDelta(t, 4.37, job.Utilization.CPULoad1mAvg, 0.01)
	assert.InDelta(t, 17.94, job.Utilization.MemoryAvgPercent, 0.01)
	assert.InDelta(t, 23.83, job.Utilization.MemoryPeakPercent, 0.01)
	// Peak GB: 64 * 0.2383 = 15.25 GB
	assert.InDelta(t, 15.25, job.Utilization.MemoryPeakGB, 0.1)

	// Check recommendations
	require.NotEmpty(t, auditReport.Recommendations)

	var rightsizingRec *Recommendation
	var spotRec *Recommendation
	for i := range auditReport.Recommendations {
		rec := &auditReport.Recommendations[i]
		if rec.Type == RecommendationTypeRunnerRightsizing {
			rightsizingRec = rec
		}
		if rec.Type == RecommendationTypeCostOptimization && rec.RecommendedConfig == "spot=true" {
			spotRec = rec
		}
	}

	require.NotNil(t, rightsizingRec, "should generate runner rightsizing recommendation")
	assert.Contains(t, rightsizingRec.CurrentConfig, "cpu=16/ram=64")
	assert.Contains(t, rightsizingRec.RecommendedConfig, "cpu=8/ram=32")
	assert.Greater(t, rightsizingRec.EstimatedSavingsPercent, 30.0)

	require.NotNil(t, spotRec, "should generate spot instance recommendation")
	assert.Equal(t, "spot=true", spotRec.RecommendedConfig)
}

func TestBuildAuditReport_WellSizedRunner(t *testing.T) {
	t.Parallel()

	now := time.Now()
	runs := []*gather.WorkflowRunData{
		{
			WorkflowRun: &github.WorkflowRun{
				ID:         new(int64(5001)),
				Name:       new("well-sized"),
				Path:       new(".github/workflows/well-sized.yaml"),
				Status:     new("completed"),
				Conclusion: new("success"),
				CreatedAt:  &github.Timestamp{Time: now},
			},
			Jobs: []*gather.JobData{
				{
					WorkflowJob: &github.WorkflowJob{
						ID:   new(int64(6001)),
						Name: new("build"),
						Labels: []string{
							"runs-on=id/cpu=4/ram=16/family=c7g/spot=true/image=ubuntu24-full-arm64",
						},
					},
					Runner: "runs-on:c7g.xlarge (spot)",
					Cost:   100,
					RunsOnMetrics: &gather.RunsOnJobMetrics{
						CPULoad1m: &gather.MinMaxAvg{
							Min: 1.0, Max: 4.0, Avg: 3.5,
						},
						CPUUtilCorePct: &gather.MinMaxAvg{
							Min: 20.0, Max: 85.0, Avg: 65.0,
						},
						MemoryUtilPct: &gather.MinMaxAvg{
							Min: 30.0, Max: 78.0, Avg: 60.0,
						},
					},
				},
			},
		},
	}

	opts := Options{
		Owner:               "test",
		Repo:                "repo",
		Workflow:            "well-sized.yaml",
		RunsCount:           1,
		CPUThresholdPercent: 30.0,
		RAMThresholdPercent: 50.0,
	}

	report := BuildAuditReport(runs, opts)
	require.NotNil(t, report)

	// Since CPU avg is 65% (>30%) and Memory peak is 78% (>50%), no rightsizing downgrade should be recommended
	for _, rec := range report.Recommendations {
		assert.NotEqual(t, RecommendationTypeRunnerRightsizing, rec.Type)
	}
}
