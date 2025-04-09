package observe

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	grob "github.com/MetalBlueberry/go-plotly/generated/v2.34.0/graph_objects"
	"github.com/MetalBlueberry/go-plotly/pkg/types"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/monitor"
)

type monitoringData struct {
	PlotlyData []*plotlyData
}

type plotlyData struct {
	GraphID    string
	B64Content string
}

func Monitoring(
	log zerolog.Logger,
	analysis *monitor.Analysis,
	outputTypes []string,
) (*monitoringData, error) {
	if analysis == nil {
		return nil, fmt.Errorf("analysis is nil")
	}

	plotlyData := []*plotlyData{}

	cpuData, err := cpu(analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get cpu data: %w", err)
	}
	plotlyData = append(plotlyData, cpuData)

	memoryData, err := memory(analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory data: %w", err)
	}
	plotlyData = append(plotlyData, memoryData)

	diskData, err := disk(analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk data: %w", err)
	}
	plotlyData = append(plotlyData, diskData)

	ioData, err := io(analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get io data: %w", err)
	}
	plotlyData = append(plotlyData, ioData)

	return &monitoringData{
		PlotlyData: plotlyData,
	}, nil
}

func cpu(analysis *monitor.Analysis) (*plotlyData, error) {
	if len(analysis.CPUMeasurements) == 0 {
		return nil, nil
	}

	data := []types.Trace{}
	for cpuNum, cpuMeasurements := range analysis.CPUMeasurements {
		x := make([]time.Time, len(cpuMeasurements))
		y := make([]float64, len(cpuMeasurements))
		for _, cpuMeasurement := range cpuMeasurements {
			x = append(x, cpuMeasurement.Time)
			y = append(y, cpuMeasurement.UsedPercent)
		}
		data = append(data, &grob.Scatter{
			X:    types.DataArray(x),
			Y:    types.DataArray(y),
			Name: types.StringType(fmt.Sprintf("CPU %d", cpuNum)),
			Mode: grob.ScatterModeLines,
		})
	}

	fig := &grob.Fig{
		Data: data,
		Layout: &grob.Layout{
			Title: &grob.LayoutTitle{
				Text: "CPU Usage",
			},
		},
	}

	return buildPlotlyData(fig, "cpu_usage")
}

func memory(analysis *monitor.Analysis) (*plotlyData, error) {
	if len(analysis.MemoryMeasurements) == 0 {
		return nil, nil
	}

	if analysis.SystemInfo == nil || analysis.SystemInfo.Memory == nil {
		return nil, fmt.Errorf("system memory info is missing")
	}

	totalMemory := analysis.SystemInfo.Memory.Total
	if totalMemory == 0 {
		return nil, fmt.Errorf("total memory is 0")
	}

	x := make([]time.Time, len(analysis.MemoryMeasurements))
	y := make([]float64, len(analysis.MemoryMeasurements))
	for i, measurement := range analysis.MemoryMeasurements {
		x[i] = measurement.Time
		y[i] = float64(measurement.Used) / 1024 / 1024
	}

	data := []types.Trace{
		&grob.Scatter{
			X:    types.DataArray(x),
			Y:    types.DataArray(y),
			Name: types.StringType("Memory Usage"),
			Mode: grob.ScatterModeLines,
			Fill: grob.ScatterFillTonexty,
		},
	}

	fig := &grob.Fig{
		Data: data,
		Layout: &grob.Layout{
			Title: &grob.LayoutTitle{
				Text: "Memory Usage",
			},
			Yaxis: &grob.LayoutYaxis{
				Title: &grob.LayoutYaxisTitle{
					Text: "Usage (MB)",
				},
				Range: []float64{0, float64(totalMemory) / 1024 / 1024},
			},
		},
	}

	return buildPlotlyData(fig, "memory_usage")
}

func disk(analysis *monitor.Analysis) (*plotlyData, error) {
	if len(analysis.DiskMeasurements) == 0 {
		return nil, nil
	}

	if analysis.SystemInfo == nil || analysis.SystemInfo.Disk == nil {
		return nil, fmt.Errorf("system disk info is missing")
	}

	totalDisk := analysis.SystemInfo.Disk.Total
	if totalDisk == 0 {
		return nil, fmt.Errorf("total disk is 0")
	}

	x := make([]time.Time, len(analysis.DiskMeasurements))
	y := make([]float64, len(analysis.DiskMeasurements))
	for i, measurement := range analysis.DiskMeasurements {
		x[i] = measurement.Time
		y[i] = float64(measurement.Used) / 1024 / 1024 / 1024
	}

	data := []types.Trace{
		&grob.Scatter{
			X:    types.DataArray(x),
			Y:    types.DataArray(y),
			Name: types.StringType("Disk Usage"),
			Mode: grob.ScatterModeLines,
			Fill: grob.ScatterFillTonexty,
		},
	}

	fig := &grob.Fig{
		Data: data,
		Layout: &grob.Layout{
			Title: &grob.LayoutTitle{
				Text: "Disk Usage",
			},
			Yaxis: &grob.LayoutYaxis{
				Title: &grob.LayoutYaxisTitle{
					Text: "Usage (GB)",
				},
				Range: []float64{0, float64(totalDisk) / 1024 / 1024 / 1024},
			},
		},
	}

	return buildPlotlyData(fig, "disk_usage")
}

func io(analysis *monitor.Analysis) (*plotlyData, error) {
	if len(analysis.IOMeasurements) == 0 {
		return nil, nil
	}

	x := make([]time.Time, len(analysis.IOMeasurements))
	bytesSent := make([]float64, len(analysis.IOMeasurements))
	bytesRecv := make([]float64, len(analysis.IOMeasurements))
	for i, measurement := range analysis.IOMeasurements {
		x[i] = measurement.Time
		bytesSent[i] = float64(measurement.BytesSent) / 1024 / 1024 // Convert to MB
		bytesRecv[i] = float64(measurement.BytesRecv) / 1024 / 1024 // Convert to MB
	}

	data := []types.Trace{
		&grob.Scatter{
			X:    types.DataArray(x),
			Y:    types.DataArray(bytesSent),
			Name: types.StringType("Bytes Sent"),
			Mode: grob.ScatterModeLines,
			Line: &grob.ScatterLine{
				Color: types.Color("red"),
			},
		},
		&grob.Scatter{
			X:    types.DataArray(x),
			Y:    types.DataArray(bytesRecv),
			Name: types.StringType("Bytes Received"),
			Mode: grob.ScatterModeLines,
			Line: &grob.ScatterLine{
				Color: types.Color("blue"),
			},
		},
	}

	fig := &grob.Fig{
		Data: data,
		Layout: &grob.Layout{
			Title: &grob.LayoutTitle{
				Text: "Network IO",
			},
			Yaxis: &grob.LayoutYaxis{
				Title: &grob.LayoutYaxisTitle{
					Text: "Data (MB)",
				},
			},
		},
	}

	return buildPlotlyData(fig, "io_usage")
}

func buildPlotlyData(fig types.Fig, graphID string) (*plotlyData, error) {
	figBytes, err := json.Marshal(fig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal plotly fig: %w", err)
	}
	return &plotlyData{
		GraphID:    graphID,
		B64Content: base64.StdEncoding.EncodeToString(figBytes),
	}, nil
}
