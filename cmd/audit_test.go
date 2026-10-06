package cmd

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

	"github.com/kalverra/octometrics/audit"
	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/config"
)

func TestAuditCmdRegistration(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, auditCmd)
	assert.Equal(t, "audit [workflow-url|workflow-name] [runs-count]", auditCmd.Use)
	assert.Contains(t, auditCmd.Aliases, "workflow")
	assert.Contains(t, auditCmd.Aliases, "wf")

	sub, _, err := rootCmd.Find([]string{"audit"})
	require.NoError(t, err)
	assert.Equal(t, auditCmd.Use, sub.Use)

	subWF, _, err := rootCmd.Find([]string{"workflow"})
	require.NoError(t, err)
	assert.Equal(t, auditCmd.Use, subWF.Use)

	subShort, _, err := rootCmd.Find([]string{"wf"})
	require.NoError(t, err)
	assert.Equal(t, auditCmd.Use, subShort.Use)
}

func TestAuditCmdFlags(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, auditCmd.Flags().Lookup("ai-output"))
	assert.NotNil(t, auditCmd.Flags().Lookup("json"))
	assert.NotNil(t, auditCmd.Flags().Lookup("runs"))
	assert.NotNil(t, auditCmd.Flags().ShorthandLookup("n"))
	assert.NotNil(t, auditCmd.Flags().Lookup("limit"))
	assert.NotNil(t, auditCmd.Flags().Lookup("include-runs"))
	assert.NotNil(t, auditCmd.Flags().Lookup("exclude-runs"))
	assert.NotNil(t, auditCmd.Flags().Lookup("format"))
	assert.NotNil(t, auditCmd.Flags().Lookup("output"))
	assert.NotNil(t, auditCmd.Flags().Lookup("threshold-cpu"))
	assert.NotNil(t, auditCmd.Flags().Lookup("threshold-mem"))
}

//nolint:paralleltest // modifies global config and audit flags
func TestAuditFlow_WithCachedData(t *testing.T) {
	tempDataDir := t.TempDir()
	owner := "smartcontractkit"
	repo := "chainlink-data-feeds"
	wfDir := filepath.Join(tempDataDir, owner, repo, gather.WorkflowRunsDataDir)
	logDir := filepath.Join(tempDataDir, owner, repo, "logs", "37497090385")
	require.NoError(t, os.MkdirAll(wfDir, 0o700))
	require.NoError(t, os.MkdirAll(logDir, 0o700))

	// Mock run
	now := time.Now()
	run := &gather.WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			ID:         new(int64(37497090385)),
			Name:       new("monitoring-alert-firedrill-pr"),
			Path:       new(".github/workflows/monitoring-alert-firedrill-pr.yaml"),
			Status:     new("completed"),
			Conclusion: new("success"),
			CreatedAt:  &github.Timestamp{Time: now},
			HeadBranch: new("main"),
			HTMLURL:    new("https://github.com/smartcontractkit/chainlink-data-feeds/actions/runs/37497090385"),
		},
		Jobs: []*gather.JobData{
			{
				WorkflowJob: &github.WorkflowJob{
					ID:        new(int64(112384302687)),
					Name:      new("firedrill"),
					StartedAt: &github.Timestamp{Time: now},
					Labels: []string{
						"runs-on=37497090385-p-wf-de-006/cpu=16/ram=64/family=m6i/spot=false/image=ubuntu24-full-x64",
					},
				},
				Runner: "runs-on:m6i.4xlarge",
				Cost:   280,
			},
		},
	}
	data, err := json.Marshal(run)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "37497090385.json"), data, 0o600))

	// Mock log with Job Metrics
	sampleLog := `
📊 Job Metrics
================================================================================
  system.cpu.load_average.1m                     min: 0  max: 30.63  avg: 4.37 
  system.cpu.load_average.5m                     min: 0  max: 10.29  avg: 4.13 
  system.memory.utilization (used)               min: 0.80  max: 23.83  avg: 17.94 %
================================================================================
`
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "112384302687.log"), []byte(sampleLog), 0o600))

	// Set cfg
	origCfg := cfg
	cfg = &config.Config{
		DataDir: tempDataDir,
	}
	defer func() { cfg = origCfg }()

	outMDFile := filepath.Join(t.TempDir(), "audit.md")
	auditFlagOutput = outMDFile
	auditFlagAIOutput = true
	auditFlagFormat = "md"
	auditFlagRuns = 10
	auditFlagOwner = ""
	auditFlagRepo = ""
	auditFlagWorkflow = ""
	defer func() {
		auditFlagOutput = ""
		auditFlagAIOutput = false
		auditFlagFormat = "html"
	}()

	err = runAuditFlow(
		auditCmd,
		"https://github.com/smartcontractkit/chainlink-data-feeds/actions/workflows/monitoring-alert-firedrill-pr.yaml",
	)
	require.NoError(t, err)

	content, err := os.ReadFile(outMDFile) //nolint:gosec // read test output file
	require.NoError(t, err)
	md := string(content)

	assert.Contains(t, md, "Octometrics Workflow Audit: monitoring-alert-firedrill-pr")
	assert.Contains(t, md, "Optimization & Rightsizing Recommendations")
	assert.Contains(t, md, "cpu=8/ram=32")
	assert.Contains(t, md, "firedrill")

	// Test JSON format output
	outJSONFile := filepath.Join(t.TempDir(), "audit.json")
	auditFlagOutput = outJSONFile
	auditFlagJSON = true
	auditFlagFormat = "json"
	defer func() {
		auditFlagJSON = false
		auditFlagFormat = "html"
	}()

	err = runAuditFlow(
		auditCmd,
		"https://github.com/smartcontractkit/chainlink-data-feeds/actions/workflows/monitoring-alert-firedrill-pr.yaml",
	)
	require.NoError(t, err)

	jsonBytes, err := os.ReadFile(outJSONFile) //nolint:gosec // read test output file
	require.NoError(t, err)
	var decoded audit.WorkflowAudit
	require.NoError(t, json.Unmarshal(jsonBytes, &decoded))
	assert.Equal(t, "monitoring-alert-firedrill-pr", decoded.Workflow.Name)
	assert.Equal(t, 1, decoded.SampleSize)
	require.NotEmpty(t, decoded.Recommendations)
	assert.Contains(t, decoded.Recommendations[0].RecommendedConfig, "cpu=8/ram=32")
}

//nolint:paralleltest // modifies global config and audit flags
func TestAuditFlow_IncludeAndExcludeRuns(t *testing.T) {
	tempDataDir := t.TempDir()
	owner := "smartcontractkit"
	repo := "chainlink-data-feeds"
	wfDir := filepath.Join(tempDataDir, owner, repo, gather.WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(wfDir, 0o700))

	now := time.Now()
	makeRun := func(id int64, name string) *gather.WorkflowRunData {
		return &gather.WorkflowRunData{
			WorkflowRun: &github.WorkflowRun{
				ID:         new(id),
				Name:       new("monitoring-alert-firedrill-pr"),
				Path:       new(".github/workflows/monitoring-alert-firedrill-pr.yaml"),
				Status:     new("completed"),
				Conclusion: new("success"),
				CreatedAt:  &github.Timestamp{Time: now},
				HeadBranch: new("main"),
			},
			Jobs: []*gather.JobData{
				{
					WorkflowJob: &github.WorkflowJob{
						ID:        new(id + 1000),
						Name:      new(name),
						StartedAt: &github.Timestamp{Time: now},
						Labels: []string{
							"runs-on=test/cpu=16/ram=64/family=m6i/spot=false",
						},
					},
					Runner: "runs-on:m6i.4xlarge",
					Cost:   100,
				},
			},
		}
	}

	for _, id := range []int64{101, 102, 103} {
		data, err := json.Marshal(makeRun(id, fmt.Sprintf("job-%d", id)))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(wfDir, fmt.Sprintf("%d.json", id)), data, 0o600))
	}

	origCfg := cfg
	cfg = &config.Config{DataDir: tempDataDir}
	defer func() { cfg = origCfg }()

	// 1. Test IncludeRuns
	outJSONFile := filepath.Join(t.TempDir(), "include.json")
	auditFlagOutput = outJSONFile
	auditFlagJSON = true
	auditFlagFormat = "json"
	auditFlagIncludeRuns = []string{"101", "https://github.com/smartcontractkit/chainlink-data-feeds/actions/runs/103"}
	auditFlagExcludeRuns = nil
	auditFlagRuns = 10
	defer func() {
		auditFlagOutput = ""
		auditFlagJSON = false
		auditFlagFormat = "html"
		auditFlagIncludeRuns = nil
		auditFlagExcludeRuns = nil
	}()

	err := runAuditFlow(
		auditCmd,
		"https://github.com/smartcontractkit/chainlink-data-feeds/actions/workflows/monitoring-alert-firedrill-pr.yaml",
	)
	require.NoError(t, err)

	bytes, err := os.ReadFile(outJSONFile) //nolint:gosec // test file
	require.NoError(t, err)
	var report audit.WorkflowAudit
	require.NoError(t, json.Unmarshal(bytes, &report))
	assert.Equal(t, 2, report.SampleSize)
	var sampledIDs []int64
	for _, r := range report.SampledRuns {
		sampledIDs = append(sampledIDs, r.ID)
	}
	assert.Contains(t, sampledIDs, int64(101))
	assert.Contains(t, sampledIDs, int64(103))
	assert.NotContains(t, sampledIDs, int64(102))

	// 2. Test ExcludeRuns
	outJSONFile2 := filepath.Join(t.TempDir(), "exclude.json")
	auditFlagOutput = outJSONFile2
	auditFlagIncludeRuns = nil
	auditFlagExcludeRuns = []string{"102"}
	auditFlagRuns = 10

	err = runAuditFlow(
		auditCmd,
		"https://github.com/smartcontractkit/chainlink-data-feeds/actions/workflows/monitoring-alert-firedrill-pr.yaml",
	)
	require.NoError(t, err)

	bytes2, err := os.ReadFile(outJSONFile2) //nolint:gosec // test file
	require.NoError(t, err)
	var report2 audit.WorkflowAudit
	require.NoError(t, json.Unmarshal(bytes2, &report2))
	assert.Equal(t, 2, report2.SampleSize)
	for _, r := range report2.SampledRuns {
		assert.NotEqual(t, int64(102), r.ID)
	}
}

//nolint:paralleltest // modifies global config and audit flags
func TestAuditPositionalRunsArg(t *testing.T) {
	origRuns := auditFlagRuns
	defer func() { auditFlagRuns = origRuns }()

	auditFlagRuns = 10
	// Test positional arg for runs count
	cmd := auditCmd
	err := cmd.RunE(cmd, []string{"ci.yaml", "5"})
	// It will try to run auditFlow with ci.yaml, auditFlagRuns should be updated to 5
	assert.Equal(t, 5, auditFlagRuns)
	_ = err
}
