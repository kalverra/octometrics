package gather

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestCleanLog(t *testing.T) {
	t.Parallel()

	raw := "\x1b[32m2026-08-03T19:57:55.2040149Z\x1b[0m \x1b[1mStarting step...\x1b[0m"
	cleaned := CleanLog(raw)
	assert.NotContains(t, cleaned, "\x1b")
	assert.Contains(t, cleaned, "2026-08-03T19:57:55.204014Z Starting step...")
}

func TestGetCleanJobLogs_FromDisk(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	logDir := filepath.Join(dataDir, "owner", "repo", "logs", "100")
	require.NoError(t, os.MkdirAll(logDir, 0o700))

	logFile := filepath.Join(logDir, "500.log")
	require.NoError(t, os.WriteFile(logFile, []byte("\x1b[31m2026-08-03T19:57:55.1234567Z Error!\x1b[0m"), 0o600))

	wfDir := filepath.Join(dataDir, "owner", "repo", WorkflowRunsDataDir)
	require.NoError(t, os.MkdirAll(wfDir, 0o700))
	wfData := `{"id":100,"jobs":[{"id":500,"name":"Test Job"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "100.json"), []byte(wfData), 0o600))

	log, _ := testhelpers.Setup(t)
	cleaned, err := GetCleanJobLogs(t.Context(), log, nil, "", "", 500, dataDir)
	require.NoError(t, err)
	assert.NotContains(t, cleaned, "\x1b")
	assert.Contains(t, cleaned, "2026-08-03T19:57:55.123456Z Error!")
}

func TestParseRunsOnCostSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		logs   string
		want   *RunsOnCostSummary
		wantOk bool
	}{
		{
			name: "full cost summary with log timestamps",
			logs: `2026-08-03T19:57:55.2040149Z Current runner version: '2.336.0'
2026-08-03T19:59:35.7804837Z Mapped zone name us-east-2c to zone ID use2-az3
2026-08-03T19:59:35.9822153Z ## Execution Cost Summary
2026-08-03T19:59:35.9822405Z
2026-08-03T19:59:35.9822527Z | metric                 | value           |
2026-08-03T19:59:35.9822936Z | ---------------------- | --------------- |
2026-08-03T19:59:35.9823200Z | Instance Type          | m7i.4xlarge     |
2026-08-03T19:59:35.9823452Z | Instance Lifecycle     | spot            |
2026-08-03T19:59:35.9823711Z | Region                 | us-east-2       |
2026-08-03T19:59:35.9823966Z | Platform               | Linux/UNIX      |
2026-08-03T19:59:35.9824209Z | Arch                   | x64             |
2026-08-03T19:59:35.9824858Z | Az                     | us-east-2c      |
2026-08-03T19:59:35.9825105Z | Zone ID                | use2-az3        |
2026-08-03T19:59:35.9825343Z | Duration               | 6.25 minutes    |
2026-08-03T19:59:35.9826000Z | Cost                   | $0.0280         |
2026-08-03T19:59:35.9826100Z | GitHub equivalent cost | $0.2940         |
2026-08-03T19:59:35.9826200Z | Savings                | $0.2660 (90.5%) |
2026-08-03T19:59:36.0000000Z ## Some Other Section
2026-08-03T19:59:36.0000001Z Some other content
`,
			want: &RunsOnCostSummary{
				InstanceType:         "m7i.4xlarge",
				InstanceLifecycle:    "spot",
				Region:               "us-east-2",
				Platform:             "Linux/UNIX",
				Arch:                 "x64",
				Az:                   "us-east-2c",
				ZoneID:               "use2-az3",
				Duration:             "6.25 minutes",
				CostUSD:              0.0280,
				GitHubEquivalentCost: 0.2940,
				Savings:              "$0.2660 (90.5%)",
			},
			wantOk: true,
		},
		{
			name: "cost summary with extra log noise before and after",
			logs: `Some step output here
## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | c8g.large       |
| Instance Lifecycle     | on-demand       |
| Region                 | us-west-2       |
| Platform               | Linux/UNIX      |
| Arch                   | arm64           |
| Az                     | us-west-2a      |
| Zone ID                | usw2-az2        |
| Duration               | 2.5 minutes     |
| Cost                   | $0.0090         |
| GitHub equivalent cost | $0.0125         |
| Savings                | $0.0035 (28.0%) |

Some other output after`,
			want: &RunsOnCostSummary{
				InstanceType:         "c8g.large",
				InstanceLifecycle:    "on-demand",
				Region:               "us-west-2",
				Platform:             "Linux/UNIX",
				Arch:                 "arm64",
				Az:                   "us-west-2a",
				ZoneID:               "usw2-az2",
				Duration:             "2.5 minutes",
				CostUSD:              0.0090,
				GitHubEquivalentCost: 0.0125,
				Savings:              "$0.0035 (28.0%)",
			},
			wantOk: true,
		},
		{
			name: "no cost summary in logs",
			logs: `Just regular job output
No cost data here
## Some Other Summary
| thing | value |
`,
			wantOk: false,
		},
		{
			name:   "empty logs",
			logs:   "",
			wantOk: false,
		},
		{
			name: "cost summary missing some fields",
			logs: `## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | m7i.4xlarge     |
| Cost                   | $0.0280         |
`,
			want: &RunsOnCostSummary{
				InstanceType: "m7i.4xlarge",
				CostUSD:      0.0280,
			},
			wantOk: true,
		},
		{
			name: "cost summary with cost 0",
			logs: `## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | m7i.4xlarge     |
| Cost                   | $0.0000         |
`,
			want: &RunsOnCostSummary{
				InstanceType: "m7i.4xlarge",
				CostUSD:      0,
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseRunsOnCostSummary(tt.logs)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestRunsOnCostSummaryToTenthsOfCent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		usd  float64
		want int64
	}{
		{name: "$0.0280", usd: 0.0280, want: 28},
		{name: "$0.0090", usd: 0.0090, want: 9},
		{name: "$0.2940", usd: 0.2940, want: 294},
		{name: "$0.0010", usd: 0.0010, want: 1},
		{name: "$0.0000", usd: 0.0000, want: 0},
		{name: "$1.50", usd: 1.50, want: 1500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &RunsOnCostSummary{CostUSD: tt.usd}
			assert.Equal(t, tt.want, s.CostInTenthsOfCent())
		})
	}
}

func TestFetchRunsOnCostFromLogs_DiskCache(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	// Pre-create disk cache for job 999
	cacheDir := filepath.Join(tempDir, "owner", "repo", "runs_on_costs")
	require.NoError(t, os.MkdirAll(cacheDir, 0o750))
	cacheFile := filepath.Join(cacheDir, "999.json")
	summary := &RunsOnCostSummary{InstanceType: "c7i.2xlarge", CostUSD: 0.05}
	data, err := json.Marshal(summary)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cacheFile, data, 0o600))

	// client is nil to prove no HTTP network calls are made
	cost, loadedSummary, err := fetchRunsOnCostFromLogs(t.Context(), log, nil, "owner", "repo", 999, tempDir)
	require.NoError(t, err)
	require.NotNil(t, loadedSummary)
	assert.Equal(t, int64(50), cost)
	assert.Equal(t, "c7i.2xlarge", loadedSummary.InstanceType)
}

func TestParseLogGaps(t *testing.T) {
	t.Parallel()

	rawLog := `2026-08-11T20:00:00.000Z Starting job...
2026-08-11T20:00:02.000Z Setup complete
2026-08-11T20:00:46.000Z Waiting for DON boot finished (44s gap)
2026-08-11T20:00:48.000Z Test run started
2026-08-11T20:01:13.000Z Untar cache finished (25s gap)
`
	gaps := ParseLogGaps(rawLog, 5)
	require.GreaterOrEqual(t, len(gaps), 2)
	assert.InDelta(t, 44.0, gaps[0].Duration.Seconds(), 0.001)
	assert.Contains(t, gaps[0].LineBefore, "Setup complete")
	assert.Contains(t, gaps[0].LineAfter, "Waiting for DON boot")

	assert.InDelta(t, 25.0, gaps[1].Duration.Seconds(), 0.001)
	assert.Contains(t, gaps[1].LineBefore, "Test run started")
	assert.Contains(t, gaps[1].LineAfter, "Untar cache finished")
}

func TestParseLogGaps_BufferedFlushHeuristic(t *testing.T) {
	t.Parallel()

	rawLog := `2026-08-11T20:00:00.000Z Using precompiled binary
2026-08-11T20:03:34.000Z === RUN TestA
2026-08-11T20:03:34.000Z --- PASS: TestA (0.01s)
2026-08-11T20:03:34.000Z === RUN TestB
2026-08-11T20:03:34.000Z --- PASS: TestB (0.02s)
`
	gaps := ParseLogGaps(rawLog, 5)
	require.NotEmpty(t, gaps)
	assert.True(
		t,
		gaps[0].IsBufferedFlush,
		"Gap ending with multiple lines sharing same timestamp should be flagged as buffered flush",
	)
}
