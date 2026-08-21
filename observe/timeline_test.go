package observe

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
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
	expected := longName[:38] + "..." + longName[len(longName)-38:]
	assert.Equal(t, expected, sanitizeMermaidName(longName))
	assert.Len(t, sanitizeMermaidName(longName), 79)
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

func TestObservation_MultipleEventTimelines_PerEventCost(t *testing.T) {
	t.Parallel()

	obs := &Observation{
		ID:           "abc1234",
		Name:         "Commit abc1234",
		Owner:        "owner",
		Repo:         "repo",
		DataType:     "commit",
		State:        "success",
		Cost:         5000, // $5.00 total
		CostGathered: true,
		TimelineData: []*Timeline{
			{
				Event:        "push",
				Duration:     5 * time.Minute,
				Cost:         1200, // $1.20
				CostGathered: true,
				Items: []TimelineItem{
					{Name: "build", ID: "1", Duration: 5 * time.Minute, Cost: 1200, CostGathered: true},
				},
			},
			{
				Event:        "pull_request",
				Duration:     10 * time.Minute,
				Cost:         3800, // $3.80
				CostGathered: true,
				Items: []TimelineItem{
					{Name: "test", ID: "2", Duration: 10 * time.Minute, Cost: 3800, CostGathered: true},
				},
			},
		},
	}

	buf, err := obs.renderToFormat("html")
	require.NoError(t, err)
	htmlStr := buf.String()

	// Overall cost in header badge
	assert.Contains(t, htmlStr, `<span class="badge-label">Cost</span> $5.00`)

	// Event summary headers should show their respective event costs, not overall cost
	assert.Contains(t, htmlStr, "push &mdash; 5m0s, $1.20")
	assert.Contains(t, htmlStr, "pull_request &mdash; 10m0s, $3.80")
	assert.NotContains(t, htmlStr, "push &mdash; 5m0s, $5.00")
	assert.NotContains(t, htmlStr, "pull_request &mdash; 10m0s, $5.00")

	// Check markdown format as well
	bufMD, err := obs.renderToFormat("md")
	require.NoError(t, err)
	mdStr := bufMD.String()

	assert.Contains(t, mdStr, "## push — 5m0s, $1.20")
	assert.Contains(t, mdStr, "## pull_request — 10m0s, $3.80")
	assert.NotContains(t, mdStr, "## push — 5m0s, $5.00")
	assert.NotContains(t, mdStr, "## pull_request — 10m0s, $5.00")
}

func TestBuildCommitTimelineData_PerEventCost(t *testing.T) {
	t.Parallel()

	now := time.Now()
	commitData := &gather.CommitData{
		Owner: "owner",
		Repo:  "repo",
	}

	run1 := &gather.WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			ID:           new(int64(1)),
			Name:         new("CI"),
			Event:        new("push"),
			Status:       new("completed"),
			Conclusion:   new("success"),
			RunStartedAt: &github.Timestamp{Time: now},
		},
		RunCompletedAt: now.Add(5 * time.Minute),
		Cost:           1200,
		CostGathered:   true,
	}

	run2 := &gather.WorkflowRunData{
		WorkflowRun: &github.WorkflowRun{
			ID:           new(int64(2)),
			Name:         new("PR Checks"),
			Event:        new("pull_request"),
			Status:       new("completed"),
			Conclusion:   new("success"),
			RunStartedAt: &github.Timestamp{Time: now},
		},
		RunCompletedAt: now.Add(10 * time.Minute),
		Cost:           3800,
		CostGathered:   true,
	}

	timelines := buildCommitTimelineData(zerolog.Nop(), commitData, []*gather.WorkflowRunData{run1, run2})
	require.Len(t, timelines, 2)

	timelineByEvent := make(map[string]*Timeline)
	for _, tl := range timelines {
		timelineByEvent[tl.Event] = tl
	}

	pushTL, ok := timelineByEvent["push"]
	require.True(t, ok)
	assert.Equal(t, int64(1200), pushTL.Cost)
	assert.True(t, pushTL.CostGathered)

	prTL, ok := timelineByEvent["pull_request"]
	require.True(t, ok)
	assert.Equal(t, int64(3800), prTL.Cost)
	assert.True(t, prTL.CostGathered)
}

type recordingReporter struct {
	starts []string
	stops  []string
}

func (r *recordingReporter) Start(msg string) {
	r.starts = append(r.starts, msg)
}

func (r *recordingReporter) Update(_ string, _ time.Duration) {}

func (r *recordingReporter) Stop(msg string) {
	r.stops = append(r.stops, msg)
}

func TestObserveProgressReporter_Option(t *testing.T) {
	t.Parallel()

	rec := &recordingReporter{}
	opts := defaultOptions()
	WithProgressReporter(rec)(opts)

	assert.Equal(t, rec, opts.reporter)
}

func TestInteractive_StopsProgressReporter(t *testing.T) {
	t.Parallel()

	rec := &recordingReporter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	log := zerolog.Nop()
	_ = Interactive(ctx, log, nil, "/", t.TempDir(), WithProgressReporter(rec), WithNoOpen(true), WithPort(0))

	assert.NotEmpty(t, rec.stops, "Interactive must call reporter.Stop before starting server")
}
