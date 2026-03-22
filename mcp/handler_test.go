package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/observe"
	"github.com/kalverra/octometrics/report"
)

func TestHandleGetWorkflowSummary(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	sampleObs := &observe.Observation{
		ID:    "123",
		Name:  "CI Workflow",
		State: "success",
		Cost:  1500, // $1.50
		TimelineData: []*observe.Timeline{
			{
				Duration: 5 * time.Minute,
				Items: []observe.TimelineItem{
					{
						Name:       "build",
						ID:         "456",
						Conclusion: "success",
						Duration:   3 * time.Minute,
					},
				},
			},
		},
	}

	obsMock.On("WorkflowRun", mock.Anything, mock.Anything, "owner", "repo", int64(123)).Return(sampleObs, nil)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_workflow_summary",
			Arguments: map[string]any{
				"owner":  "owner",
				"repo":   "repo",
				"run_id": float64(123),
			},
		},
	}

	res, err := h.handleGetWorkflowSummary(context.Background(), req)
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, res.Content, 1)

	text := res.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "Workflow Run: CI Workflow (ID: 123)")
	assert.Contains(t, text, "State: success")
	assert.Contains(t, text, "Cost: $1.50")
	assert.Contains(t, text, "Total Duration: 5m0s")
	assert.Contains(t, text, "- build (ID: 456) [success]: 3m0s")
}

func TestHandleGetJobTimeline(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	startTime := time.Now()
	sampleJobs := []*observe.Observation{
		{
			ID:    "456",
			Name:  "build",
			State: "success",
			TimelineData: []*observe.Timeline{
				{
					StartTime: startTime,
					Duration:  3 * time.Minute,
					Items: []observe.TimelineItem{
						{
							Name:      "Setup",
							StartTime: startTime.Add(10 * time.Second),
							Duration:  20 * time.Second,
						},
						{
							Name:       "Test",
							StartTime:  startTime.Add(30 * time.Second),
							Duration:   2 * time.Minute,
							Conclusion: "failure",
						},
					},
				},
			},
		},
	}

	obsMock.On("JobRuns", mock.Anything, mock.Anything, "owner", "repo", int64(123)).Return(sampleJobs, nil)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_job_timeline",
			Arguments: map[string]any{
				"owner":  "owner",
				"repo":   "repo",
				"run_id": float64(123),
				"job_id": float64(456),
			},
		},
	}

	res, err := h.handleGetJobTimeline(context.Background(), req)
	require.NoError(t, err)
	require.False(t, res.IsError)

	text := res.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "Job: build (Total: 3m0s)")
	assert.Contains(t, text, "[00m 10s - 00m 30s] Setup")
	assert.Contains(t, text, "[00m 30s - 02m 30s] Test (FAILURE)")
}

func TestHandleCompareRuns(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	sampleComp := &observe.Comparison{
		Left:  &observe.Observation{Name: "Run 1"},
		Right: &observe.Observation{Name: "Run 2"},
		Summary: observe.ComparisonSummary{
			LeftDuration:  10 * time.Minute,
			RightDuration: 12 * time.Minute,
			DurationDelta: 2 * time.Minute,
		},
		EventPairs: []observe.EventPair{
			{
				Event: "push",
				Items: []observe.ComparisonItem{
					{
						Name:            "build",
						LeftDuration:    5 * time.Minute,
						RightDuration:   6 * time.Minute,
						DurationDelta:   1 * time.Minute,
						StatusChanged:   true,
						LeftConclusion:  "failure",
						RightConclusion: "success",
					},
				},
			},
		},
	}

	obsMock.On("CompareWorkflowRuns", mock.Anything, mock.Anything, "owner", "repo", int64(1), int64(2)).
		Return(sampleComp, nil)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "compare_runs",
			Arguments: map[string]any{
				"owner":    "owner",
				"repo":     "repo",
				"left_id":  float64(1),
				"right_id": float64(2),
			},
		},
	}

	res, err := h.handleCompareRuns(context.Background(), req)
	require.NoError(t, err)
	require.False(t, res.IsError)

	text := res.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "Comparison: Run 1 vs Run 2")
	assert.Contains(t, text, "Overall Duration: 10m0s -> 12m0s (Delta: +2m0s)")
	assert.Contains(t, text, "Event: push")
	assert.Contains(t, text, "build: 5m0s -> 6m0s (Delta: +1m0s) [STATUS: failure -> success]")
}

func TestHandleGetPerformanceMetrics(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	sampleJobs := []*observe.Observation{
		{
			ID:   "456",
			Name: "build",
			MonitoringData: &observe.Monitoring{
				Charts: []report.MonitoringChart{
					{
						Title:   "CPU Usage %",
						Diagram: "xychart-beta ...",
					},
				},
			},
		},
	}

	obsMock.On("JobRuns", mock.Anything, mock.Anything, "owner", "repo", int64(123)).Return(sampleJobs, nil)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_performance_metrics",
			Arguments: map[string]any{
				"owner":  "owner",
				"repo":   "repo",
				"run_id": float64(123),
				"job_id": float64(456),
			},
		},
	}

	res, err := h.handleGetPerformanceMetrics(context.Background(), req)
	require.NoError(t, err)
	require.False(t, res.IsError)

	text := res.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "Performance Metrics for Job build:")
	assert.Contains(t, text, "--- CPU Usage % ---")
	assert.Contains(t, text, "xychart-beta ...")
}

func TestHandlers_ObserverErrors(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	obsMock.On("WorkflowRun", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_workflow_summary",
			Arguments: map[string]any{
				"owner":  "o",
				"repo":   "r",
				"run_id": float64(1),
			},
		},
	}

	res, err := h.handleGetWorkflowSummary(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content[0].(mcp.TextContent).Text, "Failed to get workflow run")
}

func TestHandleGetJobTimeline_NotFound(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	obsMock.On("JobRuns", mock.Anything, mock.Anything, "o", "r", int64(1)).Return([]*observe.Observation{}, nil)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_job_timeline",
			Arguments: map[string]any{
				"owner":  "o",
				"repo":   "r",
				"run_id": float64(1),
				"job_id": float64(999),
			},
		},
	}

	res, err := h.handleGetJobTimeline(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content[0].(mcp.TextContent).Text, "Job ID 999 not found")
}

func TestHandleListWorkflowRuns(t *testing.T) {
	t.Parallel()

	obsMock := NewMockObserver(t)
	h := &serverHandler{
		log:      zerolog.Nop(),
		client:   nil,
		observer: obsMock,
	}

	from, _ := time.Parse("2006-01-02", "2025-01-01")
	to, _ := time.Parse("2006-01-02", "2025-01-07")

	sampleRuns := []*github.WorkflowRun{
		{
			ID:         new(int64(123)),
			Name:       new("CI"),
			Status:     new("completed"),
			Conclusion: new("success"),
			CreatedAt:  &github.Timestamp{Time: from},
		},
	}

	obsMock.On("ListWorkflowRuns", mock.Anything, mock.Anything, "owner", "repo", from, to, "pull_request").
		Return(sampleRuns, nil)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "list_workflow_runs",
			Arguments: map[string]any{
				"owner": "owner",
				"repo":  "repo",
				"from":  "2025-01-01",
				"to":    "2025-01-07",
				"event": "pull_request",
			},
		},
	}

	res, err := h.handleListWorkflowRuns(context.Background(), req)
	require.NoError(t, err)
	require.False(t, res.IsError)

	text := res.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "Found 1 workflow runs:")
	assert.Contains(t, text, "- CI (ID: 123) [success]")
}
