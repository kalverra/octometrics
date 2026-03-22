package observe

import (
	"time"

	"github.com/kalverra/octometrics/monitor"
	"github.com/kalverra/octometrics/report"
)

// Monitoring contains performance monitoring charts.
type Monitoring struct {
	Charts []report.MonitoringChart
}

func monitoring(analysis *monitor.Analysis, windowStart, windowEnd time.Time) (*Monitoring, error) {
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
	return &Monitoring{Charts: charts}, nil
}
