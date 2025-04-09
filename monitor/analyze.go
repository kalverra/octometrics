package monitor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// SystemInfo contains system-level information about CPU, memory, and disk.
type SystemInfo struct {
	CPU    []*SystemCPUInfo `json:"cpu"`
	Memory *SytemMemoryInfo `json:"memory"`
	Disk   *SystemDiskInfo  `json:"disk"`
}

type SystemCPUInfo struct {
	Num       int     `json:"num"`
	Model     string  `json:"model"`
	Vendor    string  `json:"vendor"`
	Family    string  `json:"family"`
	CacheSize int32   `json:"cache_size"`
	Cores     int32   `json:"cores"`
	Mhz       float64 `json:"mhz"`
}

type SytemMemoryInfo struct {
	Total uint64 `json:"total"`
}

type SystemDiskInfo struct {
	Total uint64 `json:"total"`
}

type CPUMeasurement struct {
	Time        time.Time `json:"time"`
	Num         int       `json:"num"`
	UsedPercent float64   `json:"used_percent"`
}

type MemoryMeasurement struct {
	Time      time.Time `json:"time"`
	Available uint64    `json:"available"`
	Used      uint64    `json:"used"`
}

type DiskMeasurement struct {
	Time        time.Time `json:"time"`
	Used        uint64    `json:"used"`
	Available   uint64    `json:"available"`
	UsedPercent float64   `json:"used_percent"`
}

type IOMeasurement struct {
	Time        time.Time `json:"time"`
	BytesSent   uint64    `json:"bytes_sent"`
	BytesRecv   uint64    `json:"bytes_recv"`
	PacketsSent uint64    `json:"packets_sent"`
	PacketsRecv uint64    `json:"packets_recv"`
}

// Analysis is the processed results of monitoring data.
type Analysis struct {
	SystemInfo *SystemInfo `json:"system_info"`
	// CPUMeasurements is a map of CPU number to its measurements.
	CPUMeasurements    map[int][]*CPUMeasurement `json:"cpu_measurements"`
	MemoryMeasurements []*MemoryMeasurement      `json:"memory_measurements"`
	DiskMeasurements   []*DiskMeasurement        `json:"disk_measurements"`
	IOMeasurements     []*IOMeasurement          `json:"io_measurements"`
}

// Analyze reads the monitor data from a file and processes each entry.
func Analyze(log zerolog.Logger, dataFile string) (*Analysis, error) {
	file, err := os.Open(dataFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close analysis file")
		}
	}()

	var (
		scanner      = bufio.NewScanner(file)
		startTime    = time.Now()
		linesScanned = 0
		analysis     = &Analysis{
			SystemInfo:         &SystemInfo{},
			CPUMeasurements:    map[int][]*CPUMeasurement{},
			MemoryMeasurements: []*MemoryMeasurement{},
			DiskMeasurements:   []*DiskMeasurement{},
			IOMeasurements:     []*IOMeasurement{},
		}
	)
	for scanner.Scan() {
		line := scanner.Text()
		linesScanned++

		var entry *monitorEntry
		err := json.Unmarshal([]byte(line), &entry)
		if err != nil {
			return nil, fmt.Errorf("failed to parse entry: %w", err)
		}

		if entry == nil {
			return nil, fmt.Errorf("entry %d is nil", linesScanned)
		}
		err = processEntry(analysis, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to process entry %d: %w", linesScanned, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}
	log.Info().
		Str("Duration", time.Since(startTime).String()).
		Int("Lines scanned", linesScanned).
		Msg("Finished analyzing monitor data")
	return analysis, nil
}

func processEntry(analysis *Analysis, entry *monitorEntry) error {
	if analysis == nil {
		return errors.New("analysis is nil")
	}
	if entry == nil {
		return errors.New("entry is nil")
	}

	switch entry.Message {
	case CPUSystemInfoMsg:
		analysis.SystemInfo.CPU = append(analysis.SystemInfo.CPU, &SystemCPUInfo{
			Num:       entry.GetNum(),
			Model:     entry.GetModel(),
			Vendor:    entry.GetVendor(),
			Family:    entry.GetFamily(),
			CacheSize: entry.GetCacheSize(),
			Cores:     entry.GetCores(),
			Mhz:       entry.GetMhz(),
		})
	case MemSystemInfoMsg:
		analysis.SystemInfo.Memory = &SytemMemoryInfo{
			Total: entry.GetTotal(),
		}
	case DiskSystemInfoMsg:
		analysis.SystemInfo.Disk = &SystemDiskInfo{
			Total: entry.GetTotal(),
		}
	case ObservedCPUMsg:
		cpuNum := entry.GetNum()
		if _, ok := analysis.CPUMeasurements[cpuNum]; !ok {
			analysis.CPUMeasurements[cpuNum] = []*CPUMeasurement{}
		}
		analysis.CPUMeasurements[cpuNum] = append(analysis.CPUMeasurements[cpuNum], &CPUMeasurement{
			Time:        entry.GetTime(),
			Num:         cpuNum,
			UsedPercent: entry.GetUsedPercent(),
		})
	case ObservedMemMsg:
		analysis.MemoryMeasurements = append(analysis.MemoryMeasurements, &MemoryMeasurement{
			Time:      entry.GetTime(),
			Available: entry.GetAvailable(),
			Used:      entry.GetUsed(),
		})
	case ObservedDiskMsg:
		analysis.DiskMeasurements = append(analysis.DiskMeasurements, &DiskMeasurement{
			Time:        entry.GetTime(),
			Used:        entry.GetUsed(),
			Available:   entry.GetAvailable(),
			UsedPercent: entry.GetUsedPercent(),
		})
	case ObservedIOMsg:
		analysis.IOMeasurements = append(analysis.IOMeasurements, &IOMeasurement{
			Time:        entry.GetTime(),
			BytesSent:   entry.GetBytesSent(),
			BytesRecv:   entry.GetBytesRecv(),
			PacketsSent: entry.GetPacketsSent(),
			PacketsRecv: entry.GetPacketsRecv(),
		})
	}

	return nil
}
