package observe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGanttFormatsForDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		span       time.Duration
		dateFormat string
	}{
		{name: "under one hour", span: 30 * time.Minute, dateFormat: "mm:ss"},
		{name: "one hour", span: time.Hour, dateFormat: "HH:mm:ss"},
		{name: "multi hour", span: 3 * time.Hour, dateFormat: "HH:mm:ss"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dateFormat, _, _ := GanttFormatsForDuration(tt.span)
			assert.Equal(t, tt.dateFormat, dateFormat)
		})
	}
}

func TestTimelineNormalize_crossMidnight(t *testing.T) {
	t.Parallel()

	base := time.Date(2025, 1, 1, 23, 30, 0, 0, time.UTC)
	timeline := &Timeline{
		Event: "push",
		Items: []TimelineItem{
			{Name: "a", StartTime: base, Duration: 45 * time.Minute},
			{Name: "b", StartTime: base.Add(30 * time.Minute), Duration: 30 * time.Minute},
		},
	}

	require.NoError(t, timeline.normalize())
	assert.False(t, timeline.RealStartTime.IsZero())
	assert.True(t, timeline.RealEndTime.After(timeline.RealStartTime))
	assert.Equal(t, timeline.RealStartTime, base)
	assert.False(
		t,
		timeline.StartTime.Before(time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location())),
	)
}

func TestTimelineNormalize_emptyItems(t *testing.T) {
	t.Parallel()

	timeline := &Timeline{Event: "push"}
	require.NoError(t, timeline.normalize())
}

func TestSanitizeMermaidName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "hello", sanitizeMermaidName("hello"))
	assert.Equal(t, "has#colon;colons", sanitizeMermaidName("has:colons"))
	longName := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyz"
	expected := "..." + longName[len(longName)-77:]
	assert.Equal(t, expected, sanitizeMermaidName(longName))
	assert.Len(t, sanitizeMermaidName(longName), 80)
}

func TestTimelineHasRunner(t *testing.T) {
	t.Parallel()

	noRunner := &Timeline{Items: []TimelineItem{{Name: "a"}}}
	assert.False(t, noRunner.HasRunner(), "expected HasRunner to be false when no runner is set")

	withRunner := &Timeline{Items: []TimelineItem{{Name: "a", Runner: "UBUNTU"}}}
	assert.True(t, withRunner.HasRunner(), "expected HasRunner to be true when a runner is set")
}

func TestTimelineHasCost(t *testing.T) {
	t.Parallel()

	noCost := &Timeline{Items: []TimelineItem{{Name: "a"}}}
	assert.False(t, noCost.HasCost(), "expected HasCost to be false when no cost is set")

	withCost := &Timeline{Items: []TimelineItem{{Name: "a", Cost: 40, CostGathered: true}}}
	assert.True(t, withCost.HasCost(), "expected HasCost to be true when cost is gathered")

	gatheredZero := &Timeline{Items: []TimelineItem{{Name: "a", CostGathered: true}}}
	assert.True(t, gatheredZero.HasCost(), "expected HasCost to be true when cost is gathered even if zero")
}

func TestTimelineTable_RunnerColumnHiddenWhenNoRunner(t *testing.T) {
	t.Parallel()

	obs := observationWithTimeline([]TimelineItem{{Name: "build", ID: "1", Duration: 5 * time.Minute}})
	buf, err := obs.renderToFormat("html")
	require.NoError(t, err)
	htmlStr := buf.String()

	assert.Contains(t, htmlStr, "Runs (")
	assert.NotContains(t, htmlStr, `<th class="rt-runner">`)
	assert.NotContains(t, htmlStr, `<td class="rt-runner">`)
}

func TestTimelineTable_CostColumnHiddenWhenNoCost(t *testing.T) {
	t.Parallel()

	obs := observationWithTimeline(
		[]TimelineItem{{Name: "build", ID: "1", Duration: 5 * time.Minute, Runner: "UBUNTU"}},
	)
	buf, err := obs.renderToFormat("html")
	require.NoError(t, err)
	htmlStr := buf.String()

	assert.NotContains(t, htmlStr, `<th class="rt-cost">`)
	assert.NotContains(t, htmlStr, `<td class="rt-cost">`)
}

func TestTimelineTable_CostShownWhenGathered(t *testing.T) {
	t.Parallel()

	obs := observationWithTimeline([]TimelineItem{
		{Name: "build", ID: "1", Duration: 5 * time.Minute, Runner: "UBUNTU", Cost: 40, CostGathered: true},
	})
	buf, err := obs.renderToFormat("html")
	require.NoError(t, err)
	htmlStr := buf.String()

	assert.Contains(t, htmlStr, `<th class="rt-cost"`)
	assert.Contains(t, htmlStr, `<td class="rt-cost"`)
	assert.Contains(t, htmlStr, "$0.04")
}

func TestTimelineTable_CostEstimateShown(t *testing.T) {
	t.Parallel()

	obs := observationWithTimeline([]TimelineItem{
		{Name: "build", ID: "1", Duration: 5 * time.Minute, Cost: 100, CostGathered: true, CostEstimate: true},
	})
	buf, err := obs.renderToFormat("html")
	require.NoError(t, err)
	htmlStr := buf.String()

	assert.Contains(t, htmlStr, "$0.10")
	assert.Contains(t, htmlStr, "(est.)")
}

func TestTimelineTable_SortableHeaders(t *testing.T) {
	t.Parallel()

	obs := observationWithTimeline([]TimelineItem{
		{Name: "build", ID: "1", Duration: 5 * time.Minute, Runner: "UBUNTU", Cost: 40, CostGathered: true},
	})
	buf, err := obs.renderToFormat("html")
	require.NoError(t, err)
	htmlStr := buf.String()

	assert.Contains(t, htmlStr, `data-sort="name"`)
	assert.Contains(t, htmlStr, `data-sort="runner"`)
	assert.Contains(t, htmlStr, `data-sort="duration"`)
	assert.Contains(t, htmlStr, `data-sort="cost"`)
	assert.Contains(t, htmlStr, `data-sort="status"`)
	assert.Contains(t, htmlStr, `<td class="rt-name" data-sort-key="name"`)
	assert.Contains(t, htmlStr, `<td class="rt-duration" data-sort-key="duration"`)
}

func TestTimelineMD_RenamedRuns(t *testing.T) {
	t.Parallel()

	obs := observationWithTimeline([]TimelineItem{{Name: "build", ID: "1", Duration: 5 * time.Minute}})
	buf, err := obs.renderToFormat("md")
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Runs (")
}

func observationWithTimeline(items []TimelineItem) *Observation {
	duration := 5 * time.Minute
	if len(items) > 0 {
		duration = items[0].Duration
	}
	return &Observation{
		ID:       "1",
		Name:     "test-run",
		Owner:    "owner",
		Repo:     "repo",
		DataType: "workflow_run",
		State:    "success",
		TimelineData: []*Timeline{
			{
				Event:    "push",
				Duration: duration,
				Items:    items,
			},
		},
	}
}

func TestConclusionToGanttStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "crit", conclusionToGanttStatus("failure"))
	assert.Equal(t, "done", conclusionToGanttStatus("cancelled"))
	assert.Empty(t, conclusionToGanttStatus("success"))
}
