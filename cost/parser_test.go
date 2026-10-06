package cost

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV_ValidActionsRecords(t *testing.T) {
	t.Parallel()

	csvContent := "\xef\xbb\xbf\"date\",\"product\",\"sku\",\"quantity\",\"unit_type\",\"applied_cost_per_quantity\",\"gross_amount\",\"discount_amount\",\"net_amount\",\"username\",\"organization\",\"repository\",\"workflow_path\",\"cost_center_name\"\n" +
		"\"2026-09-01\",\"actions\",\"actions_linux\",\"10\",\"minutes\",\"0.006\",\"0.06\",\"0.01\",\"0.05\",\"user1\",\"smartcontractkit\",\"repo-a\",\".github/workflows/test.yml\",\"\"\n" +
		"\"2026-09-01\",\"actions\",\"actions_self_hosted_linux\",\"20\",\"minutes\",\"0\",\"0\",\"0\",\"0\",\"user2\",\"smartcontractkit\",\"repo-b\",\".github/workflows/ci.yml\",\"\"\n" +
		"\"2026-09-01\",\"actions\",\"actions_storage\",\"100\",\"gigabyte-hours\",\"0.0003\",\"0.03\",\"0\",\"0.03\",\"\",\"smartcontractkit\",\"repo-a\",\"\",\"\"\n" +
		"\"2026-09-01\",\"copilot\",\"copilot_for_business\",\"1\",\"months\",\"19\",\"19\",\"0\",\"19\",\"user3\",\"smartcontractkit\",\"\",\"\",\"\"\n"

	records, err := ParseCSV(strings.NewReader(csvContent), "")
	require.NoError(t, err)

	// Copilot and storage should be filtered out from runner compute records
	require.Len(t, records, 2)

	assert.Equal(t, "repo-a", records[0].Repository)
	assert.Equal(t, "actions_linux", records[0].SKU)
	assert.InDelta(t, 10.0, records[0].Quantity, 0.01)
	assert.InDelta(t, 0.05, records[0].NetAmount, 0.001)
	assert.InDelta(t, 0.06, records[0].GrossAmount, 0.001)
	assert.Equal(t, RunnerTypeGHANative, records[0].RunnerType)

	assert.Equal(t, "repo-b", records[1].Repository)
	assert.Equal(t, "actions_self_hosted_linux", records[1].SKU)
	assert.InDelta(t, 20.0, records[1].Quantity, 0.01)
	assert.InDelta(t, 0.0, records[1].NetAmount, 0.001)
	assert.Equal(t, RunnerTypeRunsOn, records[1].RunnerType)
}

func TestParseCSV_OrgFilter(t *testing.T) {
	t.Parallel()

	csvContent := "\"date\",\"product\",\"sku\",\"quantity\",\"unit_type\",\"applied_cost_per_quantity\",\"gross_amount\",\"discount_amount\",\"net_amount\",\"username\",\"organization\",\"repository\",\"workflow_path\",\"cost_center_name\"\n" +
		"\"2026-09-01\",\"actions\",\"actions_linux\",\"10\",\"minutes\",\"0.006\",\"0.06\",\"0\",\"0.06\",\"user1\",\"org-a\",\"repo-a\",\".github/workflows/test.yml\",\"\"\n" +
		"\"2026-09-01\",\"actions\",\"actions_linux\",\"20\",\"minutes\",\"0.006\",\"0.12\",\"0\",\"0.12\",\"user2\",\"org-b\",\"repo-b\",\".github/workflows/ci.yml\",\"\"\n"

	records, err := ParseCSV(strings.NewReader(csvContent), "org-a")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "repo-a", records[0].Repository)
}

func TestParseCSV_InvalidHeader(t *testing.T) {
	t.Parallel()

	csvContent := "\"col1\",\"col2\"\n\"val1\",\"val2\"\n"
	_, err := ParseCSV(strings.NewReader(csvContent), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required column")
}
