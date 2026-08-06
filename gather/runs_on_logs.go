package gather

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var unauthenticatedHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

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
const costSummaryMarker = "Execution Cost Summary"

var logTimestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s`)

// stripLogTimestamp removes the GitHub Actions log timestamp prefix from a line.
func stripLogTimestamp(line string) string {
	return logTimestampPattern.ReplaceAllString(line, "")
}

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

	// Find the next heading or end of string to bound the section
	// Lines may have timestamp prefixes, so search for "## " on its own
	restIdx := strings.Index(section, "\n")
	if restIdx >= 0 {
		rest := section[restIdx:]
		nextHeading := strings.Index(rest[1:], "## ")
		if nextHeading >= 0 {
			section = section[:restIdx+1+nextHeading]
		}
	}

	summary := &RunsOnCostSummary{}
	foundAny := false

	for line := range strings.SplitSeq(section, "\n") {
		line = stripLogTimestamp(strings.TrimSpace(line))
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
func fetchJobLogs(parentCtx context.Context, client *GitHubClient, owner, repo string, jobID int64) (string, error) {
	ctx, cancel := ghCtx(parentCtx)
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

	// 1. Try a tail Range request first (256 KiB)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create log request for job %d: %w", jobID, err)
	}
	req.Header.Set("Range", "bytes=-262144")

	downloadResp, err := unauthenticatedHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download job logs for job %d: %w", jobID, err)
	}
	bodyBytes, err := readAllLimited(downloadResp.Body, maxMonitorJSONLSize)
	_ = downloadResp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("failed to read job logs body for job %d: %w", jobID, err)
	}

	logs := string(bodyBytes)
	if downloadResp.StatusCode == http.StatusPartialContent || downloadResp.StatusCode == http.StatusOK {
		if strings.Contains(logs, costSummaryMarker) {
			return logs, nil
		}
	}

	// 2. If range request was 206 and summary wasn't in tail, do a full GET request
	if downloadResp.StatusCode == http.StatusPartialContent {
		fullReq, fullErr := http.NewRequestWithContext(ctx, http.MethodGet, logURL.String(), nil)
		if fullErr == nil {
			fullResp, getErr := unauthenticatedHTTPClient.Do(fullReq)
			if getErr == nil && fullResp.StatusCode == http.StatusOK {
				fullBodyBytes, readErr := readAllLimited(fullResp.Body, maxMonitorJSONLSize)
				_ = fullResp.Body.Close()
				if readErr == nil {
					return string(fullBodyBytes), nil
				}
			}
		}
	}

	return logs, nil
}

// fetchRunsOnCostFromLogs fetches job logs and parses the runs-on cost summary.
// Checks disk cache at data/<owner>/<repo>/runs_on_costs/<jobID>.json first.
func fetchRunsOnCostFromLogs(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	jobID int64,
	dataDir string,
) (int64, *RunsOnCostSummary, error) {
	cacheFile := filepath.Join(dataDir, owner, repo, "runs_on_costs", fmt.Sprintf("%d.json", jobID))

	if cacheFileExists(cacheFile) {
		if summary, err := readJSONFile[*RunsOnCostSummary](cacheFile); err == nil && summary != nil {
			log.Debug().Int64("job_id", jobID).Msg("loaded runs-on cost summary from disk cache")
			return summary.CostInTenthsOfCent(), summary, nil
		}
	}

	logs, err := fetchJobLogs(ctx, client, owner, repo, jobID)
	if err != nil {
		log.Debug().Err(err).Int64("job_id", jobID).Msg("failed to fetch job logs for runs-on cost")
		return 0, nil, err
	}

	summary, ok := ParseRunsOnCostSummary(logs)
	if !ok || summary == nil {
		log.Debug().Int64("job_id", jobID).Int("log_size", len(logs)).Msg("no runs-on cost summary found in job logs")
		return 0, nil, nil
	}

	_ = ensureDataDir(filepath.Dir(cacheFile), "runs_on_costs")
	_ = writeJSONFile(cacheFile, summary)

	return summary.CostInTenthsOfCent(), summary, nil
}
