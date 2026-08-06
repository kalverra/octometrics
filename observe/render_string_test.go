package observe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestObservation_RenderString(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	obs := &Observation{
		ID:         "123",
		Name:       "Test Workflow",
		GitHubLink: "https://github.com/kalverra/octometrics/actions/runs/123",
		Owner:      "kalverra",
		Repo:       "octometrics",
		DataType:   "workflow_run",
		State:      "success",
		Actor:      "kalverra",
	}

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)
	assert.Contains(t, md, "# [Test Workflow]")
	assert.Contains(t, md, "success")
}

func TestComparison_RenderString(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	comp := &Comparison{
		Owner: "kalverra",
		Repo:  "octometrics",
		Left:  &Observation{ID: "111", Name: "Run 1"},
		Right: &Observation{ID: "222", Name: "Run 2"},
		Summary: ComparisonSummary{
			LeftStartedAt:  time.Now(),
			RightStartedAt: time.Now().Add(time.Hour),
		},
	}

	md, err := comp.RenderString(log, "md")
	require.NoError(t, err)
	assert.Contains(t, md, "# Comparison: Run 1 vs Run 2")
}
