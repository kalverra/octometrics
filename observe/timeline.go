package observe

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type postTimelineItem struct {
	Name string
	Link string
	Time time.Time
}

type timelineData struct {
	// Triggering event
	Event string
	// Items that happen after the specified timeline
	PostTimelineItems []postTimelineItem
	Items             []timelineItem
	SkippedItems      []string
	QueuedItems       []string

	// Set by the renderer
	StartTime    time.Time
	EndTime      time.Time
	GoDateFormat string
	DateFormat   string
	AxisFormat   string
	Duration     time.Duration
}

type timelineItem struct {
	Name       string
	ID         string
	StartTime  time.Time
	Duration   time.Duration
	Conclusion string
	Link       string
	IsRequired bool
}

func (g *timelineData) process() error {
	if g == nil {
		return fmt.Errorf("timelineData is nil")
	}
	if len(g.Items) == 0 {
		return nil
	}

	sort.Slice(g.Items, func(i, j int) bool {
		if g.Items[i].StartTime.Equal(g.Items[j].StartTime) {
			return g.Items[i].Duration < g.Items[j].Duration
		}
		return g.Items[i].StartTime.Before(g.Items[j].StartTime)
	})

	// Determine the total duration of the timeline
	startTime := g.Items[0].StartTime
	endTime := g.Items[0].StartTime.Add(g.Items[0].Duration)
	for _, item := range g.Items {
		if item.StartTime.Before(startTime) {
			startTime = item.StartTime
		}
		if item.StartTime.Add(item.Duration).After(endTime) {
			endTime = item.StartTime.Add(item.Duration)
		}
	}

	// Adjust the start time of each item so that the full timeline starts at 0
	newStartTime := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	startTimeDiff := newStartTime.Sub(startTime)
	for i := range g.Items {
		g.Items[i].StartTime = g.Items[i].StartTime.Add(startTimeDiff)
	}

	// Recompute bounds from shifted times. Pre-shift g.StartTime/g.EndTime would not match
	// item StartTimes (used by compare Gantt alignment and observation headers).
	g.StartTime = g.Items[0].StartTime
	g.EndTime = g.Items[0].StartTime.Add(g.Items[0].Duration)
	for _, item := range g.Items {
		if item.StartTime.Before(g.StartTime) {
			g.StartTime = item.StartTime
		}
		if itemEnd := item.StartTime.Add(item.Duration); itemEnd.After(g.EndTime) {
			g.EndTime = itemEnd
		}
	}
	g.Duration = g.EndTime.Sub(g.StartTime)
	g.DateFormat, g.AxisFormat, g.GoDateFormat = GanttFormatsForDuration(g.Duration)
	return nil
}

// GanttFormatsForDuration returns the dateFormat, axisFormat, and goDateFormat for a given duration.
// Durations under one hour use mm:ss; longer spans use HH:mm:ss (matches Mermaid gantt dateFormat).
func GanttFormatsForDuration(span time.Duration) (dateFormat, axisFormat, goDateFormat string) {
	if span >= time.Hour {
		return "HH:mm:ss", "%H:%M:%S", "15:04:05"
	}
	return "mm:ss", "%M:%S", "04:05"
}

// ItemsByDuration returns items sorted by duration descending (longest first).
func (g *timelineData) ItemsByDuration() []timelineItem {
	sorted := make([]timelineItem, len(g.Items))
	copy(sorted, g.Items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Duration > sorted[j].Duration
	})
	return sorted
}

func sanitizeMermaidName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	s = strings.ReplaceAll(s, ":", "#colon;")
	s = strings.ReplaceAll(s, ",", "#comma;")
	return s
}
