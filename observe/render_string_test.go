package observe

import (
	"encoding/json"
	"strings"
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
	assert.Contains(t, md, "# Test Workflow")
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

func TestObservation_RenderString_LogsDir(t *testing.T) {
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
		LogsDir:    "/tmp/octometrics/logs/123",
	}

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)
	assert.Contains(t, md, "/tmp/octometrics/logs/123")

	html, err := obs.RenderString(log, "html")
	require.NoError(t, err)
	assert.Contains(t, html, "/tmp/octometrics/logs/123")
}

func TestObservation_RenderString_JSON(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	obs := &Observation{
		ID:         "12345",
		Name:       "JSON Workflow",
		GitHubLink: "https://github.com/kalverra/octometrics/actions/runs/12345",
		Owner:      "kalverra",
		Repo:       "octometrics",
		DataType:   "workflow_run",
		State:      "success",
		Actor:      "kalverra",
		CriticalPath: &CriticalPathInfo{
			TotalDuration: 120 * time.Second,
			CriticalNodes: []CriticalPathNode{
				{JobID: 99, JobName: "Build", Duration: 120 * time.Second, IsCritical: true},
			},
		},
	}

	jsonStr, err := obs.RenderString(log, "json")
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal([]byte(jsonStr), &raw)
	require.NoError(t, err, "output should be valid JSON")
	assert.Equal(t, "12345", raw["ID"])
	assert.Equal(t, "JSON Workflow", raw["Name"])
	assert.NotNil(t, raw["critical_path"])
}

func TestObservation_RenderString_NoHTMLEscapingInMarkdown(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	obs := &Observation{
		ID:         "123",
		Name:       "Test & Workflow \"Quotes\"",
		GitHubLink: "https://github.com/kalverra/octometrics/actions/runs/123",
		Owner:      "kalverra",
		Repo:       "octometrics",
		DataType:   "workflow_run",
		State:      "success",
		Actor:      "kalverra",
		FlowChart:  "flowchart TD\n  A --> B",
	}

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)

	assert.NotContains(t, md, "&amp;", "Markdown output should not contain &amp;")
	assert.NotContains(t, md, "&#34;", "Markdown output should not contain &#34;")
	assert.NotContains(t, md, "--&gt;", "Markdown output should not contain --&gt;")
	assert.Contains(t, md, "-->", "Markdown output should contain raw --> arrow")
}

func TestObservation_RenderString_AnalyticsSections(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	obs := &Observation{
		ID:         "555",
		Name:       "Analytics Workflow",
		GitHubLink: "https://github.com/kalverra/octometrics/actions/runs/555",
		Owner:      "kalverra",
		Repo:       "octometrics",
		DataType:   "workflow_run",
		State:      "success",
		Actor:      "kalverra",
		CriticalPath: &CriticalPathInfo{
			TotalDuration:  300 * time.Second,
			TotalExecution: 280 * time.Second,
			TotalQueue:     20 * time.Second,
			CriticalNodes: []CriticalPathNode{
				{
					JobID:      101,
					JobName:    "Build",
					Duration:   100 * time.Second,
					QueueTime:  5 * time.Second,
					IsCritical: true,
				},
				{
					JobID:        102,
					JobName:      "Test Matrix",
					Duration:     180 * time.Second,
					QueueTime:    15 * time.Second,
					IsCritical:   true,
					BlockingNeed: "Build",
				},
			},
		},
		StepSummaries: []StepSummary{
			{
				Name:           "Set up Go",
				Count:          10,
				TotalDuration:  150 * time.Second,
				MedianDuration: 15 * time.Second,
				MaxDuration:    18 * time.Second,
				PctTotal:       50.0,
			},
		},
		SlowestJobSteps: []JobStepBreakdown{
			{
				JobID:    102,
				JobName:  "Test Matrix",
				Duration: 180 * time.Second,
				Steps: []StepDetail{
					{Name: "Set up Go", Duration: 15 * time.Second},
					{Name: "Run tests", Duration: 165 * time.Second},
				},
			},
		},
	}

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)
	assert.Contains(t, md, "Critical Path", "Markdown should contain Critical Path section")
	assert.Contains(t, md, "Step Aggregation", "Markdown should contain Step Aggregation section")
	assert.Contains(t, md, "Set up Go", "Markdown should contain step summary")
	assert.Contains(t, md, "Test Matrix", "Markdown should contain slowest job step breakdown")

	html, err := obs.RenderString(log, "html")
	require.NoError(t, err)
	assert.Contains(t, html, "Critical path", "HTML should contain Critical path section")
	assert.Contains(t, html, "Step aggregation", "HTML should contain Step aggregation section")
}

func TestObservation_RenderString_AnalyticsUnderEventSectionPreCollapsed(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	timeline := &Timeline{
		Event:         "workflow_dispatch",
		Duration:      300 * time.Second,
		RealStartTime: time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC),
		RealEndTime:   time.Date(2026, 8, 11, 20, 5, 0, 0, time.UTC),
		Items: []TimelineItem{
			{Name: "Build", Duration: 100 * time.Second},
		},
		CriticalPath: &CriticalPathInfo{
			TotalDuration: 300 * time.Second,
			CriticalNodes: []CriticalPathNode{
				{JobID: 1, JobName: "Build", Duration: 100 * time.Second, IsCritical: true},
			},
		},
		StepSummaries: []StepSummary{
			{Name: "Set up Go", Count: 1, TotalDuration: 10 * time.Second, PctTotal: 10.0},
		},
		SlowestJobSteps: []JobStepBreakdown{
			{
				JobID:    1,
				JobName:  "Build",
				Duration: 100 * time.Second,
				Steps:    []StepDetail{{Name: "Compile", Duration: 90 * time.Second}},
			},
		},
	}

	obs := &Observation{
		ID:           "999",
		Name:         "Event Analytics Workflow",
		DataType:     "workflow_run",
		TimelineData: []*Timeline{timeline},
	}

	html, err := obs.RenderString(log, "html")
	require.NoError(t, err)

	// Ensure Step aggregation and Critical path are pre-collapsed (not open at the panel level)
	assert.NotContains(t, html, `<details class="details-panel" open>`, "No analytics panel should have open attribute")
	assert.Contains(t, html, "Step aggregation across matrix", "Step aggregation should be present")
	assert.Contains(t, html, "Critical path", "Critical path should be present")

	// Ensure all analytics sections are inside event section (after workflow_dispatch summary)
	eventSummaryIdx := strings.Index(html, "workflow_dispatch")
	stepAggIdx := strings.Index(html, "Step aggregation across matrix")
	critPathIdx := strings.Index(html, "Critical path")
	slowestIdx := strings.Index(html, "Top slowest jobs step breakdown")

	require.GreaterOrEqual(t, eventSummaryIdx, 0, "HTML should contain event summary")
	require.Greater(t, stepAggIdx, eventSummaryIdx, "Step aggregation must appear under event section")
	require.Greater(t, critPathIdx, eventSummaryIdx, "Critical path must appear under event section")
	require.Greater(t, slowestIdx, eventSummaryIdx, "Top slowest jobs must appear under event section")

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)
	mdEventIdx := strings.Index(md, "workflow_dispatch")
	mdStepAggIdx := strings.Index(md, "Step Aggregation Across Matrix")
	mdCritPathIdx := strings.Index(md, "Critical Path")
	mdSlowestIdx := strings.Index(md, "Top Slowest Jobs Step Breakdown")

	require.GreaterOrEqual(t, mdEventIdx, 0, "MD should contain event section")
	require.Greater(t, mdStepAggIdx, mdEventIdx, "MD Step aggregation must appear under event section")
	require.Greater(t, mdCritPathIdx, mdEventIdx, "MD Critical path must appear under event section")
	require.Greater(t, mdSlowestIdx, mdEventIdx, "MD Top slowest jobs must appear under event section")
}

func TestObservation_RenderString_RealTimestampsInHeader(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	realStart := time.Date(2026, 8, 11, 20, 48, 0, 0, time.UTC)
	realEnd := time.Date(2026, 8, 11, 20, 59, 0, 0, time.UTC)

	timeline := &Timeline{
		Event:         "pull_request",
		RealStartTime: realStart,
		RealEndTime:   realEnd,
		Duration:      11 * time.Minute,
		Items: []TimelineItem{
			{Name: "Job 1", StartTime: realStart, Duration: 11 * time.Minute},
		},
	}
	require.NoError(t, timeline.normalize())

	obs := &Observation{
		ID:           "777",
		Name:         "PR Run",
		DataType:     "workflow_run",
		TimelineData: []*Timeline{timeline},
	}

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)
	assert.Contains(t, md, "2026-08-11T20:48:00")
	assert.NotContains(t, md, "2026-08-11T00:00:00")
}

func TestEventSection_HTMLAutoCollapsedDetailsAndJobLinks(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	timeline := &Timeline{
		Event:         "push",
		Duration:      300 * time.Second,
		RealStartTime: time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC),
		RealEndTime:   time.Date(2026, 8, 12, 10, 5, 0, 0, time.UTC),
		Items: []TimelineItem{
			{Name: "Build", JobID: 101, Duration: 100 * time.Second, Link: "/owner/repo/job_runs/101.html"},
		},
		CriticalPath: &CriticalPathInfo{
			TotalDuration: 300 * time.Second,
			CriticalNodes: []CriticalPathNode{
				{JobID: 101, JobName: "Build", Duration: 100 * time.Second, IsCritical: true},
			},
		},
		SlowestJobSteps: []JobStepBreakdown{
			{
				JobID:    101,
				JobName:  "Build",
				Duration: 100 * time.Second,
				Steps:    []StepDetail{{Name: "Compile", Duration: 90 * time.Second}},
			},
		},
	}

	obs := &Observation{
		ID:           "888",
		Name:         "Event UI Test Workflow",
		Owner:        "owner",
		Repo:         "repo",
		DataType:     "workflow_run",
		TimelineData: []*Timeline{timeline},
	}

	html, err := obs.RenderString(log, "html")
	require.NoError(t, err)

	// 1. Details section auto-collapsed (no open attribute on details-group)
	assert.NotContains(
		t,
		html,
		`<details class="section details-group" open>`,
		"Details group should be auto-collapsed without open attribute",
	)
	assert.Contains(
		t,
		html,
		`<details class="section details-group">`,
		"Details group should exist without open attribute",
	)

	// 2. Clickable job links in Critical Path and Top Slowest Jobs
	assert.Contains(
		t,
		html,
		`<a href="/owner/repo/job_runs/101.html">Build</a>`,
		"Job name in Critical Path / Slowest Jobs should link to octometrics page",
	)
}

func TestEventSection_MarkdownPureNoHTMLNoLinks(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	timeline := &Timeline{
		Event:         "push",
		Duration:      300 * time.Second,
		RealStartTime: time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC),
		RealEndTime:   time.Date(2026, 8, 12, 10, 5, 0, 0, time.UTC),
		Items: []TimelineItem{
			{Name: "Build", JobID: 101, Duration: 100 * time.Second},
		},
	}

	obs := &Observation{
		ID:           "888",
		Name:         "Event UI Test Workflow",
		GitHubLink:   "https://github.com/owner/repo/actions/runs/888",
		Owner:        "owner",
		Repo:         "repo",
		DataType:     "workflow_run",
		TimelineData: []*Timeline{timeline},
	}

	md, err := obs.RenderString(log, "md")
	require.NoError(t, err)

	// Pure markdown: no HTML tags (like <details>, <a>, etc.) and no clickable links ([text](url))
	assert.NotContains(t, md, "<details>", "Markdown output should have no HTML tags")
	assert.NotContains(t, md, "<a ", "Markdown output should have no HTML <a> tags")
	assert.NotContains(t, md, "[Event UI Test Workflow](", "Markdown output should have no clickable markdown links")
}
