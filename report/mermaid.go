package report

import (
	"fmt"
	"math"
	"sort"
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

// MonitoringChart is one Mermaid xychart-beta diagram for HTML embedding (no markdown fences).
type MonitoringChart struct {
	Title   string
	Diagram string
}

// MonitoringMermaidCharts returns the same xychart-beta diagrams used in GitHub step summaries and PR comments.
func MonitoringMermaidCharts(analysis *monitor.Analysis) []MonitoringChart {
	if analysis == nil {
		return nil
	}
	var charts []MonitoringChart
	if d := cpuChartDiagram(analysis); d != "" {
		charts = append(charts, MonitoringChart{Title: "CPU Usage", Diagram: d})
	}
	charts = append(charts, cpuPerCoreChartDiagrams(analysis)...)
	if d := memoryChartDiagram(analysis); d != "" {
		charts = append(charts, MonitoringChart{Title: "Memory Usage", Diagram: d})
	}
	if d := diskChartDiagram(analysis); d != "" {
		charts = append(charts, MonitoringChart{Title: "Disk Usage", Diagram: d})
	}
	charts = append(charts, ioChartDiagrams(analysis)...)
	if len(charts) == 0 {
		return nil
	}
	return charts
}

// MonitoringMermaidChartsWithWindow builds xychart-beta diagrams whose x-axis is elapsed time from
// windowStart through windowEnd (extended to include any samples after windowEnd). Used by observe
// so metric charts align with the job Gantt timeline.
func MonitoringMermaidChartsWithWindow(analysis *monitor.Analysis, windowStart, windowEnd time.Time) []MonitoringChart {
	if analysis == nil || !windowEnd.After(windowStart) {
		return nil
	}
	axisEnd := maxTime(windowEnd, latestMetricTime(analysis))
	if !axisEnd.After(windowStart) {
		return nil
	}
	var charts []MonitoringChart
	if d := cpuChartDiagramWindowed(analysis, windowStart, axisEnd); d != "" {
		charts = append(charts, MonitoringChart{Title: "CPU Usage", Diagram: d})
	}
	charts = append(charts, cpuPerCoreChartDiagramsWindowed(analysis, windowStart, axisEnd)...)
	if d := memoryChartDiagramWindowed(analysis, windowStart, axisEnd); d != "" {
		charts = append(charts, MonitoringChart{Title: "Memory Usage", Diagram: d})
	}
	if d := diskChartDiagramWindowed(analysis, windowStart, axisEnd); d != "" {
		charts = append(charts, MonitoringChart{Title: "Disk Usage", Diagram: d})
	}
	charts = append(charts, ioChartDiagramsWindowed(analysis, windowStart, axisEnd)...)
	if len(charts) == 0 {
		return nil
	}
	return charts
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func latestMetricTime(a *monitor.Analysis) time.Time {
	var latest time.Time
	for _, series := range a.CPUMeasurements {
		for _, p := range series {
			if p != nil && p.Time.After(latest) {
				latest = p.Time
			}
		}
	}
	for _, p := range a.MemoryMeasurements {
		if p != nil && p.Time.After(latest) {
			latest = p.Time
		}
	}
	for _, p := range a.DiskMeasurements {
		if p != nil && p.Time.After(latest) {
			latest = p.Time
		}
	}
	for _, p := range a.IOMeasurements {
		if p != nil && p.Time.After(latest) {
			latest = p.Time
		}
	}
	return latest
}

// cpuChart builds a Mermaid xychart-beta line chart for average CPU usage.
func cpuChart(analysis *monitor.Analysis) string {
	d := cpuChartDiagram(analysis)
	if d == "" {
		return ""
	}
	return markdownMermaidBlock(d)
}

func cpuChartDiagram(analysis *monitor.Analysis) string {
	if len(analysis.CPUMeasurements) == 0 {
		return ""
	}

	points := cpuAverageOverTime(analysis.CPUMeasurements)
	if len(points) == 0 {
		return ""
	}

	downsampled := downsample(points, defaultTargetPoints, maxAggregator)
	return buildXYChartDiagram("CPU Usage (%)", "Usage %", 0, 100, downsampled)
}

func cpuChartDiagramWindowed(analysis *monitor.Analysis, windowStart, axisEnd time.Time) string {
	if len(analysis.CPUMeasurements) == 0 {
		return ""
	}
	points := cpuAverageOverTime(analysis.CPUMeasurements)
	if len(points) == 0 {
		return ""
	}
	downsampled := downsample(points, defaultTargetPoints, maxAggregator)
	return buildXYChartDiagramWindowed("CPU Usage (%)", "Usage %", 0, 100, downsampled, windowStart, axisEnd)
}

func sortedCPUNums(m map[int][]*monitor.CPUMeasurement) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func cpuCoreTimeValues(measurements []*monitor.CPUMeasurement) []timeValue {
	if len(measurements) == 0 {
		return nil
	}
	points := make([]timeValue, 0, len(measurements))
	for _, m := range measurements {
		if m == nil {
			continue
		}
		points = append(points, timeValue{Time: m.Time, Value: m.UsedPercent})
	}
	return points
}

// cpuPerCoreChartDiagrams returns one chart per CPU when there are multiple cores; skipped for a single series.
func cpuPerCoreChartDiagrams(analysis *monitor.Analysis) []MonitoringChart {
	if analysis == nil || len(analysis.CPUMeasurements) <= 1 {
		return nil
	}
	var charts []MonitoringChart
	for _, cpuNum := range sortedCPUNums(analysis.CPUMeasurements) {
		points := cpuCoreTimeValues(analysis.CPUMeasurements[cpuNum])
		if len(points) == 0 {
			continue
		}
		downsampled := downsample(points, defaultTargetPoints, maxAggregator)
		title := fmt.Sprintf("CPU %d Usage (%%)", cpuNum)
		d := buildXYChartDiagram(title, "Usage %", 0, 100, downsampled)
		if d == "" {
			continue
		}
		charts = append(charts, MonitoringChart{Title: title, Diagram: d})
	}
	return charts
}

func cpuPerCoreChartDiagramsWindowed(
	analysis *monitor.Analysis,
	windowStart, axisEnd time.Time,
) []MonitoringChart {
	if analysis == nil || len(analysis.CPUMeasurements) <= 1 || !axisEnd.After(windowStart) {
		return nil
	}
	var charts []MonitoringChart
	for _, cpuNum := range sortedCPUNums(analysis.CPUMeasurements) {
		points := cpuCoreTimeValues(analysis.CPUMeasurements[cpuNum])
		if len(points) == 0 {
			continue
		}
		downsampled := downsample(points, defaultTargetPoints, maxAggregator)
		title := fmt.Sprintf("CPU %d Usage (%%)", cpuNum)
		d := buildXYChartDiagramWindowed(title, "Usage %", 0, 100, downsampled, windowStart, axisEnd)
		if d == "" {
			continue
		}
		charts = append(charts, MonitoringChart{Title: title, Diagram: d})
	}
	return charts
}

// memoryChart builds a Mermaid xychart-beta line chart for memory usage.
func memoryChart(analysis *monitor.Analysis) string {
	d := memoryChartDiagram(analysis)
	if d == "" {
		return ""
	}
	return markdownMermaidBlock(d)
}

func memoryChartDiagram(analysis *monitor.Analysis) string {
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
	return buildXYChartDiagram(fmt.Sprintf("Memory Usage (%s)", unit), unit, 0, maxY, downsampled)
}

func memoryChartDiagramWindowed(analysis *monitor.Analysis, windowStart, axisEnd time.Time) string {
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
	return buildXYChartDiagramWindowed(
		fmt.Sprintf("Memory Usage (%s)", unit),
		unit,
		0,
		maxY,
		downsampled,
		windowStart,
		axisEnd,
	)
}

// diskChart builds a Mermaid xychart-beta line chart for disk usage.
func diskChart(analysis *monitor.Analysis) string {
	d := diskChartDiagram(analysis)
	if d == "" {
		return ""
	}
	return markdownMermaidBlock(d)
}

func diskChartDiagram(analysis *monitor.Analysis) string {
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
	return buildXYChartDiagram(fmt.Sprintf("Disk Usage (%s)", unit), unit, 0, maxY, downsampled)
}

func diskChartDiagramWindowed(analysis *monitor.Analysis, windowStart, axisEnd time.Time) string {
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
	return buildXYChartDiagramWindowed(
		fmt.Sprintf("Disk Usage (%s)", unit),
		unit,
		0,
		maxY,
		downsampled,
		windowStart,
		axisEnd,
	)
}

// ioChart builds Mermaid xychart-beta line charts for network I/O.
// Returns two charts: one for bytes sent and one for bytes received.
// Each chart independently selects the best unit (B, KB, MB, GB).
func ioChart(analysis *monitor.Analysis) string {
	var b strings.Builder
	for _, c := range ioChartDiagrams(analysis) {
		b.WriteString(markdownMermaidBlock(c.Diagram))
	}
	return b.String()
}

func ioChartDiagrams(analysis *monitor.Analysis) []MonitoringChart {
	if len(analysis.IOMeasurements) == 0 {
		return nil
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

	var charts []MonitoringChart

	if maxRawSent > 0 {
		sentDiv, sentUnit := byteScale(maxRawSent)
		sent := make([]timeValue, len(analysis.IOMeasurements))
		for i, m := range analysis.IOMeasurements {
			sent[i] = timeValue{Time: m.Time, Value: float64(m.BytesSent) / sentDiv}
		}
		dsSent := downsample(sent, defaultTargetPoints, lastAggregator)
		maxSent := float64(maxRawSent) / sentDiv
		d := buildXYChartDiagram(fmt.Sprintf("Network Sent (%s)", sentUnit), sentUnit, 0, maxSent*1.1, dsSent)
		if d != "" {
			charts = append(charts, MonitoringChart{Title: "Network Sent", Diagram: d})
		}
	}

	if maxRawRecv > 0 {
		recvDiv, recvUnit := byteScale(maxRawRecv)
		recv := make([]timeValue, len(analysis.IOMeasurements))
		for i, m := range analysis.IOMeasurements {
			recv[i] = timeValue{Time: m.Time, Value: float64(m.BytesRecv) / recvDiv}
		}
		dsRecv := downsample(recv, defaultTargetPoints, lastAggregator)
		maxRecv := float64(maxRawRecv) / recvDiv
		d := buildXYChartDiagram(fmt.Sprintf("Network Received (%s)", recvUnit), recvUnit, 0, maxRecv*1.1, dsRecv)
		if d != "" {
			charts = append(charts, MonitoringChart{Title: "Network Received", Diagram: d})
		}
	}

	return charts
}

func ioChartDiagramsWindowed(analysis *monitor.Analysis, windowStart, axisEnd time.Time) []MonitoringChart {
	if len(analysis.IOMeasurements) == 0 {
		return nil
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
	var charts []MonitoringChart
	if maxRawSent > 0 {
		sentDiv, sentUnit := byteScale(maxRawSent)
		sent := make([]timeValue, len(analysis.IOMeasurements))
		for i, m := range analysis.IOMeasurements {
			sent[i] = timeValue{Time: m.Time, Value: float64(m.BytesSent) / sentDiv}
		}
		dsSent := downsample(sent, defaultTargetPoints, lastAggregator)
		maxSent := float64(maxRawSent) / sentDiv
		d := buildXYChartDiagramWindowed(
			fmt.Sprintf("Network Sent (%s)", sentUnit),
			sentUnit,
			0,
			maxSent*1.1,
			dsSent,
			windowStart,
			axisEnd,
		)
		if d != "" {
			charts = append(charts, MonitoringChart{Title: "Network Sent", Diagram: d})
		}
	}
	if maxRawRecv > 0 {
		recvDiv, recvUnit := byteScale(maxRawRecv)
		recv := make([]timeValue, len(analysis.IOMeasurements))
		for i, m := range analysis.IOMeasurements {
			recv[i] = timeValue{Time: m.Time, Value: float64(m.BytesRecv) / recvDiv}
		}
		dsRecv := downsample(recv, defaultTargetPoints, lastAggregator)
		maxRecv := float64(maxRawRecv) / recvDiv
		d := buildXYChartDiagramWindowed(
			fmt.Sprintf("Network Received (%s)", recvUnit),
			recvUnit,
			0,
			maxRecv*1.1,
			dsRecv,
			windowStart,
			axisEnd,
		)
		if d != "" {
			charts = append(charts, MonitoringChart{Title: "Network Received", Diagram: d})
		}
	}
	return charts
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

func markdownMermaidBlock(diagram string) string {
	return "```mermaid\n" + diagram + "```\n"
}

// buildXYChartDiagram returns Mermaid xychart-beta source without markdown fences.
func buildXYChartDiagram(title, yLabel string, yMin, yMax float64, points []timeValue) string {
	if len(points) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("xychart-beta\n")
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

	return b.String()
}

func interpolateAt(points []timeValue, t time.Time) float64 {
	if len(points) == 0 {
		return 0
	}
	if !t.After(points[0].Time) {
		return points[0].Value
	}
	last := points[len(points)-1]
	if !t.Before(last.Time) {
		return last.Value
	}
	for i := 0; i < len(points)-1; i++ {
		a, b := points[i], points[i+1]
		if !t.Before(a.Time) && !t.After(b.Time) {
			if b.Time.Equal(a.Time) {
				return a.Value
			}
			w := float64(t.Sub(a.Time)) / float64(b.Time.Sub(a.Time))
			return a.Value + w*(b.Value-a.Value)
		}
	}
	return last.Value
}

func resampleLineValues(points []timeValue, t0, t1 time.Time, n int) []string {
	if n < 2 {
		n = 2
	}
	duration := t1.Sub(t0)
	values := make([]string, n)
	for i := range n {
		frac := float64(i) / float64(n-1)
		t := t0.Add(time.Duration(float64(duration) * frac))
		v := interpolateAt(points, t)
		values[i] = fmt.Sprintf("%.1f", v)
	}
	return values
}

// buildXYChartDiagramWindowed maps evenly spaced xychart line points to elapsed time in [windowStart, axisEnd].
func buildXYChartDiagramWindowed(
	title, yLabel string,
	yMin, yMax float64,
	points []timeValue,
	windowStart, axisEnd time.Time,
) string {
	if len(points) == 0 || !axisEnd.After(windowStart) {
		return ""
	}
	duration := axisEnd.Sub(windowStart)
	var xMax float64
	if duration >= 2*time.Minute {
		xMax = math.Ceil(duration.Minutes())
		if xMax < 1 {
			xMax = 1
		}
	} else {
		xMax = math.Ceil(duration.Seconds())
		if xMax < 1 {
			xMax = 1
		}
	}
	n := max(len(points), 2)
	valueStrs := resampleLineValues(points, windowStart, axisEnd, n)

	var b strings.Builder
	b.WriteString("xychart-beta\n")
	fmt.Fprintf(&b, "    title %q\n", title)
	if duration >= 2*time.Minute {
		fmt.Fprintf(&b, "    x-axis \"Minutes\" 0 --> %.0f\n", xMax)
	} else {
		fmt.Fprintf(&b, "    x-axis \"Seconds\" 0 --> %.0f\n", xMax)
	}
	fmt.Fprintf(&b, "    y-axis %q %.0f --> %.0f\n", yLabel, yMin, math.Ceil(yMax))
	fmt.Fprintf(&b, "    line [%s]\n", strings.Join(valueStrs, ", "))

	return b.String()
}

// buildXYChart produces a Mermaid xychart-beta code block for Markdown.
func buildXYChart(title, yLabel string, yMin, yMax float64, points []timeValue) string {
	d := buildXYChartDiagram(title, yLabel, yMin, yMax, points)
	if d == "" {
		return ""
	}
	return markdownMermaidBlock(d)
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
