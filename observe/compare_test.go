package observe

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestNormalizeCompareName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "build", want: "build"},
		{name: "in progress suffix", in: "build (in progress)", want: "build"},
		{name: "cancelled suffix", in: "build (cancelled)", want: "build"},
		{name: "attempt suffix", in: "build (attempt 2)", want: "build"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, normalizeCompareName(tt.in))
		})
	}
}

func TestMatchItems_byID(t *testing.T) {
	t.Parallel()

	left := []TimelineItem{
		{Name: "matrix", ID: "1", Duration: time.Minute, Conclusion: "done"},
		{Name: "matrix", ID: "2", Duration: 2 * time.Minute, Conclusion: "crit"},
	}
	right := []TimelineItem{
		{Name: "matrix", ID: "1", Duration: 90 * time.Second, Conclusion: "done"},
		{Name: "matrix", ID: "2", Duration: 3 * time.Minute, Conclusion: "crit"},
	}

	matched, onlyLeft, onlyRight := matchItems(left, right)
	require.Len(t, matched, 2)
	assert.Empty(t, onlyLeft)
	assert.Empty(t, onlyRight)

	byID := make(map[string]ComparisonItem, len(matched))
	for _, m := range matched {
		byID[m.LeftID] = m
	}
	assert.Equal(t, 30*time.Second, byID["1"].DurationDelta)
	assert.Equal(t, time.Minute, byID["2"].DurationDelta)
}

func TestMatchItems_nameFallback(t *testing.T) {
	t.Parallel()

	left := []TimelineItem{{Name: "lint (in progress)", Duration: time.Minute, Conclusion: "active"}}
	right := []TimelineItem{{Name: "lint", Duration: 2 * time.Minute, Conclusion: "done"}}

	matched, onlyLeft, onlyRight := matchItems(left, right)
	require.Len(t, matched, 1)
	assert.Empty(t, onlyLeft)
	assert.Empty(t, onlyRight)
	assert.Equal(t, time.Minute, matched[0].DurationDelta)
}

func TestFormatPercentDelta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		delta int64
		base  int64
		want  string
	}{
		{name: "zero", delta: 0, base: 100, want: "0.0%"},
		{name: "positive", delta: 50, base: 100, want: "+50.0%"},
		{name: "negative", delta: -50, base: 100, want: "-50.0%"},
		{name: "both zero", delta: 0, base: 0, want: "0.0%"},
		{name: "base zero", delta: 50, base: 0, want: "N/A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatPercentDelta(tt.delta, tt.base))
		})
	}
}

func TestSumItemCosts(t *testing.T) {
	t.Parallel()

	items := []TimelineItem{
		{Cost: 100},
		{Cost: 250},
		{Cost: 0},
	}
	assert.Equal(t, int64(350), sumItemCosts(items))
	assert.Equal(t, int64(0), sumItemCosts(nil))
}

func TestEarliestStartTime(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	timelines := []*Timeline{
		{Items: []TimelineItem{{StartTime: t1}}},
		{Items: []TimelineItem{{StartTime: t2}}},
	}
	assert.Equal(t, t2, earliestStartTime(timelines))
	assert.True(t, earliestStartTime(nil).IsZero())
}

func TestBuildEventPairs_metrics(t *testing.T) {
	t.Parallel()

	left := []*Timeline{{
		Event: "push",
		Items: []TimelineItem{
			{Name: "build", ID: "1", StartTime: time.Now(), Duration: time.Minute, Cost: 100},
		},
	}}
	right := []*Timeline{{
		Event: "push",
		Items: []TimelineItem{
			{Name: "build", ID: "1", StartTime: time.Now(), Duration: 2 * time.Minute, Cost: 150},
		},
	}}

	pairs := buildEventPairs(left, right, "owner", "repo", "workflow_run")
	require.Len(t, pairs, 1)
	pair := pairs[0]

	assert.Equal(t, time.Minute, pair.LeftDuration)
	assert.Equal(t, 2*time.Minute, pair.RightDuration)
	assert.Equal(t, time.Minute, pair.DurationDelta)
	assert.Equal(t, "+100.0%", pair.DurationDeltaPercent)
	assert.Equal(t, int64(100), pair.LeftCost)
	assert.Equal(t, int64(150), pair.RightCost)
	assert.Equal(t, int64(50), pair.CostDelta)
	assert.Equal(t, "+50.0%", pair.CostDeltaPercent)
}

func TestFormatCostDelta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		costDelta int64
		want      string
	}{
		{name: "zero", costDelta: 0, want: "$0.00"},
		{name: "positive", costDelta: 500, want: "+$0.50"},
		{name: "negative", costDelta: -3620, want: "-$3.62"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatCostDelta(tt.costDelta))
		})
	}
}

func TestCompareHTMLVisualFixes(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	comp := &Comparison{
		Owner: "owner",
		Repo:  "repo",
		Left:  &Observation{ID: "111", Name: "left"},
		Right: &Observation{ID: "222", Name: "right"},
		Summary: ComparisonSummary{
			LeftStartedAt:  now,
			RightStartedAt: now.Add(time.Hour),
		},
		EventPairs: []EventPair{
			{
				Event:         "push",
				LeftDuration:  time.Minute,
				RightDuration: 2 * time.Minute,
				DurationDelta: time.Minute,
				LeftCost:      4430,
				RightCost:     800,
				CostDelta:     -3630,
			},
		},
	}

	setActiveHTMLOutputDir(tempDir)
	renderedRelPath, err := comp.Render(log, "html")
	require.NoError(t, err)

	outPath := filepath.Join(tempDir, renderedRelPath)
	//nolint:gosec // test file read
	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	htmlStr := string(content)

	// Check header run-at chicklet badge
	assert.Contains(t, htmlStr, `class="badge badge-run-at"`)
	assert.Contains(t, htmlStr, `<span class="badge-label">Run at</span>`)

	// Check delta classes and negative currency formatting -$3.63 (faster/cheaper = delta-faster)
	assert.Contains(t, htmlStr, `class="delta-faster"`)
	assert.Contains(t, htmlStr, `-$3.63`)
}
