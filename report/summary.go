package report

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/google/go-github/v84/github"

	"github.com/kalverra/octometrics/monitor"
)

// buildReport assembles the full markdown report from analysis data and optional job steps.
func buildReport(analysis *monitor.Analysis, steps []*github.TaskStep, _ *ghaContext) string {
	var b strings.Builder

	title := "Octometrics Report"
	if analysis.JobName != "" {
		title = fmt.Sprintf("Octometrics — %s", analysis.JobName)
	}
	fmt.Fprintf(&b, "## %s\n\n", title)

	if gantt := ganttChart(steps); gantt != "" {
		b.WriteString("### Step Timeline\n\n")
		b.WriteString(gantt)
		b.WriteString("\n")
	}

	if chart := cpuChart(analysis); chart != "" {
		b.WriteString("### CPU Usage\n\n")
		b.WriteString(chart)
		b.WriteString("\n")
	}

	if chart := memoryChart(analysis); chart != "" {
		b.WriteString("### Memory Usage\n\n")
		b.WriteString(chart)
		b.WriteString("\n")
	}

	if chart := diskChart(analysis); chart != "" {
		b.WriteString("### Disk Usage\n\n")
		b.WriteString(chart)
		b.WriteString("\n")
	}

	if chart := ioChart(analysis); chart != "" {
		b.WriteString("### Network I/O\n\n")
		b.WriteString(chart)
		b.WriteString("\n")
	}

	if table := metricSummaryTable(analysis); table != "" {
		b.WriteString("### Resource Summary\n\n")
		b.WriteString(table)
		b.WriteString("\n")
	}

	return b.String()
}

// metricSummaryTable produces a markdown table with peak and average values.
func metricSummaryTable(analysis *monitor.Analysis) string {
	var rows []string

	if cpuRow := cpuSummaryRow(analysis); cpuRow != "" {
		rows = append(rows, cpuRow)
	}
	if memRow := memorySummaryRow(analysis); memRow != "" {
		rows = append(rows, memRow)
	}
	if diskRow := diskSummaryRow(analysis); diskRow != "" {
		rows = append(rows, diskRow)
	}
	sentRow, recvRow := ioSummaryRows(analysis)
	if sentRow != "" {
		rows = append(rows, sentRow)
	}
	if recvRow != "" {
		rows = append(rows, recvRow)
	}

	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("| Metric | Peak | Average |\n")
	b.WriteString("|--------|------|---------|\n")
	for _, row := range rows {
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func cpuSummaryRow(analysis *monitor.Analysis) string {
	if len(analysis.CPUMeasurements) == 0 {
		return ""
	}

	points := cpuAverageOverTime(analysis.CPUMeasurements)
	if len(points) == 0 {
		return ""
	}

	var peak, sum float64
	for _, p := range points {
		if p.Value > peak {
			peak = p.Value
		}
		sum += p.Value
	}
	avg := sum / float64(len(points))

	return fmt.Sprintf("| CPU | %.1f%% | %.1f%% |", peak, avg)
}

func memorySummaryRow(analysis *monitor.Analysis) string {
	if len(analysis.MemoryMeasurements) == 0 {
		return ""
	}

	var peakUsed uint64
	var sumUsed float64
	for _, m := range analysis.MemoryMeasurements {
		if m.Used > peakUsed {
			peakUsed = m.Used
		}
		sumUsed += float64(m.Used)
	}
	avgUsed := sumUsed / float64(len(analysis.MemoryMeasurements))

	peakGB := float64(peakUsed) / 1024 / 1024 / 1024
	avgGB := avgUsed / 1024 / 1024 / 1024

	if analysis.SystemInfo != nil && analysis.SystemInfo.Memory != nil && analysis.SystemInfo.Memory.Total > 0 {
		totalGB := float64(analysis.SystemInfo.Memory.Total) / 1024 / 1024 / 1024
		peakPct := (peakGB / totalGB) * 100
		avgPct := (avgGB / totalGB) * 100
		return fmt.Sprintf("| Memory | %.1f / %.1f GB (%.1f%%) | %.1f GB (%.1f%%) |",
			peakGB, totalGB, peakPct, avgGB, avgPct)
	}

	return fmt.Sprintf("| Memory | %.1f GB | %.1f GB |", peakGB, avgGB)
}

func diskSummaryRow(analysis *monitor.Analysis) string {
	if len(analysis.DiskMeasurements) == 0 {
		return ""
	}

	var peakUsed uint64
	var sumUsed float64
	for _, m := range analysis.DiskMeasurements {
		if m.Used > peakUsed {
			peakUsed = m.Used
		}
		sumUsed += float64(m.Used)
	}
	avgUsed := sumUsed / float64(len(analysis.DiskMeasurements))

	peakGB := float64(peakUsed) / 1024 / 1024 / 1024
	avgGB := avgUsed / 1024 / 1024 / 1024

	if analysis.SystemInfo != nil && analysis.SystemInfo.Disk != nil && analysis.SystemInfo.Disk.Total > 0 {
		totalGB := float64(analysis.SystemInfo.Disk.Total) / 1024 / 1024 / 1024
		peakPct := (peakGB / totalGB) * 100
		avgPct := (avgGB / totalGB) * 100
		return fmt.Sprintf("| Disk | %.1f / %.1f GB (%.1f%%) | %.1f GB (%.1f%%) |",
			peakGB, totalGB, peakPct, avgGB, avgPct)
	}

	return fmt.Sprintf("| Disk | %.1f GB | %.1f GB |", peakGB, avgGB)
}

func ioSummaryRows(analysis *monitor.Analysis) (sentRow, recvRow string) {
	if len(analysis.IOMeasurements) == 0 {
		return "", ""
	}

	var maxSent, maxRecv uint64
	for _, m := range analysis.IOMeasurements {
		if m.BytesSent > maxSent {
			maxSent = m.BytesSent
		}
		if m.BytesRecv > maxRecv {
			maxRecv = m.BytesRecv
		}
	}

	sentRow = fmt.Sprintf("| Net Sent | %s total | — |", formatBytes(maxSent))
	recvRow = fmt.Sprintf("| Net Recv | %s total | — |", formatBytes(maxRecv))
	return sentRow, recvRow
}

func formatBytes(b uint64) string {
	gb := float64(b) / 1024 / 1024 / 1024
	if gb >= 1 {
		return fmt.Sprintf("%.1f GB", gb)
	}
	mb := float64(b) / 1024 / 1024
	if mb >= 1 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	kb := float64(b) / 1024
	if kb >= 1 {
		return fmt.Sprintf("%.1f KB", math.Ceil(kb))
	}
	return fmt.Sprintf("%d B", b)
}

// writeSummary appends the markdown report to the GITHUB_STEP_SUMMARY file.
func writeSummary(summaryPath, markdown string) (err error) {
	if summaryPath == "" {
		return fmt.Errorf("github_step_summary path is empty")
	}

	//nolint:gosec // GHA controls this path
	f, err := os.OpenFile(
		summaryPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0600,
	) //nolint:gosec // GHA controls this path
	if err != nil {
		return fmt.Errorf("failed to open step summary file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = fmt.Errorf("failed to close step summary file: %w", cerr)
		}
	}()

	if _, err = f.WriteString(markdown); err != nil {
		return fmt.Errorf("failed to write step summary: %w", err)
	}

	return nil
}
