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
	assert.Len(t, sanitizeMermaidName(string(make([]byte, 100))), 80)
}

func TestConclusionToGanttStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "crit", conclusionToGanttStatus("failure"))
	assert.Equal(t, "done", conclusionToGanttStatus("cancelled"))
	assert.Empty(t, conclusionToGanttStatus("success"))
}
