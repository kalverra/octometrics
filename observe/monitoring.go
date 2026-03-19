package observe

import (
	"time"

	"github.com/kalverra/octometrics/monitor"
	"github.com/kalverra/octometrics/report"
)

type monitoringData struct {
	Charts []report.MonitoringChart
}

func monitoring(analysis *monitor.Analysis, windowStart, windowEnd time.Time) (*monitoringData, error) {
	if analysis == nil {
		return nil, nil
	}
	var charts []report.MonitoringChart
	if windowEnd.After(windowStart) {
		charts = report.MonitoringMermaidChartsWithWindow(analysis, windowStart, windowEnd)
	}
	if len(charts) == 0 {
		charts = report.MonitoringMermaidCharts(analysis)
	}
	if len(charts) == 0 {
		return nil, nil
	}
	return &monitoringData{Charts: charts}, nil
}
