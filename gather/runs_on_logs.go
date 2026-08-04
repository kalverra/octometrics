package gather

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

// RunsOnCostSummary holds parsed cost data from runs-on's "Execution Cost Summary" step.
type RunsOnCostSummary struct {
	InstanceType         string
	InstanceLifecycle    string
	Region               string
	Platform             string
	Arch                 string
	Az                   string
	ZoneID               string
	Duration             string
	CostUSD              float64
	GitHubEquivalentCost float64
	Savings              string
}

// CostInTenthsOfCent converts CostUSD to tenths-of-cent (matching existing convention).
func (s *RunsOnCostSummary) CostInTenthsOfCent() int64 {
	return int64(math.Round(s.CostUSD * 1000))
}

// costSummaryMarker is the marker that runs-on uses in job logs.
const costSummaryMarker = "## Execution Cost Summary"

// tableRowPattern matches markdown table rows: | key | value |
var tableRowPattern = regexp.MustCompile(`\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|`)

// ParseRunsOnCostSummary extracts the Execution Cost Summary table from job logs.
// Returns nil and false if the summary is not found.
func ParseRunsOnCostSummary(logs string) (*RunsOnCostSummary, bool) {
	_, after, ok := strings.Cut(logs, costSummaryMarker)
	if !ok {
		return nil, false
	}

	// Get the section after the marker
	section := after

	// Find the next ## heading or end of string to bound the section
	endIdx := strings.Index(section, "\n## ")
	if endIdx >= 0 {
		section = section[:endIdx]
	}

	summary := &RunsOnCostSummary{}
	foundAny := false

	for line := range strings.SplitSeq(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Skip header separator rows
		if strings.HasPrefix(line, "| -") || strings.HasPrefix(line, "| --") {
			continue
		}
		// Skip header row
		if strings.Contains(line, "metric") && strings.Contains(line, "value") {
			continue
		}

		matches := tableRowPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		key := strings.TrimSpace(matches[1])
		value := strings.TrimSpace(matches[2])

		foundAny = true
		switch key {
		case "Instance Type":
			summary.InstanceType = value
		case "Instance Lifecycle":
			summary.InstanceLifecycle = value
		case "Region":
			summary.Region = value
		case "Platform":
			summary.Platform = value
		case "Arch":
			summary.Arch = value
		case "Az":
			summary.Az = value
		case "Zone ID":
			summary.ZoneID = value
		case "Duration":
			summary.Duration = value
		case "Cost":
			summary.CostUSD = parseUSDValue(value)
		case "GitHub equivalent cost":
			summary.GitHubEquivalentCost = parseUSDValue(value)
		case "Savings":
			summary.Savings = value
		}
	}

	if !foundAny {
		return nil, false
	}

	return summary, true
}

// parseUSDValue parses a dollar value like "$0.0280" into a float64.
func parseUSDValue(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// fetchJobLogs downloads the logs for a single workflow job.
// Returns the raw log content as a string.
// The GitHub API returns a 302 redirect to a download URL.
func fetchJobLogs(client *GitHubClient, owner, repo string, jobID int64) (string, error) {
	ctx, cancel := ghCtx()
	defer cancel()

	logURL, resp, err := client.Rest.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 5)
	if err != nil {
		return "", fmt.Errorf("failed to get job logs URL for job %d: %w", jobID, err)
	}
	if resp.StatusCode != http.StatusFound {
		return "", fmt.Errorf(
			"expected status code %d for job logs redirect, got %d",
			http.StatusFound,
			resp.StatusCode,
		)
	}

	downloadResp, err := client.Rest.Client().Get(logURL.String())
	if err != nil {
		return "", fmt.Errorf("failed to download job logs for job %d: %w", jobID, err)
	}
	defer func() {
		if err := downloadResp.Body.Close(); err != nil {
			// Best effort close
			_ = err
		}
	}()

	if downloadResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"unexpected status code %d downloading job logs for job %d",
			downloadResp.StatusCode,
			jobID,
		)
	}

	body, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read job logs body for job %d: %w", jobID, err)
	}

	return string(body), nil
}

// fetchRunsOnCostFromLogs fetches job logs and parses the runs-on cost summary.
// Returns (cost in tenths-of-cent, parsed summary, error).
// Returns (0, nil, nil) if logs are fetched but no cost summary is found.
func fetchRunsOnCostFromLogs(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	jobID int64,
) (int64, *RunsOnCostSummary, error) {
	logs, err := fetchJobLogs(client, owner, repo, jobID)
	if err != nil {
		log.Trace().Err(err).Int64("job_id", jobID).Msg("failed to fetch job logs for runs-on cost")
		return 0, nil, err
	}

	summary, ok := ParseRunsOnCostSummary(logs)
	if !ok || summary == nil {
		return 0, nil, nil
	}

	return summary.CostInTenthsOfCent(), summary, nil
}
