package cost

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCloudzeroCSV(t *testing.T) {
	t.Parallel()

	csvContent := "# RETRIEVED:2026-09-29T18:43:45\n" +
		"tag:runs-on-repo-full-name,2026-09-01,2026-09-02,2026-09-03\n" +
		"smartcontractkit/chainlink,100.50,200.00,50.25\n" +
		"smartcontractkit/chainlink-ccv,10.00,,5.00\n" +
		"Resources Without The Specified Tag,-5.00,2.00,0.00\n"

	result, err := ParseCloudzeroCSV(strings.NewReader(csvContent))
	require.NoError(t, err)

	assert.InDelta(t, 350.75, result.RepoCosts["chainlink"], 0.01)
	assert.InDelta(t, 15.00, result.RepoCosts["chainlink-ccv"], 0.01)
	assert.InDelta(t, -3.00, result.UntaggedCost, 0.01)
	assert.InDelta(t, 362.75, result.TotalCost, 0.01)
}
