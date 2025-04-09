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
	_, err := Analyze(log, "testdata/monitor.log.json")
	require.NoError(t, err, "error analyzing monitor log")
	t.Fatal("not implemented fully")
}
