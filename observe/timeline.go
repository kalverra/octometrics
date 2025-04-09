package observe

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type timelineData struct {
	Items        []timelineItem
	SkippedItems []string

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
	g.Duration = endTime.Sub(startTime)
	g.StartTime = startTime
	g.EndTime = endTime

	// Adjust the start time of each item so that you start at 0
	newStartTime := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	startTimeDiff := newStartTime.Sub(startTime)
	for i := range g.Items {
		g.Items[i].StartTime = g.Items[i].StartTime.Add(startTimeDiff)
	}

	if endTime.Sub(startTime) >= time.Hour {
		g.DateFormat, g.AxisFormat, g.GoDateFormat = "HH:mm:ss", "%H:%M:%S", "15:04:05"
	} else {
		g.DateFormat, g.AxisFormat, g.GoDateFormat = "mm:ss", "%M:%S", "04:05"
	}
	return nil
}

func sanitizeMermaidName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ":", "#colon;")
	s = strings.ReplaceAll(s, ",", "<comma>")
	return s
}
