package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestAnalyze(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)
	require.FileExists(t, "testdata/monitorlog", "monitor log file does not exist")
	_, err := Analyze(log, "testdata/monitorlog")
	require.NoError(t, err, "error analyzing monitor log")
	// TODO: Verify the analysis is as expected
}
