package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatDurationCompact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "00m 00s",
		},
		{
			name:     "only seconds",
			duration: 45 * time.Second,
			expected: "00m 45s",
		},
		{
			name:     "exactly one minute",
			duration: 1 * time.Minute,
			expected: "01m 00s",
		},
		{
			name:     "minutes and seconds",
			duration: 5*time.Minute + 12*time.Second,
			expected: "05m 12s",
		},
		{
			name:     "more than an hour",
			duration: 2*time.Hour + 3*time.Minute + 4*time.Second,
			expected: "123m 04s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, formatDurationCompact(tt.duration))
		})
	}
}

func TestFormatDelta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero delta",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "positive delta (slower)",
			duration: 45 * time.Second,
			expected: "+45s",
		},
		{
			name:     "positive delta with minutes",
			duration: 2*time.Minute + 30*time.Second,
			expected: "+2m30s",
		},
		{
			name:     "negative delta (faster)",
			duration: -15 * time.Second,
			expected: "-15s",
		},
		{
			name:     "negative delta with minutes",
			duration: -(1*time.Minute + 5*time.Second),
			expected: "-1m5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, formatDelta(tt.duration))
		})
	}
}

func TestHandlers_MissingArgs(t *testing.T) {
	t.Parallel()

	h := &serverHandler{
		log:    zerolog.Nop(),
		client: nil,
	}

	tests := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		req     mcp.CallToolRequest
	}{
		{
			name:    "getWorkflowSummary missing owner",
			handler: h.handleGetWorkflowSummary,
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "getWorkflowSummary",
					Arguments: map[string]any{"repo": "r", "run_id": float64(1)},
				},
			},
		},
		{
			name:    "getJobTimeline missing job_id",
			handler: h.handleGetJobTimeline,
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "getJobTimeline",
					Arguments: map[string]any{"owner": "o", "repo": "r", "run_id": float64(1)},
				},
			},
		},
		{
			name:    "getPerformanceMetrics missing run_id",
			handler: h.handleGetPerformanceMetrics,
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "getPerformanceMetrics",
					Arguments: map[string]any{"owner": "o", "repo": "r", "job_id": float64(1)},
				},
			},
		},
		{
			name:    "compareRuns missing right_id",
			handler: h.handleCompareRuns,
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "compareRuns",
					Arguments: map[string]any{"owner": "o", "repo": "r", "left_id": float64(1)},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := tt.handler(context.Background(), tt.req)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.True(t, res.IsError)
			require.Len(t, res.Content, 1)
			require.IsType(t, mcp.TextContent{}, res.Content[0])
			assert.Contains(t, res.Content[0].(mcp.TextContent).Text, "Failed to get")
		})
	}
}
