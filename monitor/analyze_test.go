package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestAnalyze(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)
	require.FileExists(t, "testdata/monitor.log.json", "monitor log file does not exist")
	analysis, err := Analyze(log, "testdata/monitor.log.json")
	require.NoError(t, err, "error analyzing monitor log")
	require.NotNil(t, analysis, "analysis should not be nil")

	cpuMeasurements := analysis.CPUMeasurements
	require.NotNil(t, cpuMeasurements, "cpu measurements should not be nil")
	require.NotEmpty(t, cpuMeasurements, "cpu measurements should not be empty")

	for _, measurements := range cpuMeasurements {
		require.NotNil(t, measurements, "measurements should not be nil")
		require.NotEmpty(t, measurements, "measurements should not be empty")
	}

	// TODO: Add more tests and better assertions on exact data expected
}
