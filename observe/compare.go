package observe

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

const comparisonsOutputDir = "comparisons"

// Comparison holds two observations of the same type and their computed diff.
type Comparison struct {
	Left        *Observation
	Right       *Observation
	Owner       string
	Repo        string
	CompareType string // "workflow_run" or "commit"
	Summary     ComparisonSummary

	// EventPairs groups left/right timelines by event for side-by-side rendering.
	// Populated during Render() after process() normalizes the timelines.
	EventPairs []EventPair

	// MonitoringPairs pairs corresponding charts by title from Left and Right observations.
	MonitoringPairs []MonitoringPair
}

// MonitoringPair holds related charts from the left and right observations.
type MonitoringPair struct {
	Title        string
	LeftDiagram  string
	RightDiagram string
}

// EventPair groups the left and right Timeline for the same triggering event.
// Either side may be nil when the event only appears in one observation.
type EventPair struct {
	Event         string
	Left          *Timeline
	Right         *Timeline
	IsTypical     bool // true for common events (pull_request, push, merge_group)
	LeftDuration  time.Duration
	RightDuration time.Duration
	DurationDelta time.Duration
	Items         []ComparisonItem
	OnlyLeft      []ComparisonOnlyItem
	OnlyRight     []ComparisonOnlyItem

	// CombinedGantt is a single Mermaid Gantt with Left/Right sections, aligned to the same start time.
	CombinedGantt *CompareGanttData
}

// CompareGanttData holds sectioned tasks for the combined comparison Gantt chart.
type CompareGanttData struct {
	Sections     []CompareGanttSection
	DateFormat   string
	AxisFormat   string
	GoDateFormat string
}

// CompareGanttSection is one Mermaid Gantt section (e.g. Left vs Right).
type CompareGanttSection struct {
	Label string
	Tasks []CompareGanttTask
}

// CompareGanttTask is a single row in the combined Gantt (exported for html/template).
type CompareGanttTask struct {
	Name       string
	ID         string
	StartTime  time.Time
	Duration   time.Duration
	Conclusion string
	Link       string
}

var typicalEvents = map[string]bool{
	"pull_request": true,
	"push":         true,
	"merge_group":  true,
}

// ComparisonItem represents a matched item present in both observations.
type ComparisonItem struct {
	Name            string
	LeftID          string
	RightID         string
	LeftDuration    time.Duration
	RightDuration   time.Duration
	DurationDelta   time.Duration // Right - Left; positive means right is slower
	LeftConclusion  string        // Gantt status: "", "crit", "done", "active"
	RightConclusion string
	StatusChanged   bool
}

// ComparisonOnlyItem represents an item present in only one observation.
type ComparisonOnlyItem struct {
	Name       string
	Duration   time.Duration
	Conclusion string
}

// ComparisonSummary holds aggregate comparison metrics.
type ComparisonSummary struct {
	LeftDuration  time.Duration
	RightDuration time.Duration
	DurationDelta time.Duration
	LeftCost      int64
	RightCost     int64
}

// CompareWorkflowRuns builds a comparison between two workflow runs.
func CompareWorkflowRuns(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	leftID, rightID int64,
	opts ...Option,
) (*Comparison, error) {
	left, err := WorkflowRun(log, client, owner, repo, leftID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build left observation (run %d): %w", leftID, err)
	}
	right, err := WorkflowRun(log, client, owner, repo, rightID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build right observation (run %d): %w", rightID, err)
	}
	return buildComparison(left, right, owner, repo, "workflow_run"), nil
}

// CompareCommits builds a comparison between two commits.
func CompareCommits(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	leftSHA, rightSHA string,
	opts ...Option,
) (*Comparison, error) {
	left, err := Commit(log, client, owner, repo, leftSHA, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build left observation (commit %s): %w", leftSHA, err)
	}
	right, err := Commit(log, client, owner, repo, rightSHA, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build right observation (commit %s): %w", rightSHA, err)
	}
	return buildComparison(left, right, owner, repo, "commit"), nil
}

// CompareJobRuns builds a comparison between two job runs.
func CompareJobRuns(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	leftWfID, rightWfID int64,
	leftJobID, rightJobID int64,
	opts ...Option,
) (*Comparison, error) {
	leftJobs, err := JobRuns(log, client, owner, repo, leftWfID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to gather jobs for left workflow %d: %w", leftWfID, err)
	}
	var leftObs *Observation
	for _, j := range leftJobs {
		if j.ID == fmt.Sprint(leftJobID) {
			leftObs = j
			break
		}
	}
	if leftObs == nil {
		return nil, fmt.Errorf("job %d not found in workflow %d", leftJobID, leftWfID)
	}

	rightJobs, err := JobRuns(log, client, owner, repo, rightWfID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to gather jobs for right workflow %d: %w", rightWfID, err)
	}
	var rightObs *Observation
	for _, j := range rightJobs {
		if j.ID == fmt.Sprint(rightJobID) {
			rightObs = j
			break
		}
	}
	if rightObs == nil {
		return nil, fmt.Errorf("job %d not found in workflow %d", rightJobID, rightWfID)
	}

	return buildComparison(leftObs, rightObs, owner, repo, "job_run"), nil
}

// buildComparison constructs a Comparison with per-event matching.
// Must be called before process() is invoked on either observation's timelines,
// since process() shifts start times for chart rendering.
func buildComparison(left, right *Observation, owner, repo, compareType string) *Comparison {
	leftDur := wallClockDuration(left.TimelineData)
	rightDur := wallClockDuration(right.TimelineData)

	c := &Comparison{
		Left:        left,
		Right:       right,
		Owner:       owner,
		Repo:        repo,
		CompareType: compareType,
		Summary: ComparisonSummary{
			LeftDuration:  leftDur,
			RightDuration: rightDur,
			DurationDelta: rightDur - leftDur,
			LeftCost:      left.Cost,
			RightCost:     right.Cost,
		},
	}

	// Build pairs of monitoring charts by matching on Title
	if left.MonitoringData != nil || right.MonitoringData != nil {
		leftCharts := make(map[string]string)
		if left.MonitoringData != nil {
			for _, chart := range left.MonitoringData.Charts {
				leftCharts[chart.Title] = chart.Diagram
			}
		}

		rightCharts := make(map[string]string)
		if right.MonitoringData != nil {
			for _, chart := range right.MonitoringData.Charts {
				rightCharts[chart.Title] = chart.Diagram
			}
		}

		// Use a stable order: first all titles from left, then any remaining from right
		seenCharts := make(map[string]bool)
		if left.MonitoringData != nil {
			for _, chart := range left.MonitoringData.Charts {
				c.MonitoringPairs = append(c.MonitoringPairs, MonitoringPair{
					Title:        chart.Title,
					LeftDiagram:  chart.Diagram,
					RightDiagram: rightCharts[chart.Title],
				})
				seenCharts[chart.Title] = true
			}
		}

		if right.MonitoringData != nil {
			for _, chart := range right.MonitoringData.Charts {
				if !seenCharts[chart.Title] {
					c.MonitoringPairs = append(c.MonitoringPairs, MonitoringPair{
						Title:        chart.Title,
						RightDiagram: chart.Diagram,
					})
				}
			}
		}
	}

	return c
}

// matchItems computes matched, only-left, and only-right items for two sets of timeline items.
func matchItems(leftItems, rightItems []TimelineItem) ([]ComparisonItem, []ComparisonOnlyItem, []ComparisonOnlyItem) {
	leftByName := make(map[string]TimelineItem, len(leftItems))
	for _, item := range leftItems {
		leftByName[item.Name] = item
	}
	rightByName := make(map[string]TimelineItem, len(rightItems))
	for _, item := range rightItems {
		rightByName[item.Name] = item
	}

	var matched []ComparisonItem
	seen := make(map[string]bool)
	for name, li := range leftByName {
		if ri, ok := rightByName[name]; ok {
			delta := ri.Duration - li.Duration
			matched = append(matched, ComparisonItem{
				Name:            name,
				LeftID:          li.ID,
				RightID:         ri.ID,
				LeftDuration:    li.Duration,
				RightDuration:   ri.Duration,
				DurationDelta:   delta,
				LeftConclusion:  li.Conclusion,
				RightConclusion: ri.Conclusion,
				StatusChanged:   li.Conclusion != ri.Conclusion,
			})
			seen[name] = true
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return absDur(matched[i].DurationDelta) > absDur(matched[j].DurationDelta)
	})

	var onlyLeft []ComparisonOnlyItem
	for name, item := range leftByName {
		if !seen[name] {
			onlyLeft = append(onlyLeft, ComparisonOnlyItem{
				Name:       name,
				Duration:   item.Duration,
				Conclusion: item.Conclusion,
			})
		}
	}
	sort.Slice(onlyLeft, func(i, j int) bool { return onlyLeft[i].Name < onlyLeft[j].Name })

	var onlyRight []ComparisonOnlyItem
	for name, item := range rightByName {
		if _, inLeft := leftByName[name]; !inLeft {
			onlyRight = append(onlyRight, ComparisonOnlyItem{
				Name:       name,
				Duration:   item.Duration,
				Conclusion: item.Conclusion,
			})
		}
	}
	sort.Slice(onlyRight, func(i, j int) bool { return onlyRight[i].Name < onlyRight[j].Name })

	return matched, onlyLeft, onlyRight
}

// wallClockDuration computes the wall-clock span across all Timeline items
// (earliest start to latest end), accounting for parallel execution.
func wallClockDuration(timelines []*Timeline) time.Duration {
	var earliest, latest time.Time
	first := true
	for _, td := range timelines {
		for _, item := range td.Items {
			if first || item.StartTime.Before(earliest) {
				earliest = item.StartTime
			}
			end := item.StartTime.Add(item.Duration)
			if first || end.After(latest) {
				latest = end
			}
			first = false
		}
	}
	if first {
		return 0
	}
	return latest.Sub(earliest)
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// compareGanttEpoch is a fixed calendar anchor so both sides share one Mermaid time axis.
var compareGanttEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// buildCompareGantt builds a single Gantt chart with one section per non-empty side.
// Each side is shifted so its Timeline start aligns with compareGanttEpoch, making
// parallel structure and relative timing easy to compare across runs.
func buildCompareGantt(left, right *Timeline) *CompareGanttData {
	var sections []CompareGanttSection
	maxEnd := compareGanttEpoch

	appendSide := func(label string, side *Timeline, idPrefix string) {
		if side == nil || len(side.Items) == 0 {
			return
		}
		origin := side.StartTime
		tasks := make([]CompareGanttTask, 0, len(side.Items))
		for _, it := range side.Items {
			off := it.StartTime.Sub(origin)
			ns := compareGanttEpoch.Add(off)
			tasks = append(tasks, CompareGanttTask{
				Name:       it.Name,
				ID:         idPrefix + it.ID,
				StartTime:  ns,
				Duration:   it.Duration,
				Conclusion: it.Conclusion,
				Link:       it.Link,
			})
			end := ns.Add(it.Duration)
			if end.After(maxEnd) {
				maxEnd = end
			}
		}
		sections = append(sections, CompareGanttSection{Label: label, Tasks: tasks})
	}

	appendSide("Left", left, "cl-")
	appendSide("Right", right, "cr-")

	if len(sections) == 0 {
		return nil
	}

	span := maxEnd.Sub(compareGanttEpoch)
	if span <= 0 {
		span = time.Second
	}
	df, af, gf := GanttFormatsForDuration(span)
	return &CompareGanttData{
		Sections:     sections,
		DateFormat:   df,
		AxisFormat:   af,
		GoDateFormat: gf,
	}
}

func buildEventPairs(left, right []*Timeline, owner, repo, compareType string) []EventPair {
	leftByEvent := make(map[string]*Timeline, len(left))
	for _, td := range left {
		leftByEvent[td.Event] = td
	}
	rightByEvent := make(map[string]*Timeline, len(right))
	for _, td := range right {
		rightByEvent[td.Event] = td
	}

	seen := make(map[string]bool)
	var pairs []EventPair

	for _, td := range left {
		pair := EventPair{
			Event:     td.Event,
			Left:      td,
			Right:     rightByEvent[td.Event],
			IsTypical: typicalEvents[td.Event],
		}
		var rightItems []TimelineItem
		if pair.Right != nil {
			rightItems = pair.Right.Items
		}
		pair.Items, pair.OnlyLeft, pair.OnlyRight = matchItems(td.Items, rightItems)

		var leftDur, rightDur time.Duration
		if pair.Left != nil {
			leftDur = wallClockDuration([]*Timeline{pair.Left})
		}
		if pair.Right != nil {
			rightDur = wallClockDuration([]*Timeline{pair.Right})
		}
		pair.LeftDuration = leftDur
		pair.RightDuration = rightDur
		pair.DurationDelta = rightDur - leftDur

		rewriteLinksForComparisonItems(&pair, owner, repo, compareType)
		pair.CombinedGantt = buildCompareGantt(pair.Left, pair.Right)
		pairs = append(pairs, pair)
		seen[td.Event] = true
	}

	for _, td := range right {
		if seen[td.Event] {
			continue
		}
		pair := EventPair{
			Event:     td.Event,
			Right:     td,
			IsTypical: typicalEvents[td.Event],
		}
		pair.Items, pair.OnlyLeft, pair.OnlyRight = matchItems(nil, td.Items)

		rightDur := wallClockDuration([]*Timeline{pair.Right})
		pair.RightDuration = rightDur
		pair.DurationDelta = rightDur

		rewriteLinksForComparisonItems(&pair, owner, repo, compareType)
		pair.CombinedGantt = buildCompareGantt(pair.Left, pair.Right)
		pairs = append(pairs, pair)
	}

	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].IsTypical != pairs[j].IsTypical {
			return pairs[i].IsTypical
		}
		return false
	})

	return pairs
}

func rewriteLinksForComparisonItems(pair *EventPair, owner, repo, compareType string) {
	if compareType != "commit" && compareType != "workflow_run" {
		return
	}

	// Create a map of matched items by their name
	matchedLeft := make(map[string]string)
	matchedRight := make(map[string]string)
	for _, item := range pair.Items {
		matchedLeft[item.Name] = item.RightID
		matchedRight[item.Name] = item.LeftID
	}

	// Update left items
	if pair.Left != nil {
		for i, item := range pair.Left.Items {
			if rightID, ok := matchedLeft[item.Name]; ok {
				pair.Left.Items[i].Link = path.Join(
					"/",
					owner,
					repo,
					comparisonsOutputDir,
					fmt.Sprintf("%s_vs_%s.html", item.ID, rightID),
				)
			}
		}
	}

	// Update right items
	if pair.Right != nil {
		for i, item := range pair.Right.Items {
			if leftID, ok := matchedRight[item.Name]; ok {
				pair.Right.Items[i].Link = path.Join(
					"/",
					owner,
					repo,
					comparisonsOutputDir,
					fmt.Sprintf("%s_vs_%s.html", leftID, item.ID),
				)
			}
		}
	}
}

func formatDelta(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d > 0 {
		return "+" + d.String()
	}
	return d.String()
}

// collectWorkflowRunIDsFromObservation returns unique workflow run IDs from timeline items
// (used for commit observations where each item is a workflow run).
func collectWorkflowRunIDsFromObservation(obs *Observation) []int64 {
	if obs == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	var out []int64
	for _, td := range obs.TimelineData {
		for _, it := range td.Items {
			id, err := strconv.ParseInt(it.ID, 10, 64)
			if err != nil {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// EnsureCompareObservationLinks renders HTML for pages linked from the comparison Gantt
// and from linked workflow pages (workflow run + job run observations).
// Compare only writes the comparison file; without this, those links 404.
// Does not render or mutate the Left/Right observations used for the comparison chart.
//
// Commit compare: timeline items link to workflow run pages; each workflow's Gantt links to jobs.
// Workflow-run compare: timeline items link to job run pages only, but we render the full
// workflow run + jobs so navigation stays consistent with observe.
//
// It also recursively generates nested comparison pages for matched items (e.g. comparing
// matched workflow runs within a commit comparison).
func EnsureCompareObservationLinks(
	log zerolog.Logger,
	client *gather.GitHubClient,
	c *Comparison,
	opts ...Option,
) error {
	return ensureCompareObservationLinks(log, client, c, make(map[string]struct{}), opts...)
}

func ensureCompareObservationLinks(
	log zerolog.Logger,
	client *gather.GitHubClient,
	c *Comparison,
	seen map[string]struct{},
	opts ...Option,
) error {
	// Render standalone observation pages for parent items so that "only in left/right"
	// links and standard Gantt links still work.
	for _, obs := range []*Observation{c.Left, c.Right} {
		if obs == nil {
			continue
		}
		var wfIDs []int64
		switch obs.DataType {
		case "commit":
			wfIDs = collectWorkflowRunIDsFromObservation(obs)
		case "workflow_run":
			id, err := strconv.ParseInt(obs.ID, 10, 64)
			if err != nil {
				continue
			}
			wfIDs = []int64{id}
		default:
			continue
		}
		for _, wfID := range wfIDs {
			key := obs.Owner + "/" + obs.Repo + "/" + strconv.FormatInt(wfID, 10)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if err := renderWorkflowRunAndJobs(log, client, obs.Owner, obs.Repo, wfID, opts...); err != nil {
				return fmt.Errorf("workflow run %d: %w", wfID, err)
			}
		}
	}

	// Recursively generate nested comparison pages for matched items.
	for _, pair := range c.EventPairs {
		for _, item := range pair.Items {
			var childC *Comparison
			var err error

			switch c.CompareType {
			case "commit":
				leftWfID, errL := strconv.ParseInt(item.LeftID, 10, 64)
				rightWfID, errR := strconv.ParseInt(item.RightID, 10, 64)
				if errL == nil && errR == nil {
					childC, err = CompareWorkflowRuns(log, client, c.Owner, c.Repo, leftWfID, rightWfID, opts...)
				}
			case "workflow_run":
				leftWfID, errLW := strconv.ParseInt(c.Left.ID, 10, 64)
				rightWfID, errRW := strconv.ParseInt(c.Right.ID, 10, 64)
				leftJobID, errLJ := strconv.ParseInt(item.LeftID, 10, 64)
				rightJobID, errRJ := strconv.ParseInt(item.RightID, 10, 64)
				if errLW == nil && errRW == nil && errLJ == nil && errRJ == nil {
					childC, err = CompareJobRuns(
						log,
						client,
						c.Owner,
						c.Repo,
						leftWfID,
						rightWfID,
						leftJobID,
						rightJobID,
						opts...)
				}
			}

			if err != nil {
				return fmt.Errorf("failed to generate nested comparison for %s: %w", item.Name, err)
			}

			if childC != nil {
				// Prevent infinite recursion if data is somehow circular (unlikely in GH Actions, but safe)
				compKey := fmt.Sprintf("comp:%s/%s/%s_vs_%s", c.Owner, c.Repo, childC.Left.ID, childC.Right.ID)
				if _, ok := seen[compKey]; !ok {
					seen[compKey] = struct{}{}
					if _, err := childC.Render(log, "html"); err != nil {
						return fmt.Errorf("failed to render nested comparison for %s: %w", item.Name, err)
					}
					if err := ensureCompareObservationLinks(log, client, childC, seen, opts...); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func renderWorkflowRunAndJobs(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	workflowRunID int64,
	opts ...Option,
) error {
	wfObs, err := WorkflowRun(log, client, owner, repo, workflowRunID, opts...)
	if err != nil {
		return err
	}
	if _, err := wfObs.Render(log, "html"); err != nil {
		return err
	}
	jobs, err := JobRuns(log, client, owner, repo, workflowRunID, opts...)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if _, err := j.Render(log, "html"); err != nil {
			return fmt.Errorf("render job %s: %w", j.ID, err)
		}
	}
	return nil
}

// Render writes the comparison to a file in the given format ("html" or "md") and returns
// the output file path. For HTML the path is a URL-style path suitable for the browser;
// for markdown it is a filesystem path.
func (c *Comparison) Render(log zerolog.Logger, outputType string) (string, error) {
	if len(c.EventPairs) == 0 {
		c.EventPairs = buildEventPairs(c.Left.TimelineData, c.Right.TimelineData, c.Owner, c.Repo, c.CompareType)
	}

	tmpl, _, compareName := templateForFormat(outputType)
	baseDir := outputDirForFormat(outputType)
	fileName := fmt.Sprintf("%s_vs_%s.%s", c.Left.ID, c.Right.ID, outputType)

	targetFile := filepath.Join(baseDir, c.Owner, c.Repo, comparisonsOutputDir, fileName)

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, compareName, c); err != nil {
		return "", fmt.Errorf("failed to render comparison %s: %w", outputType, err)
	}

	if err := os.MkdirAll(filepath.Dir(targetFile), 0750); err != nil {
		return "", fmt.Errorf("failed to create comparison directory: %w", err)
	}
	if err := os.WriteFile(targetFile, buf.Bytes(), 0600); err != nil {
		return "", fmt.Errorf("failed to write comparison file: %w", err)
	}

	log.Info().Str("file", targetFile).Str("format", outputType).Msg("Rendered comparison")

	if outputType == "html" {
		return path.Join("/", c.Owner, c.Repo, comparisonsOutputDir, fileName), nil
	}
	return targetFile, nil
}
