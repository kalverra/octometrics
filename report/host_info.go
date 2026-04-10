package report

import (
	"fmt"
	"strings"

	"github.com/kalverra/octometrics/monitor"
)

// machineInfoMarkdown returns a "### Host" section with CPU/memory/disk from analysis, or empty if none.
func machineInfoMarkdown(analysis *monitor.Analysis) string {
	if analysis == nil || analysis.SystemInfo == nil {
		return ""
	}
	si := analysis.SystemInfo
	var lines []string

	if len(si.CPU) > 0 {
		line := fmt.Sprintf("- **CPU:** %d logical processor(s)", len(si.CPU))
		if m := si.CPU[0]; m.Model != "" {
			line += fmt.Sprintf(", %s", m.Model)
		}
		if m := si.CPU[0]; m.Mhz > 0 {
			line += fmt.Sprintf(" (~%.0f MHz)", m.Mhz)
		}
		lines = append(lines, line)
	}
	if si.Memory != nil && si.Memory.Total > 0 {
		div, unit := byteScale(si.Memory.Total)
		lines = append(lines, fmt.Sprintf("- **RAM:** %.1f %s", float64(si.Memory.Total)/div, unit))
	}
	if si.Disk != nil && si.Disk.Total > 0 {
		div, unit := byteScale(si.Disk.Total)
		lines = append(lines, fmt.Sprintf("- **Disk:** %.1f %s total", float64(si.Disk.Total)/div, unit))
	}

	if len(lines) == 0 {
		return ""
	}
	return "### Host\n\n" + strings.Join(lines, "\n") + "\n\n"
}
