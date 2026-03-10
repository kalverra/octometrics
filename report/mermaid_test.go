package report

import (
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		assert.Contains(t, result, "Memory Usage")
		assert.Contains(t, result, "xychart-beta")
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
	assert.Contains(t, result, "Disk Usage")
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
	assert.Contains(t, result, "Network Sent")
	assert.Contains(t, result, "Network Received")
}

func TestDownsample(t *testing.T) {
	t.Parallel()

	t.Run("fewer points than target returns all", func(t *testing.T) {
		t.Parallel()
		points := []timeValue{
			{Time: time.Now(), Value: 1},
			{Time: time.Now(), Value: 2},
		}
		result := downsample(points, 40, maxAggregator)
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
