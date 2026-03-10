package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"

	"github.com/kalverra/octometrics/monitor"
)

const defaultTargetPoints = 500

// timeValue is a single data point with a timestamp.
type timeValue struct {
	Time  time.Time
	Value float64
}

// ganttChart builds a Mermaid gantt chart from GitHub Actions job steps.
func ganttChart(steps []*github.TaskStep) string {
	if len(steps) == 0 {
		return ""
	}

	// Collect valid steps (non-skipped, with timing data).
	type ganttItem struct {
		name       string
		conclusion string
		number     int64
		relStart   time.Duration
		dur        time.Duration
	}

	var earliest time.Time
	for _, s := range steps {
		if s.StartedAt == nil || s.CompletedAt == nil {
			continue
		}
		if earliest.IsZero() || s.StartedAt.Before(earliest) {
			earliest = s.StartedAt.Time
		}
	}
	if earliest.IsZero() {
		return ""
	}

	var items []ganttItem
	var latest time.Time
	for _, s := range steps {
		if s.StartedAt == nil || s.CompletedAt == nil {
			continue
		}
		dur := s.CompletedAt.Sub(s.StartedAt.Time)
		if dur.Seconds() == 0 || s.GetConclusion() == "skipped" {
			continue
		}
		if latest.IsZero() || s.CompletedAt.After(latest) {
			latest = s.CompletedAt.Time
		}
		items = append(items, ganttItem{
			name:       s.GetName(),
			conclusion: s.GetConclusion(),
			number:     s.GetNumber(),
			relStart:   s.StartedAt.Sub(earliest),
			dur:        dur,
		})
	}
	if len(items) == 0 {
		return ""
	}

	totalDuration := latest.Sub(earliest)
	dateFormat := "mm:ss"
	axisFormat := "%M:%S"
	goFmt := "04:05"
	if totalDuration >= time.Hour {
		dateFormat = "HH:mm:ss"
		axisFormat = "%H:%M:%S"
		goFmt = "15:04:05"
	}

	// Use midnight as the zero reference so relative offsets display correctly.
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	var b strings.Builder
	b.WriteString("```mermaid\ngantt\n")
	fmt.Fprintf(&b, "    dateFormat %s\n", dateFormat)
	fmt.Fprintf(&b, "    axisFormat %s\n", axisFormat)

	for _, item := range items {
		displayStart := base.Add(item.relStart)
		status := conclusionToGanttStatus(item.conclusion)
		name := sanitizeMermaidName(item.name)

		if status != "" {
			fmt.Fprintf(&b, "    %s :%s, step%d, %s, %.0fs\n",
				name, status, item.number, displayStart.Format(goFmt), item.dur.Seconds())
		} else {
			fmt.Fprintf(&b, "    %s :step%d, %s, %.0fs\n",
				name, item.number, displayStart.Format(goFmt), item.dur.Seconds())
		}
	}

	b.WriteString("```\n")
	return b.String()
}

// cpuChart builds a Mermaid xychart-beta line chart for average CPU usage.
func cpuChart(analysis *monitor.Analysis) string {
	if len(analysis.CPUMeasurements) == 0 {
		return ""
	}

	points := cpuAverageOverTime(analysis.CPUMeasurements)
	if len(points) == 0 {
		return ""
	}

	downsampled := downsample(points, defaultTargetPoints, maxAggregator)
	return buildXYChart("CPU Usage (%)", "Usage %", 0, 100, downsampled)
}

// memoryChart builds a Mermaid xychart-beta line chart for memory usage.
func memoryChart(analysis *monitor.Analysis) string {
	if len(analysis.MemoryMeasurements) == 0 {
		return ""
	}

	var maxRaw uint64
	if analysis.SystemInfo != nil && analysis.SystemInfo.Memory != nil && analysis.SystemInfo.Memory.Total > 0 {
		maxRaw = analysis.SystemInfo.Memory.Total
	} else {
		for _, m := range analysis.MemoryMeasurements {
			if m.Used > maxRaw {
				maxRaw = m.Used
			}
		}
	}

	divisor, unit := byteScale(maxRaw)
	points := make([]timeValue, len(analysis.MemoryMeasurements))
	for i, m := range analysis.MemoryMeasurements {
		points[i] = timeValue{Time: m.Time, Value: float64(m.Used) / divisor}
	}

	maxY := float64(maxRaw) / divisor
	if analysis.SystemInfo == nil || analysis.SystemInfo.Memory == nil || analysis.SystemInfo.Memory.Total == 0 {
		maxY *= 1.1
	}

	downsampled := downsample(points, defaultTargetPoints, lastAggregator)
	return buildXYChart(fmt.Sprintf("Memory Usage (%s)", unit), unit, 0, maxY, downsampled)
}

// diskChart builds a Mermaid xychart-beta line chart for disk usage.
func diskChart(analysis *monitor.Analysis) string {
	if len(analysis.DiskMeasurements) == 0 {
		return ""
	}

	var maxRaw uint64
	if analysis.SystemInfo != nil && analysis.SystemInfo.Disk != nil && analysis.SystemInfo.Disk.Total > 0 {
		maxRaw = analysis.SystemInfo.Disk.Total
	} else {
		for _, m := range analysis.DiskMeasurements {
			if m.Used > maxRaw {
				maxRaw = m.Used
			}
		}
	}

	divisor, unit := byteScale(maxRaw)
	points := make([]timeValue, len(analysis.DiskMeasurements))
	for i, m := range analysis.DiskMeasurements {
		points[i] = timeValue{Time: m.Time, Value: float64(m.Used) / divisor}
	}

	maxY := float64(maxRaw) / divisor
	if analysis.SystemInfo == nil || analysis.SystemInfo.Disk == nil || analysis.SystemInfo.Disk.Total == 0 {
		maxY *= 1.1
	}

	downsampled := downsample(points, defaultTargetPoints, lastAggregator)
	return buildXYChart(fmt.Sprintf("Disk Usage (%s)", unit), unit, 0, maxY, downsampled)
}

// ioChart builds Mermaid xychart-beta line charts for network I/O.
// Returns two charts: one for bytes sent and one for bytes received.
// Each chart independently selects the best unit (B, KB, MB, GB).
func ioChart(analysis *monitor.Analysis) string {
	if len(analysis.IOMeasurements) == 0 {
		return ""
	}

	var maxRawSent, maxRawRecv uint64
	for _, m := range analysis.IOMeasurements {
		if m.BytesSent > maxRawSent {
			maxRawSent = m.BytesSent
		}
		if m.BytesRecv > maxRawRecv {
			maxRawRecv = m.BytesRecv
		}
	}

	var b strings.Builder

	if maxRawSent > 0 {
		sentDiv, sentUnit := byteScale(maxRawSent)
		sent := make([]timeValue, len(analysis.IOMeasurements))
		for i, m := range analysis.IOMeasurements {
			sent[i] = timeValue{Time: m.Time, Value: float64(m.BytesSent) / sentDiv}
		}
		dsSent := downsample(sent, defaultTargetPoints, lastAggregator)
		maxSent := float64(maxRawSent) / sentDiv
		b.WriteString(buildXYChart(fmt.Sprintf("Network Sent (%s)", sentUnit), sentUnit, 0, maxSent*1.1, dsSent))
	}

	if maxRawRecv > 0 {
		recvDiv, recvUnit := byteScale(maxRawRecv)
		recv := make([]timeValue, len(analysis.IOMeasurements))
		for i, m := range analysis.IOMeasurements {
			recv[i] = timeValue{Time: m.Time, Value: float64(m.BytesRecv) / recvDiv}
		}
		dsRecv := downsample(recv, defaultTargetPoints, lastAggregator)
		maxRecv := float64(maxRawRecv) / recvDiv
		b.WriteString(buildXYChart(fmt.Sprintf("Network Received (%s)", recvUnit), recvUnit, 0, maxRecv*1.1, dsRecv))
	}

	return b.String()
}

// cpuAverageOverTime computes the average CPU usage across all cores at each time point.
func cpuAverageOverTime(cpuMeasurements map[int][]*monitor.CPUMeasurement) []timeValue {
	// Find the CPU with the most measurements as the reference timeline.
	var refCPU int
	var maxLen int
	for cpuNum, measurements := range cpuMeasurements {
		if len(measurements) > maxLen {
			maxLen = len(measurements)
			refCPU = cpuNum
		}
	}
	if maxLen == 0 {
		return nil
	}

	numCPUs := float64(len(cpuMeasurements))
	result := make([]timeValue, maxLen)
	for i := 0; i < maxLen; i++ {
		result[i].Time = cpuMeasurements[refCPU][i].Time
		var sum float64
		for _, measurements := range cpuMeasurements {
			if i < len(measurements) {
				sum += measurements[i].UsedPercent
			}
		}
		result[i].Value = sum / numCPUs
	}
	return result
}

type aggregatorFunc func(bucket []timeValue) float64

func maxAggregator(bucket []timeValue) float64 {
	m := bucket[0].Value
	for _, v := range bucket[1:] {
		if v.Value > m {
			m = v.Value
		}
	}
	return m
}

func lastAggregator(bucket []timeValue) float64 {
	return bucket[len(bucket)-1].Value
}

// downsample reduces a slice of timeValue to at most targetPoints using the given aggregation.
func downsample(points []timeValue, targetPoints int, agg aggregatorFunc) []timeValue {
	if len(points) <= targetPoints {
		return points
	}

	bucketSize := float64(len(points)) / float64(targetPoints)
	result := make([]timeValue, 0, targetPoints)

	for i := range targetPoints {
		start := int(math.Round(float64(i) * bucketSize))
		end := min(int(math.Round(float64(i+1)*bucketSize)), len(points))
		if start >= end {
			continue
		}
		bucket := points[start:end]
		mid := start + len(bucket)/2
		result = append(result, timeValue{
			Time:  points[mid].Time,
			Value: agg(bucket),
		})
	}

	return result
}

// buildXYChart produces a Mermaid xychart-beta code block.
func buildXYChart(title, yLabel string, yMin, yMax float64, points []timeValue) string {
	if len(points) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("```mermaid\nxychart-beta\n")
	fmt.Fprintf(&b, "    title %q\n", title)

	elapsed := points[len(points)-1].Time.Sub(points[0].Time)
	if elapsed >= 2*time.Minute {
		fmt.Fprintf(&b, "    x-axis \"Minutes\" 0 --> %.0f\n", math.Ceil(elapsed.Minutes()))
	} else {
		fmt.Fprintf(&b, "    x-axis \"Seconds\" 0 --> %.0f\n", math.Ceil(elapsed.Seconds()))
	}
	fmt.Fprintf(&b, "    y-axis %q %.0f --> %.0f\n", yLabel, yMin, math.Ceil(yMax))

	values := make([]string, len(points))
	for i, p := range points {
		values[i] = fmt.Sprintf("%.1f", p.Value)
	}
	fmt.Fprintf(&b, "    line [%s]\n", strings.Join(values, ", "))

	b.WriteString("```\n")
	return b.String()
}

// conclusionToGanttStatus maps a GitHub conclusion to a Mermaid Gantt status keyword.
func conclusionToGanttStatus(conclusion string) string {
	switch conclusion {
	case "failure":
		return "crit"
	case "cancelled":
		return "done"
	default:
		return ""
	}
}

// byteScale returns the best divisor and unit label for displaying byte values
// based on the maximum value in the dataset.
func byteScale(maxBytes uint64) (divisor float64, unit string) {
	switch {
	case maxBytes >= 1024*1024*1024:
		return 1024 * 1024 * 1024, "GB"
	case maxBytes >= 1024*1024:
		return 1024 * 1024, "MB"
	case maxBytes >= 1024:
		return 1024, "KB"
	default:
		return 1, "B"
	}
}

// sanitizeMermaidName cleans a name for safe use in Mermaid syntax.
func sanitizeMermaidName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	s = strings.ReplaceAll(s, ":", "#colon;")
	s = strings.ReplaceAll(s, ",", "#comma;")
	return s
}
