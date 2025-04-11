package monitor

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestAnalyze(t *testing.T) {
	t.Parallel()

	monitorLogFile := filepath.Join(testDataDir, "octometrics.monitor.testdata.json")
	log, _ := testhelpers.Setup(t)
	require.FileExists(t, monitorLogFile, "monitor log file does not exist")
	analysis, err := Analyze(log, monitorLogFile)
	require.NoError(t, err, "error analyzing monitor log")
	require.NotNil(t, analysis, "analysis should not be nil")

	cpuMeasurements := analysis.CPUMeasurements
	require.NotNil(t, cpuMeasurements, "cpu measurements should not be nil")
	require.NotEmpty(t, cpuMeasurements, "cpu measurements should not be empty")

	for _, measurements := range cpuMeasurements {
		require.NotNil(t, measurements, "measurements should not be nil")
		require.NotEmpty(t, measurements, "measurements should not be empty")
	}
}
