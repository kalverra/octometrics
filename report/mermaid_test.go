package report

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
	"github.com/kalverra/octometrics/monitor"
)

func TestGanttChart(t *testing.T) {
	t.Parallel()

	t.Run("empty steps", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, ganttChart(nil))
		assert.Empty(t, ganttChart([]*github.TaskStep{}))
	})

	t.Run("steps with timing", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		steps := []*github.TaskStep{
			{
				Name:        new("Set up job"),
				Number:      new(int64(1)),
				Conclusion:  new("success"),
				StartedAt:   &github.Timestamp{Time: base},
				CompletedAt: &github.Timestamp{Time: base.Add(5 * time.Second)},
			},
			{
				Name:        new("Run tests"),
				Number:      new(int64(2)),
				Conclusion:  new("success"),
				StartedAt:   &github.Timestamp{Time: base.Add(5 * time.Second)},
				CompletedAt: &github.Timestamp{Time: base.Add(65 * time.Second)},
			},
			{
				Name:        new("Cleanup"),
				Number:      new(int64(3)),
				Conclusion:  new("success"),
				StartedAt:   &github.Timestamp{Time: base.Add(65 * time.Second)},
				CompletedAt: &github.Timestamp{Time: base.Add(68 * time.Second)},
			},
		}

		result := ganttChart(steps)
		assert.Contains(t, result, "```mermaid")
		assert.Contains(t, result, "gantt")
		assert.Contains(t, result, "Set up job")
		assert.Contains(t, result, "Run tests")
		assert.Contains(t, result, "Cleanup")
		assert.Contains(t, result, "dateFormat")
	})

	t.Run("skipped steps are excluded", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		steps := []*github.TaskStep{
			{
				Name:        new("Build"),
				Number:      new(int64(1)),
				Conclusion:  new("success"),
				StartedAt:   &github.Timestamp{Time: base},
				CompletedAt: &github.Timestamp{Time: base.Add(30 * time.Second)},
			},
			{
				Name:        new("Skipped step"),
				Number:      new(int64(2)),
				Conclusion:  new("skipped"),
				StartedAt:   &github.Timestamp{Time: base.Add(30 * time.Second)},
				CompletedAt: &github.Timestamp{Time: base.Add(30 * time.Second)},
			},
		}

		result := ganttChart(steps)
		assert.Contains(t, result, "Build")
		assert.NotContains(t, result, "Skipped step")
	})

	t.Run("failed steps use crit status", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		steps := []*github.TaskStep{
			{
				Name:        new("Failing step"),
				Number:      new(int64(1)),
				Conclusion:  new("failure"),
				StartedAt:   &github.Timestamp{Time: base},
				CompletedAt: &github.Timestamp{Time: base.Add(10 * time.Second)},
			},
		}

		result := ganttChart(steps)
		assert.Contains(t, result, "crit")
	})
}

func TestMonitoringMermaidCharts(t *testing.T) {
	t.Parallel()

	t.Run("nil analysis", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, MonitoringMermaidCharts(nil))
	})

	t.Run("matches report markdown body without fences", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: base, UsedPercent: 50.0},
					{Time: base.Add(time.Second), UsedPercent: 75.0},
				},
			},
		}
		charts := MonitoringMermaidCharts(analysis)
		require.Len(t, charts, 1)
		assert.Equal(t, "CPU Usage", charts[0].Title)
		assert.Contains(t, charts[0].Diagram, "xychart-beta")
		assert.NotContains(t, charts[0].Diagram, "```")
		assert.Contains(t, cpuChart(analysis), charts[0].Diagram)
	})

	t.Run("multi-core adds per-CPU charts after aggregate", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: base, UsedPercent: 50.0},
					{Time: base.Add(time.Second), UsedPercent: 75.0},
				},
				1: {
					{Time: base, UsedPercent: 40.0},
					{Time: base.Add(time.Second), UsedPercent: 60.0},
				},
			},
		}
		charts := MonitoringMermaidCharts(analysis)
		require.Len(t, charts, 3)
		assert.Equal(t, "CPU Usage", charts[0].Title)
		assert.Equal(t, "CPU 0 Usage (%)", charts[1].Title)
		assert.Equal(t, "CPU 1 Usage (%)", charts[2].Title)
		assert.Contains(t, charts[1].Diagram, "xychart-beta")
		assert.Contains(t, charts[2].Diagram, "xychart-beta")
	})
}

func TestMonitoringMermaidChartsWithWindow(t *testing.T) {
	t.Parallel()

	t.Run("nil analysis", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		assert.Nil(t, MonitoringMermaidChartsWithWindow(nil, base, base.Add(time.Minute)))
	})

	t.Run("invalid window", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {{Time: base, UsedPercent: 10}},
			},
		}
		assert.Nil(t, MonitoringMermaidChartsWithWindow(analysis, base, base))
		assert.Nil(t, MonitoringMermaidChartsWithWindow(analysis, base.Add(time.Minute), base))
	})

	t.Run("x-axis spans job window in seconds", func(t *testing.T) {
		t.Parallel()
		winStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		winEnd := winStart.Add(45 * time.Second)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: winStart, UsedPercent: 10},
					{Time: winStart.Add(22 * time.Second), UsedPercent: 90},
					{Time: winEnd, UsedPercent: 20},
				},
			},
		}
		charts := MonitoringMermaidChartsWithWindow(analysis, winStart, winEnd)
		require.Len(t, charts, 1)
		assert.Contains(t, charts[0].Diagram, `x-axis "Seconds" 0 --> 45`)
		assert.Contains(t, charts[0].Diagram, "line [")
	})

	t.Run("axis extends past window when samples run longer", func(t *testing.T) {
		t.Parallel()
		winStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		winEnd := winStart.Add(10 * time.Second)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: winStart, UsedPercent: 10},
					{Time: winStart.Add(30 * time.Second), UsedPercent: 50},
				},
			},
		}
		charts := MonitoringMermaidChartsWithWindow(analysis, winStart, winEnd)
		require.Len(t, charts, 1)
		assert.Contains(t, charts[0].Diagram, `x-axis "Seconds" 0 --> 30`)
	})

	t.Run("long window uses minutes axis", func(t *testing.T) {
		t.Parallel()
		winStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		winEnd := winStart.Add(150 * time.Second)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: winStart, UsedPercent: 10},
					{Time: winEnd, UsedPercent: 20},
				},
			},
		}
		charts := MonitoringMermaidChartsWithWindow(analysis, winStart, winEnd)
		require.Len(t, charts, 1)
		assert.Contains(t, charts[0].Diagram, `x-axis "Minutes" 0 --> 3`)
	})

	t.Run("multi-core windowed adds per-CPU charts", func(t *testing.T) {
		t.Parallel()
		winStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		winEnd := winStart.Add(10 * time.Second)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: winStart, UsedPercent: 10},
					{Time: winEnd, UsedPercent: 20},
				},
				1: {
					{Time: winStart, UsedPercent: 5},
					{Time: winEnd, UsedPercent: 15},
				},
			},
		}
		charts := MonitoringMermaidChartsWithWindow(analysis, winStart, winEnd)
		require.Len(t, charts, 3)
		assert.Equal(t, "CPU Usage", charts[0].Title)
		assert.Equal(t, "CPU 0 Usage (%)", charts[1].Title)
		assert.Equal(t, "CPU 1 Usage (%)", charts[2].Title)
	})
}

func TestCPUChart(t *testing.T) {
	t.Parallel()

	t.Run("empty measurements", func(t *testing.T) {
		t.Parallel()
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{},
		}
		assert.Empty(t, cpuChart(analysis))
	})

	t.Run("generates xychart-beta", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		analysis := &monitor.Analysis{
			CPUMeasurements: map[int][]*monitor.CPUMeasurement{
				0: {
					{Time: base, UsedPercent: 50.0},
					{Time: base.Add(time.Second), UsedPercent: 75.0},
					{Time: base.Add(2 * time.Second), UsedPercent: 30.0},
				},
				1: {
					{Time: base, UsedPercent: 40.0},
					{Time: base.Add(time.Second), UsedPercent: 60.0},
					{Time: base.Add(2 * time.Second), UsedPercent: 20.0},
				},
			},
		}

		result := cpuChart(analysis)
		assert.Contains(t, result, "```mermaid")
		assert.Contains(t, result, "xychart-beta")
		assert.Contains(t, result, "CPU Usage")
		assert.Contains(t, result, `x-axis "Seconds" 0 -->`, "short duration should use Seconds x-axis")
	})
}

func TestMemoryChart(t *testing.T) {
	t.Parallel()

	t.Run("empty measurements", func(t *testing.T) {
		t.Parallel()
		analysis := &monitor.Analysis{
			MemoryMeasurements: []*monitor.MemoryMeasurement{},
		}
		assert.Empty(t, memoryChart(analysis))
	})

	t.Run("generates chart with total from system info", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		analysis := &monitor.Analysis{
			SystemInfo: &monitor.SystemInfo{
				Memory: &monitor.SystemMemoryInfo{Total: 8 * 1024 * 1024 * 1024},
			},
			MemoryMeasurements: []*monitor.MemoryMeasurement{
				{Time: base, Used: 2 * 1024 * 1024 * 1024},
				{Time: base.Add(time.Second), Used: 4 * 1024 * 1024 * 1024},
			},
		}

		result := memoryChart(analysis)
		assert.Contains(t, result, "Memory Usage (GB)", "8 GB total should select GB unit")
		assert.Contains(t, result, "xychart-beta")
		assert.Contains(t, result, `x-axis "Seconds" 0 -->`, "short duration should use Seconds x-axis")
	})
}

func TestDiskChart(t *testing.T) {
	t.Parallel()

	analysis := &monitor.Analysis{
		SystemInfo: &monitor.SystemInfo{
			Disk: &monitor.SystemDiskInfo{Total: 100 * 1024 * 1024 * 1024},
		},
		DiskMeasurements: []*monitor.DiskMeasurement{
			{Time: time.Now(), Used: 50 * 1024 * 1024 * 1024},
			{Time: time.Now().Add(time.Second), Used: 51 * 1024 * 1024 * 1024},
		},
	}

	result := diskChart(analysis)
	assert.Contains(t, result, "Disk Usage (GB)", "100 GB total should select GB unit")
}

func TestIOChart(t *testing.T) {
	t.Parallel()

	analysis := &monitor.Analysis{
		IOMeasurements: []*monitor.IOMeasurement{
			{Time: time.Now(), BytesSent: 1024 * 1024, BytesRecv: 2 * 1024 * 1024},
			{Time: time.Now().Add(time.Second), BytesSent: 2 * 1024 * 1024, BytesRecv: 4 * 1024 * 1024},
		},
	}

	result := ioChart(analysis)
	assert.Contains(t, result, "Network Sent (MB)", "MB-range data should select MB unit")
	assert.Contains(t, result, "Network Received (MB)", "MB-range data should select MB unit")
}

func TestDownsample(t *testing.T) {
	t.Parallel()

	t.Run("fewer points than target returns all", func(t *testing.T) {
		t.Parallel()
		points := []timeValue{
			{Time: time.Now(), Value: 1},
			{Time: time.Now(), Value: 2},
		}
		result := downsample(points, 500, maxAggregator)
		assert.Len(t, result, 2)
	})

	t.Run("downsamples to target count", func(t *testing.T) {
		t.Parallel()
		base := time.Now()
		points := make([]timeValue, 100)
		for i := range points {
			points[i] = timeValue{
				Time:  base.Add(time.Duration(i) * time.Second),
				Value: float64(i),
			}
		}

		result := downsample(points, 10, maxAggregator)
		require.LessOrEqual(t, len(result), 11)
		require.GreaterOrEqual(t, len(result), 9)
	})

	t.Run("max aggregator preserves peak", func(t *testing.T) {
		t.Parallel()
		base := time.Now()
		points := []timeValue{
			{Time: base, Value: 10},
			{Time: base.Add(time.Second), Value: 90},
			{Time: base.Add(2 * time.Second), Value: 20},
			{Time: base.Add(3 * time.Second), Value: 5},
		}

		result := downsample(points, 2, maxAggregator)
		var foundPeak bool
		for _, r := range result {
			if r.Value == 90 {
				foundPeak = true
			}
		}
		assert.True(t, foundPeak, "peak value should be preserved by max aggregator")
	})

	t.Run("last aggregator takes final value", func(t *testing.T) {
		t.Parallel()
		base := time.Now()
		points := []timeValue{
			{Time: base, Value: 10},
			{Time: base.Add(time.Second), Value: 20},
			{Time: base.Add(2 * time.Second), Value: 30},
			{Time: base.Add(3 * time.Second), Value: 40},
		}

		result := downsample(points, 2, lastAggregator)
		assert.InDelta(t, 40.0, result[len(result)-1].Value, 0.01)
	})
}

func TestCPUAverageOverTime(t *testing.T) {
	t.Parallel()

	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	cpuMeasurements := map[int][]*monitor.CPUMeasurement{
		0: {
			{Time: base, UsedPercent: 80},
			{Time: base.Add(time.Second), UsedPercent: 60},
		},
		1: {
			{Time: base, UsedPercent: 40},
			{Time: base.Add(time.Second), UsedPercent: 20},
		},
	}

	result := cpuAverageOverTime(cpuMeasurements)
	require.Len(t, result, 2)
	assert.InDelta(t, 60.0, result[0].Value, 0.01)
	assert.InDelta(t, 40.0, result[1].Value, 0.01)
}

func TestSanitizeMermaidName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "hello", sanitizeMermaidName("hello"))
	assert.Equal(t, "has#colon;colons", sanitizeMermaidName("has:colons"))
	assert.Equal(t, "has#comma;commas", sanitizeMermaidName("has,commas"))
	assert.Len(t, sanitizeMermaidName(string(make([]byte, 100))), 80)
}

func TestConclusionToGanttStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "crit", conclusionToGanttStatus("failure"))
	assert.Equal(t, "done", conclusionToGanttStatus("cancelled"))
	assert.Empty(t, conclusionToGanttStatus("success"))
	assert.Empty(t, conclusionToGanttStatus(""))
}

func TestByteScale(t *testing.T) {
	t.Parallel()

	divisor, unit := byteScale(500)
	assert.InEpsilon(t, 1.0, divisor, 0.000001)
	assert.Equal(t, "B", unit)

	divisor, unit = byteScale(2048)
	assert.InEpsilon(t, 1024.0, divisor, 0.000001)
	assert.Equal(t, "KB", unit)

	divisor, unit = byteScale(5 * 1024 * 1024)
	assert.InEpsilon(t, float64(1024*1024), divisor, 0.000001)
	assert.Equal(t, "MB", unit)

	divisor, unit = byteScale(3 * 1024 * 1024 * 1024)
	assert.InEpsilon(t, float64(1024*1024*1024), divisor, 0.000001)
	assert.Equal(t, "GB", unit)

	divisor, unit = byteScale(0)
	assert.InEpsilon(t, 1.0, divisor, 0.000001)
	assert.Equal(t, "B", unit)
}

func TestBuildXYChartNumericAxis(t *testing.T) {
	t.Parallel()

	t.Run("short duration uses seconds", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		points := make([]timeValue, 50)
		for i := range points {
			points[i] = timeValue{
				Time:  base.Add(time.Duration(i) * time.Second),
				Value: float64(i),
			}
		}
		result := buildXYChart("Test", "Value", 0, 100, points)
		assert.Contains(t, result, `x-axis "Seconds" 0 --> 49`)
		assert.NotContains(t, result, `x-axis [`, "should not use categorical x-axis")
	})

	t.Run("long duration uses minutes", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		points := make([]timeValue, 50)
		for i := range points {
			points[i] = timeValue{
				Time:  base.Add(time.Duration(i) * 10 * time.Second),
				Value: float64(i),
			}
		}
		result := buildXYChart("Test", "Value", 0, 100, points)
		assert.Contains(t, result, `x-axis "Minutes" 0 -->`)
		assert.NotContains(t, result, `x-axis [`, "should not use categorical x-axis")
	})
}

func TestChartsFromLongMonitorData(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)
	dataFile := filepath.Join("testdata", "octometrics.monitor.long.testdata.jsonl")
	require.FileExists(t, dataFile)

	analysis, err := monitor.Analyze(log, dataFile)
	require.NoError(t, err)

	cpu := cpuChart(analysis)
	require.NotEmpty(t, cpu, "CPU chart should not be empty")
	assert.Contains(t, cpu, `x-axis "Minutes" 0 -->`, "multi-minute workflow should use Minutes x-axis")
	assert.NotContains(t, cpu, `x-axis [`, "should not use categorical x-axis")
	assert.Contains(t, cpu, "line [")

	mem := memoryChart(analysis)
	require.NotEmpty(t, mem, "memory chart should not be empty")
	assert.Contains(t, mem, `x-axis "Minutes" 0 -->`)

	disk := diskChart(analysis)
	require.NotEmpty(t, disk, "disk chart should not be empty")
	assert.Contains(t, disk, `x-axis "Minutes" 0 -->`)

	io := ioChart(analysis)
	require.NotEmpty(t, io, "IO chart should not be empty")
	assert.Contains(t, io, `x-axis "Minutes" 0 -->`)
}
