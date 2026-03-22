// Package mcp implements the MCP server for Octometrics.
package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/observe"
)

// Observer defines the interface for building observations and comparisons.
type Observer interface {
	WorkflowRun(
		log zerolog.Logger,
		client *gather.GitHubClient,
		owner, repo string,
		runID int64,
		opts ...observe.Option,
	) (*observe.Observation, error)
	JobRuns(
		log zerolog.Logger,
		client *gather.GitHubClient,
		owner, repo string,
		runID int64,
		opts ...observe.Option,
	) ([]*observe.Observation, error)
	CompareWorkflowRuns(
		log zerolog.Logger,
		client *gather.GitHubClient,
		owner, repo string,
		leftID, rightID int64,
		opts ...observe.Option,
	) (*observe.Comparison, error)
	ListWorkflowRuns(
		log zerolog.Logger,
		client *gather.GitHubClient,
		owner, repo string,
		since, until time.Time,
		event string,
	) ([]*github.WorkflowRun, error)
}

// DefaultObserver is the default implementation that calls the observe package.
type DefaultObserver struct{}

// WorkflowRun gathers and processes a workflow run into an observation.
func (d *DefaultObserver) WorkflowRun(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	runID int64,
	opts ...observe.Option,
) (*observe.Observation, error) {
	return observe.WorkflowRun(log, client, owner, repo, runID, opts...)
}

// JobRuns observes all job runs for a given workflow run.
func (d *DefaultObserver) JobRuns(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	runID int64,
	opts ...observe.Option,
) ([]*observe.Observation, error) {
	return observe.JobRuns(log, client, owner, repo, runID, opts...)
}

// CompareWorkflowRuns builds a comparison between two workflow runs.
func (d *DefaultObserver) CompareWorkflowRuns(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	leftID, rightID int64,
	opts ...observe.Option,
) (*observe.Comparison, error) {
	return observe.CompareWorkflowRuns(log, client, owner, repo, leftID, rightID, opts...)
}

// ListWorkflowRuns lists workflow runs for a repository within a given time range.
func (d *DefaultObserver) ListWorkflowRuns(
	_ zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	since, until time.Time,
	event string,
) ([]*github.WorkflowRun, error) {
	if client == nil {
		return nil, fmt.Errorf("GitHub client is nil")
	}

	if event == "all" {
		event = ""
	}

	createdFilter := fmt.Sprintf("%s..%s", since.Format("2006-01-02"), until.Format("2006-01-02"))
	var (
		allRuns  []*github.WorkflowRun
		listOpts = &github.ListWorkflowRunsOptions{
			Created: createdFilter,
			Event:   event,
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}
	)

	for {
		// Using a simplified context here as we don't have access to the internal ghCtx()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		runs, resp, err := client.Rest.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, listOpts)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to list workflow runs: %w", err)
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("unexpected status code %d listing workflow runs", resp.StatusCode)
		}

		allRuns = append(allRuns, runs.WorkflowRuns...)
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return allRuns, nil
}

type serverHandler struct {
	log      zerolog.Logger
	client   *gather.GitHubClient
	observer Observer
}

// Server starts the MCP server over stdio.
func Server(log zerolog.Logger, client *gather.GitHubClient, obs Observer) error {
	s := server.NewMCPServer(
		"octometrics",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	h := &serverHandler{
		log:      log,
		client:   client,
		observer: obs,
	}

	// Tool: get_workflow_summary
	s.AddTool(mcp.NewTool(
		"get_workflow_summary",
		mcp.WithDescription(
			"Get a high-level summary of a GitHub Actions workflow run, including duration, cost, and a list of jobs.",
		),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		mcp.WithNumber("run_id", mcp.Required(), mcp.Description("Workflow run ID")),
	), h.handleGetWorkflowSummary)

	// Tool: get_job_timeline
	s.AddTool(mcp.NewTool("get_job_timeline",
		mcp.WithDescription("Get the step-by-step trace of a specific job with relative start times."),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		mcp.WithNumber("run_id", mcp.Required(), mcp.Description("Workflow run ID containing the job")),
		mcp.WithNumber("job_id", mcp.Required(), mcp.Description("Job ID")),
	), h.handleGetJobTimeline)

	// Tool: get_performance_metrics
	s.AddTool(mcp.NewTool(
		"get_performance_metrics",
		mcp.WithDescription(
			"Get textual summaries of performance monitoring metrics (CPU, Memory, Disk, I/O) for a job run.",
		),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		mcp.WithNumber("run_id", mcp.Required(), mcp.Description("Workflow run ID containing the job")),
		mcp.WithNumber("job_id", mcp.Required(), mcp.Description("Job ID")),
	), h.handleGetPerformanceMetrics)

	// Tool: compare_runs
	s.AddTool(mcp.NewTool("compare_runs",
		mcp.WithDescription("Compare two workflow runs and get the duration and status deltas for each job."),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		mcp.WithNumber("left_id", mcp.Required(), mcp.Description("Left Workflow run ID")),
		mcp.WithNumber("right_id", mcp.Required(), mcp.Description("Right Workflow run ID")),
	), h.handleCompareRuns)

	// Tool: list_workflow_runs
	s.AddTool(mcp.NewTool("list_workflow_runs",
		mcp.WithDescription("List workflow runs within a certain time frame."),
		mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		mcp.WithString("from", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
		mcp.WithString("to", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		mcp.WithString("event", mcp.Description("Filter by event (all, pull_request, merge_group, push)")),
	), h.handleListWorkflowRuns)

	return server.ServeStdio(s)
}

func (h *serverHandler) handleGetWorkflowSummary(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	owner, err := request.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get owner: %v", err)), nil
	}
	repo, err := request.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get repo: %v", err)), nil
	}
	runID, err := request.RequireInt("run_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get run ID: %v", err)), nil
	}

	obs, err := h.observer.WorkflowRun(h.log, h.client, owner, repo, int64(runID))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get workflow run: %v", err)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Workflow Run: %s (ID: %s)\n", obs.Name, obs.ID)
	fmt.Fprintf(&b, "State: %s\n", obs.State)
	if obs.Cost > 0 {
		fmt.Fprintf(&b, "Cost: $%.2f\n", float64(obs.Cost)/1000.0)
	}

	if len(obs.TimelineData) > 0 {
		td := obs.TimelineData[0]
		fmt.Fprintf(&b, "Total Duration: %s\n\n", td.Duration)
		fmt.Fprintf(&b, "Jobs:\n")
		for _, item := range td.Items {
			fmt.Fprintf(&b, "  - %s (ID: %s) [%s]: %s\n", item.Name, item.ID, item.Conclusion, item.Duration)
		}
	}

	return mcp.NewToolResultText(b.String()), nil
}

func (h *serverHandler) handleGetJobTimeline(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	owner, err := request.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get owner: %v", err)), nil
	}
	repo, err := request.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get repo: %v", err)), nil
	}
	runID, err := request.RequireInt("run_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get run ID: %v", err)), nil
	}
	jobID, err := request.RequireInt("job_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get job ID: %v", err)), nil
	}

	jobs, err := h.observer.JobRuns(h.log, h.client, owner, repo, int64(runID))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get jobs: %v", err)), nil
	}

	var targetJob *observe.Observation
	for _, j := range jobs {
		if j.ID == fmt.Sprintf("%d", jobID) {
			targetJob = j
			break
		}
	}

	if targetJob == nil {
		return mcp.NewToolResultError(fmt.Sprintf("Job ID %d not found in workflow run %d", jobID, runID)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Job: %s (Total: ", targetJob.Name)

	if len(targetJob.TimelineData) > 0 {
		td := targetJob.TimelineData[0]
		fmt.Fprintf(&b, "%s)\nStatus: %s\nSteps:\n", td.Duration, targetJob.State)
		for _, item := range td.Items {
			// We want relative offset. td.StartTime is normalized to 0 by process(),
			// so item.StartTime's time part is exactly the offset.
			offset := item.StartTime.Sub(td.StartTime)
			startStr := formatDurationCompact(offset)
			endStr := formatDurationCompact(offset + item.Duration)

			conclusion := ""
			if item.Conclusion != "" && item.Conclusion != "success" {
				conclusion = fmt.Sprintf(" (%s)", strings.ToUpper(item.Conclusion))
			}
			fmt.Fprintf(&b, "  [%s - %s] %s%s\n", startStr, endStr, item.Name, conclusion)
		}
	} else {
		fmt.Fprintf(&b, "0s)\nStatus: %s\nNo steps found.\n", targetJob.State)
	}

	return mcp.NewToolResultText(b.String()), nil
}

func (h *serverHandler) handleGetPerformanceMetrics(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	owner, err := request.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get owner: %v", err)), nil
	}
	repo, err := request.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get repo: %v", err)), nil
	}
	runID, err := request.RequireInt("run_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get run ID: %v", err)), nil
	}
	jobID, err := request.RequireInt("job_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get job ID: %v", err)), nil
	}

	jobs, err := h.observer.JobRuns(h.log, h.client, owner, repo, int64(runID))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get jobs: %v", err)), nil
	}

	var targetJob *observe.Observation
	for _, j := range jobs {
		if j.ID == fmt.Sprintf("%d", jobID) {
			targetJob = j
			break
		}
	}

	if targetJob == nil {
		return mcp.NewToolResultError(fmt.Sprintf("Job ID %d not found in workflow run %d", jobID, runID)), nil
	}

	if targetJob.MonitoringData == nil || len(targetJob.MonitoringData.Charts) == 0 {
		return mcp.NewToolResultText("No performance monitoring data available for this job."), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Performance Metrics for Job %s:\n\n", targetJob.Name)

	// The charts contain the mermaid diagram strings, which include peak/avg data in the titles/legends
	// We'll extract the title and attempt to summarize.
	for _, chart := range targetJob.MonitoringData.Charts {
		// Title usually looks like "CPU Usage %"
		// But wait, the title is just a string, and the actual peak data is embedded in the diagram string or we can extract it.
		// Let's just output the titles and maybe peak info if we can parse it, or we can just return the raw xychart string
		// since it is token-efficient compared to raw json.
		fmt.Fprintf(&b, "--- %s ---\n", chart.Title)
		// Let's just include the mermaid chart since xychart is relatively token-compact (downsampled to 40 points)
		fmt.Fprintf(&b, "%s\n\n", chart.Diagram)
	}

	return mcp.NewToolResultText(b.String()), nil
}

func (h *serverHandler) handleCompareRuns(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	owner, err := request.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get owner: %v", err)), nil
	}
	repo, err := request.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get repo: %v", err)), nil
	}
	leftID, err := request.RequireInt("left_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get left ID: %v", err)), nil
	}
	rightID, err := request.RequireInt("right_id")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get right ID: %v", err)), nil
	}

	comp, err := h.observer.CompareWorkflowRuns(h.log, h.client, owner, repo, int64(leftID), int64(rightID))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to compare runs: %v", err)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Comparison: %s vs %s\n", comp.Left.Name, comp.Right.Name)
	fmt.Fprintf(
		&b,
		"Overall Duration: %s -> %s (Delta: %s)\n\n",
		comp.Summary.LeftDuration,
		comp.Summary.RightDuration,
		formatDelta(comp.Summary.DurationDelta),
	)

	for _, pair := range comp.EventPairs {
		fmt.Fprintf(&b, "Event: %s\n", pair.Event)

		if len(pair.Items) > 0 {
			fmt.Fprintf(&b, "  Matched Jobs:\n")
			// Already sorted by absolute duration delta
			for _, item := range pair.Items {
				statusNote := ""
				if item.StatusChanged {
					statusNote = fmt.Sprintf(" [STATUS: %s -> %s]", item.LeftConclusion, item.RightConclusion)
				}
				fmt.Fprintf(
					&b,
					"    - %s: %s -> %s (Delta: %s)%s\n",
					item.Name,
					item.LeftDuration,
					item.RightDuration,
					formatDelta(item.DurationDelta),
					statusNote,
				)
			}
		}
		if len(pair.OnlyLeft) > 0 {
			fmt.Fprintf(&b, "  Only in Left:\n")
			for _, item := range pair.OnlyLeft {
				fmt.Fprintf(&b, "    - %s (%s) [%s]\n", item.Name, item.Duration, item.Conclusion)
			}
		}
		if len(pair.OnlyRight) > 0 {
			fmt.Fprintf(&b, "  Only in Right:\n")
			for _, item := range pair.OnlyRight {
				fmt.Fprintf(&b, "    - %s (%s) [%s]\n", item.Name, item.Duration, item.Conclusion)
			}
		}
		fmt.Fprintln(&b)
	}

	return mcp.NewToolResultText(b.String()), nil
}

func (h *serverHandler) handleListWorkflowRuns(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	owner, err := request.RequireString("owner")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get owner: %v", err)), nil
	}
	repo, err := request.RequireString("repo")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get repo: %v", err)), nil
	}
	fromStr, err := request.RequireString("from")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get from date: %v", err)), nil
	}
	toStr, err := request.RequireString("to")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get to date: %v", err)), nil
	}
	event := request.GetString("event", "all")

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid from date format (expected YYYY-MM-DD): %v", err)), nil
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid to date format (expected YYYY-MM-DD): %v", err)), nil
	}

	runs, err := h.observer.ListWorkflowRuns(h.log, h.client, owner, repo, from, to, event)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list workflow runs: %v", err)), nil
	}

	if len(runs) == 0 {
		return mcp.NewToolResultText("No workflow runs found in this range."), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d workflow runs:\n\n", len(runs))
	for _, run := range runs {
		conclusion := run.GetConclusion()
		if conclusion == "" {
			conclusion = run.GetStatus()
		}
		fmt.Fprintf(&b, "  - %s (ID: %d) [%s] - Created: %s\n",
			run.GetName(),
			run.GetID(),
			conclusion,
			run.GetCreatedAt().Format("2006-01-02 15:04"),
		)
	}

	return mcp.NewToolResultText(b.String()), nil
}

func formatDurationCompact(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02dm %02ds", m, s)
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
