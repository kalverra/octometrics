package gather

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var unauthenticatedHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

// MinMaxAvg holds min, max, and avg statistics for a resource metric.
type MinMaxAvg struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	Avg float64 `json:"avg"`
}

// RunsOnJobMetrics holds resource metrics extracted from RunsOn's "📊 Job Metrics" log section.
type RunsOnJobMetrics struct {
	CPULoad1m      *MinMaxAvg `json:"cpu_load_1m,omitempty"`
	CPULoad5m      *MinMaxAvg `json:"cpu_load_5m,omitempty"`
	CPUUtilCorePct *MinMaxAvg `json:"cpu_util_core_pct,omitempty"`
	MemoryUtilPct  *MinMaxAvg `json:"memory_util_pct,omitempty"`
	NetworkIOMB    *MinMaxAvg `json:"network_io_mb,omitempty"`
}

// RunsOnCostSummary holds parsed cost data from runs-on's "Execution Cost Summary" step.
// JSON tags were added after these fields were first cached untagged, so workflow run
// cache files written before the tags exist will not unmarshal these fields; they are
// re-populated on the next gather.
type RunsOnCostSummary struct {
	InstanceType         string            `json:"instance_type,omitempty"`
	InstanceLifecycle    string            `json:"instance_lifecycle,omitempty"`
	Region               string            `json:"region,omitempty"`
	Platform             string            `json:"platform,omitempty"`
	Arch                 string            `json:"arch,omitempty"`
	Az                   string            `json:"az,omitempty"`
	ZoneID               string            `json:"zone_id,omitempty"`
	Duration             string            `json:"duration,omitempty"`
	CostUSD              float64           `json:"cost_usd,omitempty"`
	GitHubEquivalentCost float64           `json:"github_equivalent_cost,omitempty"`
	Savings              string            `json:"savings,omitempty"`
	Metrics              *RunsOnJobMetrics `json:"metrics,omitempty"`
}

// CostInTenthsOfCent converts CostUSD to tenths-of-cent (matching existing convention).
func (s *RunsOnCostSummary) CostInTenthsOfCent() int64 {
	return int64(math.Round(s.CostUSD * 1000))
}

// HasCostData reports whether the summary contains a parsed Execution Cost Summary
// table, as opposed to only Job Metrics with no cost fields. Metrics-only summaries
// must not be treated as cost results: callers fall back to a label-based estimate.
func (s *RunsOnCostSummary) HasCostData() bool {
	return s != nil && (s.InstanceType != "" || s.CostUSD > 0 || s.GitHubEquivalentCost > 0)
}

// costSummaryMarker is the marker that runs-on uses in job logs.
const costSummaryMarker = "Execution Cost Summary"

var (
	ansiEscapePattern    = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")
	longTimestampPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\.(\d{6})\d*Z`)
	logTimestampPattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s`)
)

// CleanLog strips ANSI control characters and truncates 7+ digit fractional-second timestamps.
func CleanLog(raw string) string {
	raw = ansiEscapePattern.ReplaceAllString(raw, "")
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = longTimestampPattern.ReplaceAllString(line, "${1}.${2}Z")
	}
	return strings.Join(lines, "\n")
}

// GetCleanJobLogs fetches and cleans logs for a job ID from local disk cache or GitHub.
func GetCleanJobLogs(
	ctx context.Context,
	_ zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	jobID int64,
	dataDir string,
) (string, error) {
	if owner == "" || repo == "" {
		if autoOwner, autoRepo, _, autoErr := FindOwnerRepoForJob(
			dataDir,
			jobID,
		); autoErr == nil && autoOwner != "" &&
			autoRepo != "" {
			owner = autoOwner
			repo = autoRepo
		}
	}

	wfID, err := FindWorkflowRunIDForJob(dataDir, owner, repo, jobID)
	if err == nil {
		jobLogPath := filepath.Join(
			dataDir,
			owner,
			repo,
			"logs",
			strconv.FormatInt(wfID, 10),
			fmt.Sprintf("%d.log", jobID),
		)
		if cacheFileExists(jobLogPath) {
			//nolint:gosec // job log path is safely constructed inside dataDir
			data, readErr := os.ReadFile(jobLogPath)
			if readErr == nil {
				return CleanLog(string(data)), nil
			}
		}
	}

	var foundLogContent string
	var logFound bool
	targetLogName := fmt.Sprintf("%d.log", jobID)
	_ = filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			//nolint:nilerr // continue walking on access error
			return nil
		}
		if d.Name() == targetLogName {
			//nolint:gosec // path is inside dataDir
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				foundLogContent = string(data)
				logFound = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if logFound {
		return CleanLog(foundLogContent), nil
	}

	if client == nil {
		return "", fmt.Errorf("no cached log file found for job %d and GitHub client is nil", jobID)
	}

	raw, fetchErr := fetchFullJobLogs(ctx, client, owner, repo, jobID)
	if fetchErr != nil {
		return "", fetchErr
	}

	if owner != "" && repo != "" && dataDir != "" {
		if wfID == 0 {
			if jobObj, _, getErr := client.Rest.Actions.GetWorkflowJobByID(
				ctx,
				owner,
				repo,
				jobID,
			); getErr == nil &&
				jobObj != nil {
				wfID = jobObj.GetRunID()
			}
		}
		var cachePath string
		if wfID != 0 {
			cachePath = filepath.Join(
				dataDir,
				owner,
				repo,
				"logs",
				strconv.FormatInt(wfID, 10),
				fmt.Sprintf("%d.log", jobID),
			)
		} else {
			cachePath = filepath.Join(dataDir, owner, repo, "logs", fmt.Sprintf("%d.log", jobID))
		}
		if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o700); mkErr == nil {
			_ = os.WriteFile(cachePath, []byte(raw), 0o600)
		}
	}

	return CleanLog(raw), nil
}

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

var (
	cpuLoad1mChartPattern = regexp.MustCompile(
		`CPU Load Average \(1m\)\s*\(min:\s*([0-9.-]+),\s*max:\s*([0-9.-]+),\s*avg:\s*([0-9.-]+)\)`,
	)
	cpuUtilCoreChartPattern = regexp.MustCompile(
		`CPU Utilization per Core \(%\)\s*\(min:\s*([0-9.-]+),\s*max:\s*([0-9.-]+),\s*avg:\s*([0-9.-]+)\s*%\)`,
	)
	memUtilChartPattern = regexp.MustCompile(
		`Memory Utilization \(%\)\s*\(min:\s*([0-9.-]+),\s*max:\s*([0-9.-]+),\s*avg:\s*([0-9.-]+)\s*%\)`,
	)
	netIOChartPattern = regexp.MustCompile(
		`Network I/O by Direction \(MB\)\s*\(min:\s*([0-9.-]+),\s*max:\s*([0-9.-]+),\s*avg:\s*([0-9.-]+)\s*MB\)`,
	)

	cpuLoad1mStatPattern = regexp.MustCompile(
		`system\.cpu\.load_average\.1m\s+min:\s*([0-9.-]+)\s+max:\s*([0-9.-]+)\s+avg:\s*([0-9.-]+)`,
	)
	cpuLoad5mStatPattern = regexp.MustCompile(
		`system\.cpu\.load_average\.5m\s+min:\s*([0-9.-]+)\s+max:\s*([0-9.-]+)\s+avg:\s*([0-9.-]+)`,
	)
	memUtilStatPattern = regexp.MustCompile(
		`system\.memory\.utilization(?:\s*\(used\))?\s+min:\s*([0-9.-]+)\s+max:\s*([0-9.-]+)\s+avg:\s*([0-9.-]+)`,
	)
)

func parseMinMaxAvgMatch(m []string) *MinMaxAvg {
	if len(m) < 4 {
		return nil
	}
	minVal, _ := strconv.ParseFloat(m[1], 64)
	maxVal, _ := strconv.ParseFloat(m[2], 64)
	avgVal, _ := strconv.ParseFloat(m[3], 64)
	return &MinMaxAvg{
		Min: minVal,
		Max: maxVal,
		Avg: avgVal,
	}
}

// ParseRunsOnJobMetrics extracts CPU, memory, and network metrics from RunsOn's "📊 Job Metrics" section.
func ParseRunsOnJobMetrics(logs string) (*RunsOnJobMetrics, bool) {
	if !strings.Contains(logs, "Job Metrics") && !strings.Contains(logs, "Summary Statistics:") {
		return nil, false
	}

	metrics := &RunsOnJobMetrics{}
	found := false

	if m := cpuLoad1mStatPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.CPULoad1m = parseMinMaxAvgMatch(m)
		found = true
	} else if m := cpuLoad1mChartPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.CPULoad1m = parseMinMaxAvgMatch(m)
		found = true
	}

	if m := cpuLoad5mStatPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.CPULoad5m = parseMinMaxAvgMatch(m)
		found = true
	}

	if m := cpuUtilCoreChartPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.CPUUtilCorePct = parseMinMaxAvgMatch(m)
		found = true
	}

	if m := memUtilStatPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.MemoryUtilPct = parseMinMaxAvgMatch(m)
		found = true
	} else if m := memUtilChartPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.MemoryUtilPct = parseMinMaxAvgMatch(m)
		found = true
	}

	if m := netIOChartPattern.FindStringSubmatch(logs); len(m) >= 4 {
		metrics.NetworkIOMB = parseMinMaxAvgMatch(m)
		found = true
	}

	if !found {
		return nil, false
	}

	return metrics, true
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
		if strings.Contains(logs, costSummaryMarker) || strings.Contains(logs, "Job Metrics") {
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

// fetchFullJobLogs downloads complete logs for a single workflow job without range truncation.
func fetchFullJobLogs(
	parentCtx context.Context,
	client *GitHubClient,
	owner, repo string,
	jobID int64,
) (string, error) {
	ctx, cancel := ghCtx(parentCtx)
	defer cancel()

	logURL, resp, err := client.Rest.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 5)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf(
				"failed to get job logs URL for job %d (%s/%s): 404 Not Found.\n"+
					"Possible reasons:\n"+
					"  - Job log has expired according to GitHub retention limits (default 90 days, or org/repo limits)\n"+
					"  - Job ID %d does not exist in repository %s/%s\n"+
					"  - Incorrect owner/repo or insufficient token permissions",
				jobID, owner, repo, jobID, owner, repo,
			)
		}
		return "", fmt.Errorf("failed to get job logs URL for job %d (%s/%s): %w", jobID, owner, repo, err)
	}
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"expected status code %d or %d for job logs redirect, got %d",
			http.StatusFound,
			http.StatusOK,
			resp.StatusCode,
		)
	}

	if logURL == nil || logURL.String() == "" {
		return "", fmt.Errorf("job logs URL for job %d is empty", jobID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create log request for job %d: %w", jobID, err)
	}

	downloadResp, err := unauthenticatedHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download job logs for job %d: %w", jobID, err)
	}
	defer func() { _ = downloadResp.Body.Close() }()

	if downloadResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"unexpected status code %d downloading job logs for job %d",
			downloadResp.StatusCode,
			jobID,
		)
	}

	bodyBytes, err := readAllLimited(downloadResp.Body, maxMonitorJSONLSize)
	if err != nil {
		return "", fmt.Errorf("failed to read job logs body for job %d: %w", jobID, err)
	}

	return string(bodyBytes), nil
}

// fetchRunsOnCostFromLogs fetches job logs and parses the runs-on cost summary.
// Checks disk cache at data/<owner>/<repo>/runs_on_costs/<jobID>.json first.
// Returns a summary with cost data, a metrics-only summary, or nil if neither is found.
func fetchRunsOnCostFromLogs(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	jobID int64,
	dataDir string,
) (*RunsOnCostSummary, error) {
	cacheFile := filepath.Join(dataDir, owner, repo, "runs_on_costs", fmt.Sprintf("%d.json", jobID))

	if cacheFileExists(cacheFile) {
		if summary, err := readJSONFile[*RunsOnCostSummary](cacheFile); err == nil && summary != nil {
			log.Debug().Int64("job_id", jobID).Msg("loaded runs-on cost summary from disk cache")
			return summary, nil
		}
	}

	logs, err := fetchJobLogs(ctx, client, owner, repo, jobID)
	if err != nil {
		log.Debug().Err(err).Int64("job_id", jobID).Msg("failed to fetch job logs for runs-on cost")
		return nil, err
	}

	summary, ok := ParseRunsOnCostSummary(logs)
	metrics, hasMetrics := ParseRunsOnJobMetrics(logs)
	if hasMetrics {
		if summary == nil {
			summary = &RunsOnCostSummary{}
		}
		summary.Metrics = metrics
	}

	if summary == nil || (!ok && !hasMetrics) {
		log.Debug().
			Int64("job_id", jobID).
			Int("log_size", len(logs)).
			Msg("no runs-on cost summary or metrics found in job logs")
		return nil, nil
	}

	_ = ensureDataDir(filepath.Dir(cacheFile), "runs_on_costs")
	_ = writeJSONFile(cacheFile, summary)

	return summary, nil
}

// LogGap represents a delay between consecutive log lines.
type LogGap struct {
	Duration        time.Duration `json:"duration"`
	LineBefore      string        `json:"line_before"`
	LineAfter       string        `json:"line_after"`
	IsBufferedFlush bool          `json:"is_buffered_flush,omitempty"`
}

var isoTimestampPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?)`)

// ParseLogGaps parses timestamps in log lines and returns the top N gaps.
func ParseLogGaps(rawLog string, topN int) []LogGap {
	if topN <= 0 {
		topN = 5
	}
	lines := strings.Split(rawLog, "\n")
	type entry struct {
		t    time.Time
		line string
	}
	var entries []entry
	for _, l := range lines {
		trimmed := strings.TrimRight(l, "\r")
		m := isoTimestampPattern.FindStringSubmatch(trimmed)
		if len(m) > 1 {
			tStr := m[1]
			t, err := time.Parse(time.RFC3339Nano, tStr)
			if err != nil {
				t, err = time.Parse("2006-01-02T15:04:05.999999999Z", tStr)
			}
			if err == nil {
				entries = append(entries, entry{t: t, line: trimmed})
			}
		}
	}

	var gaps []LogGap
	for i := 1; i < len(entries); i++ {
		dur := entries[i].t.Sub(entries[i-1].t)
		if dur > 0 {
			sameCount := 0
			for j := i; j < len(entries); j++ {
				if entries[j].t.Sub(entries[i].t).Abs() <= 50*time.Millisecond {
					sameCount++
				} else {
					break
				}
			}
			gaps = append(gaps, LogGap{
				Duration:        dur,
				LineBefore:      entries[i-1].line,
				LineAfter:       entries[i].line,
				IsBufferedFlush: sameCount >= 3,
			})
		}
	}

	slices.SortFunc(gaps, func(a, b LogGap) int {
		if a.Duration > b.Duration {
			return -1
		}
		if a.Duration < b.Duration {
			return 1
		}
		return 0
	})

	if len(gaps) > topN {
		gaps = gaps[:topN]
	}
	return gaps
}
