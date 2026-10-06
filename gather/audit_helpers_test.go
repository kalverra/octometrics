package gather

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestExtractRunnerSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		labels     []string
		runnerName string
		want       RunnerSpecs
	}{
		{
			name: "RunsOn m6i 16cpu 64ram on-demand x64",
			labels: []string{
				"runs-on=123/cpu=16/ram=64/family=m6i/spot=false/image=ubuntu24-full-x64/volume=80gb",
			},
			runnerName: "runs-on:m6i.4xlarge",
			want: RunnerSpecs{
				ProvisionedCPU:   16,
				ProvisionedRAMGB: 64,
				Family:           "m6i",
				Spot:             false,
				Architecture:     "x64",
				IsRunsOn:         true,
			},
		},
		{
			name: "RunsOn arm64 spot runner",
			labels: []string{
				"runs-on=456/cpu=8/ram=32/family=c7g/spot=true/image=ubuntu24-full-arm64",
			},
			runnerName: "runs-on:c7g.2xlarge",
			want: RunnerSpecs{
				ProvisionedCPU:   8,
				ProvisionedRAMGB: 32,
				Family:           "c7g",
				Spot:             true,
				Architecture:     "arm64",
				IsRunsOn:         true,
			},
		},
		{
			name: "Docs format 4cpu-linux-x64",
			labels: []string{
				"runner=4cpu-linux-x64",
			},
			runnerName: "runs-on:4cpu-linux-x64",
			want: RunnerSpecs{
				ProvisionedCPU:   4,
				ProvisionedRAMGB: 16,
				Architecture:     "x64",
				IsRunsOn:         true,
			},
		},
		{
			name:       "GitHub Hosted 16 Core",
			labels:     []string{"ubuntu-24.04-16core"},
			runnerName: "UBUNTU_16_CORE",
			want: RunnerSpecs{
				ProvisionedCPU:   16,
				ProvisionedRAMGB: 64,
				Architecture:     "x64",
				IsRunsOn:         false,
			},
		},
		{
			name:       "GitHub Hosted default ubuntu-latest",
			labels:     []string{"ubuntu-latest"},
			runnerName: "GitHub Actions 2",
			want: RunnerSpecs{
				ProvisionedCPU:   2,
				ProvisionedRAMGB: 7,
				Architecture:     "x64",
				IsRunsOn:         false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractRunnerSpecs(tt.labels, tt.runnerName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestListWorkflowRuns_CacheFallback(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	owner := "testorg"
	repo := "testrepo"
	wfDir := filepath.Join(dataDir, owner, repo, WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(wfDir, 0o700))

	now := time.Now()
	runs := []*WorkflowRunData{
		{
			WorkflowRun: &github.WorkflowRun{
				ID:         new(int64(1001)),
				Name:       new("ci-build"),
				Path:       new(".github/workflows/ci.yaml"),
				Status:     new("completed"),
				Conclusion: new("success"),
				CreatedAt:  &github.Timestamp{Time: now.Add(-10 * time.Minute)},
			},
		},
		{
			WorkflowRun: &github.WorkflowRun{
				ID:         new(int64(1002)),
				Name:       new("ci-build"),
				Path:       new(".github/workflows/ci.yaml"),
				Status:     new("completed"),
				Conclusion: new("success"),
				CreatedAt:  &github.Timestamp{Time: now.Add(-5 * time.Minute)},
			},
		},
		{
			WorkflowRun: &github.WorkflowRun{
				ID:         new(int64(1003)),
				Name:       new("release"),
				Path:       new(".github/workflows/release.yaml"),
				Status:     new("completed"),
				Conclusion: new("success"),
				CreatedAt:  &github.Timestamp{Time: now},
			},
		},
	}

	for _, r := range runs {
		data, err := json.Marshal(r)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(wfDir, fmt.Sprintf("%d.json", r.GetID())), data, 0o600))
	}

	log, _ := testhelpers.Setup(t)

	// List ci.yaml runs - should find 1002 and 1001 in descending order
	ids, err := ListWorkflowRuns(t.Context(), log, nil, owner, repo, "ci.yaml", "", "completed", 10, dataDir)
	require.NoError(t, err)
	assert.Equal(t, []int64{1002, 1001}, ids)

	// List release.yaml runs
	ids, err = ListWorkflowRuns(
		t.Context(),
		log,
		nil,
		owner,
		repo,
		".github/workflows/release.yaml",
		"",
		"",
		10,
		dataDir,
	)
	require.NoError(t, err)
	assert.Equal(t, []int64{1003}, ids)
}

func TestEnsureJobMetrics(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	owner := "testorg"
	repo := "testrepo"
	runID := int64(2000)
	jobID := int64(3000)

	logDir := filepath.Join(dataDir, owner, repo, "logs", "2000")
	require.NoError(t, os.MkdirAll(logDir, 0o700))

	sampleMetricsLog := `
📊 Job Metrics
================================================================================
  system.cpu.load_average.1m                     min: 0  max: 12.50  avg: 3.20 
  system.cpu.load_average.5m                     min: 0  max: 6.10  avg: 2.80 
  system.memory.utilization (used)               min: 1.20  max: 25.40  avg: 18.50 %
================================================================================
`
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "3000.log"), []byte(sampleMetricsLog), 0o600))

	job := &JobData{
		WorkflowJob: &github.WorkflowJob{
			ID: new(jobID),
		},
	}

	log, _ := testhelpers.Setup(t)
	EnsureJobMetrics(t.Context(), log, nil, owner, repo, job, runID, dataDir)
	require.NotNil(t, job.RunsOnMetrics)
	assert.InDelta(t, 3.20, job.RunsOnMetrics.CPULoad1m.Avg, 0.001)
	assert.InDelta(t, 25.40, job.RunsOnMetrics.MemoryUtilPct.Max, 0.001)
}
