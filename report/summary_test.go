package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/monitor"
)

func TestBuildReport(t *testing.T) {
	t.Parallel()

	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	analysis := &monitor.Analysis{
		JobName: "Build and Test",
		SystemInfo: &monitor.SystemInfo{
			Memory: &monitor.SystemMemoryInfo{Total: 8 * 1024 * 1024 * 1024},
			Disk:   &monitor.SystemDiskInfo{Total: 100 * 1024 * 1024 * 1024},
		},
		CPUMeasurements: map[int][]*monitor.CPUMeasurement{
			0: {
				{Time: base, UsedPercent: 50},
				{Time: base.Add(time.Second), UsedPercent: 80},
			},
		},
		MemoryMeasurements: []*monitor.MemoryMeasurement{
			{Time: base, Used: 4 * 1024 * 1024 * 1024},
			{Time: base.Add(time.Second), Used: 6 * 1024 * 1024 * 1024},
		},
		DiskMeasurements: []*monitor.DiskMeasurement{
			{Time: base, Used: 50 * 1024 * 1024 * 1024},
		},
		IOMeasurements: []*monitor.IOMeasurement{
			{Time: base, BytesSent: 100 * 1024 * 1024, BytesRecv: 200 * 1024 * 1024},
		},
	}

	steps := []*github.TaskStep{
		{
			Name:        new("Setup"),
			Number:      new(int64(1)),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: base},
			CompletedAt: &github.Timestamp{Time: base.Add(5 * time.Second)},
		},
	}

	result := buildReport(analysis, steps, nil)

	assert.Contains(t, result, "## Octometrics — Build and Test")
	assert.Contains(t, result, "### Step Timeline")
	assert.Contains(t, result, "### CPU Usage")
	assert.Contains(t, result, "### Memory Usage")
	assert.Contains(t, result, "### Disk Usage")
	assert.Contains(t, result, "### Network I/O")
	assert.Contains(t, result, "### Resource Summary")
	assert.Contains(t, result, "| CPU |")
	assert.Contains(t, result, "| Memory |")
	assert.Contains(t, result, "| Disk |")
	assert.Contains(t, result, "| Net Sent |")
}

func TestBuildReportCPUPerCore(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	analysis := &monitor.Analysis{
		JobName: "CI",
		CPUMeasurements: map[int][]*monitor.CPUMeasurement{
			0: {
				{Time: base, UsedPercent: 10},
				{Time: base.Add(time.Second), UsedPercent: 20},
			},
			1: {
				{Time: base, UsedPercent: 5},
				{Time: base.Add(time.Second), UsedPercent: 15},
			},
		},
	}
	result := buildReport(analysis, nil, nil)
	assert.Contains(t, result, "### CPU Usage")
	assert.Contains(t, result, "#### CPU per core (2 cores)")
	assert.Equal(t, 2, strings.Count(result, "```mermaid"), "aggregate CPU plus one multi-line per-core block")
}

func TestBuildReportEmptyAnalysis(t *testing.T) {
	t.Parallel()

	analysis := &monitor.Analysis{
		SystemInfo:      &monitor.SystemInfo{},
		CPUMeasurements: map[int][]*monitor.CPUMeasurement{},
	}

	result := buildReport(analysis, nil, nil)
	assert.Contains(t, result, "## Octometrics Report")
	assert.NotContains(t, result, "### Step Timeline")
	assert.NotContains(t, result, "### CPU Usage")
}

func TestMetricSummaryTable(t *testing.T) {
	t.Parallel()

	t.Run("all metrics present", func(t *testing.T) {
		t.Parallel()
		base := time.Now()
		analysis := &monitor.Analysis{
			SystemInfo: &monitor.SystemInfo{
				Memory: &monitor.SystemMemoryInfo{Total: 16 * 1024 * 1024 * 1024},
				Disk:   &monitor.SystemDiskInfo{Total: 200 * 1024 * 1024 * 1024},
			},
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {{Time: base, UsedPercent: 90}},
			},
			MemoryMeasurements: []*monitor.MemoryMeasurement{
				{Time: base, Used: 8 * 1024 * 1024 * 1024},
			},
			DiskMeasurements: []*monitor.DiskMeasurement{
				{Time: base, Used: 100 * 1024 * 1024 * 1024},
			},
			IOMeasurements: []*monitor.IOMeasurement{
				{Time: base, BytesSent: 500 * 1024 * 1024, BytesRecv: 2 * 1024 * 1024 * 1024},
			},
		}

		table := metricSummaryTable(analysis)
		assert.Contains(t, table, "| CPU |")
		assert.Contains(t, table, "| Memory |")
		assert.Contains(t, table, "| Disk |")
		assert.Contains(t, table, "| Net Sent |")
		assert.Contains(t, table, "| Net Recv |")
	})

	t.Run("empty analysis returns empty", func(t *testing.T) {
		t.Parallel()
		analysis := &monitor.Analysis{
			SystemInfo:      &monitor.SystemInfo{},
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{},
		}
		assert.Empty(t, metricSummaryTable(analysis))
	})
}

func TestWriteSummary(t *testing.T) {
	t.Parallel()

	t.Run("writes to file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "summary.md")

		err := writeSummary(path, "# Hello\nWorld\n")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Clean(path))
		require.NoError(t, err)
		assert.Equal(t, "# Hello\nWorld\n", string(data))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "summary.md")

		require.NoError(t, os.WriteFile(path, []byte("existing\n"), 0600))

		err := writeSummary(path, "appended\n")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Clean(path))
		require.NoError(t, err)
		assert.Equal(t, "existing\nappended\n", string(data))
	})

	t.Run("empty path returns error", func(t *testing.T) {
		t.Parallel()
		err := writeSummary("", "content")
		assert.Error(t, err)
	})
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0 B", formatBytes(0))
	assert.Equal(t, "500 B", formatBytes(500))
	assert.Equal(t, "1.0 KB", formatBytes(1024))
	assert.Equal(t, "1.0 MB", formatBytes(1024*1024))
	assert.Equal(t, "1.0 GB", formatBytes(1024*1024*1024))
	assert.Equal(t, "2.5 GB", formatBytes(uint64(2.5*1024*1024*1024)))
}
