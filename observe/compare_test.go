package observe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
