package monitor

import (
	"fmt"
	"os"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/net"
)

func defaultDiskPath() string {
	if workspace := os.Getenv("GITHUB_WORKSPACE"); workspace != "" {
		return workspace
	}
	return "/"
}

func cpuPercentsFromDelta(prev, curr []cpu.TimesStat) ([]float64, error) {
	if len(prev) == 0 {
		return nil, nil
	}
	if len(prev) != len(curr) {
		return nil, fmt.Errorf("cpu count changed from %d to %d", len(prev), len(curr))
	}

	percents := make([]float64, len(curr))
	for i := range curr {
		percents[i] = cpuBusyPercent(prev[i], curr[i])
	}
	return percents, nil
}

func cpuBusyPercent(prev, curr cpu.TimesStat) float64 {
	totalDelta := timesStatTotal(curr) - timesStatTotal(prev)
	if totalDelta == 0 {
		return 0
	}
	idleDelta := curr.Idle - prev.Idle
	return 100 * (totalDelta - idleDelta) / totalDelta
}

func timesStatTotal(stat cpu.TimesStat) float64 {
	return stat.User + stat.System + stat.Idle + stat.Nice + stat.Iowait +
		stat.Irq + stat.Softirq + stat.Steal + stat.Guest + stat.GuestNice
}

func ioDeltasFromCounters(prev, curr []net.IOCountersStat) ([]net.IOCountersStat, error) {
	if len(prev) == 0 {
		return nil, nil
	}
	if len(prev) != len(curr) {
		return nil, fmt.Errorf("io interface count changed from %d to %d", len(prev), len(curr))
	}

	deltas := make([]net.IOCountersStat, len(curr))
	for i := range curr {
		deltas[i] = net.IOCountersStat{
			Name:        curr[i].Name,
			BytesSent:   curr[i].BytesSent - prev[i].BytesSent,
			BytesRecv:   curr[i].BytesRecv - prev[i].BytesRecv,
			PacketsSent: curr[i].PacketsSent - prev[i].PacketsSent,
			PacketsRecv: curr[i].PacketsRecv - prev[i].PacketsRecv,
		}
	}
	return deltas, nil
}
