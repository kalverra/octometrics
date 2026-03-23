package gather

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/kalverra/octometrics/monitor"
)

// WorkflowRunsDataDir is the directory name for storing workflow run data files.
const WorkflowRunsDataDir = "workflow_runs"

// maxMonitorJSONLSize caps extracted monitor log size per artifact (zip decompression).
const maxMonitorJSONLSize = 512 << 20 // 512 MiB

// maxMonitorZipDownloadSize caps raw bytes read from the artifact URL (bounded memory, DoS mitigation).
const maxMonitorZipDownloadSize = maxMonitorJSONLSize

// maxArtifactErrorBodyBytes limits how much of a failed HTTP response we read for error messages.
const maxArtifactErrorBodyBytes = 64 << 10 // 64 KiB

// maxZipEntriesPerArtifact rejects archives with excessive entries (zip quine / parser DoS).
const maxZipEntriesPerArtifact = 4096

// Mapping of how much a minute for each runner type costs
// cost depicted in tenths of a cent
// https://docs.github.com/en/billing/managing-billing-for-your-products/managing-billing-for-github-actions/about-billing-for-github-actions#per-minute-rates
var rateByRunner = map[string]int64{
	// https://docs.github.com/en/billing/managing-billing-for-your-products/managing-billing-for-github-actions/about-billing-for-github-actions#per-minute-rates-for-x64-powered-larger-runners
	"UBUNTU":         8,   // $0.008
	"UBUNTU_2_CORE":  8,   // $0.008
	"UBUNTU_4_CORE":  16,  // $0.016
	"UBUNTU_8_CORE":  32,  // $0.032
	"UBUNTU_16_CORE": 64,  // $0.064
	"UBUNTU_32_CORE": 128, // $0.128
	"UBUNTU_64_CORE": 256, // $0.256

	// https://docs.github.com/en/billing/managing-billing-for-your-products/managing-billing-for-github-actions/about-billing-for-github-actions#per-minute-rates-for-arm64-powered-larger-runners
	"UBUNTU_ARM":         5,   // $0.005
	"UBUNTU_2_CORE_ARM":  5,   // $0.005
	"UBUNTU_4_CORE_ARM":  10,  // $0.01
	"UBUNTU_8_CORE_ARM":  20,  // $0.02
	"UBUNTU_16_CORE_ARM": 40,  // $0.04
	"UBUNTU_32_CORE_ARM": 80,  // $0.08
	"UBUNTU_64_CORE_ARM": 160, // $0.16
}

// JobData wraps standard GitHub WorkflowJob data with additional cost fields
type JobData struct {
	*github.WorkflowJob
	// Runner is the type of runner used for the job, e.g. "UBUNTU", "UBUNTU_2_CORE", "UBUNTU_4_CORE"
	Runner string `json:"runner"`
	// Cost is the cost of the job run in tenths of a cent
	Cost int64 `json:"cost"`
	// Analysis is monitoring analysis data for the job run
	Analysis *monitor.Analysis `json:"analysis,omitempty"`
}

// GetRunner returns the runner type used for the job.
func (j *JobData) GetRunner() string {
	if j == nil || j.WorkflowJob == nil {
		return ""
	}
	return j.Runner
}

// GetCost returns the cost of the job run in tenths of a cent.
func (j *JobData) GetCost() int64 {
	if j == nil || j.WorkflowJob == nil {
		return 0
	}
	return j.Cost
}

// GetAnalysis returns the monitoring analysis data for the job run.
func (j *JobData) GetAnalysis() *monitor.Analysis {
	if j == nil || j.Analysis == nil {
		return nil
	}
	return j.Analysis
}

// WorkflowRunData wraps standard GitHub WorkflowRun data with additional fields
// to help with data visualization and cost calculation
type WorkflowRunData struct {
	*github.WorkflowRun
	Jobs                     []*JobData               `json:"jobs"`
	Cost                     int64                    `json:"cost"`
	RunCompletedAt           time.Time                `json:"completed_at"`
	Usage                    *github.WorkflowRunUsage `json:"usage,omitempty"`
	CorrespondingPRNum       int                      `json:"corresponding_pr_number,omitempty"`
	CorrespondingPRCloseTime time.Time                `json:"corresponding_pr_close_time,omitzero"`
	CorrespondingCommitSHA   string                   `json:"corresponding_commit_sha,omitempty"`
}

// GetJobs returns the list of jobs for the workflow run
func (w *WorkflowRunData) GetJobs() []*JobData {
	if w == nil || w.WorkflowRun == nil {
		return nil
	}
	return w.Jobs
}

// GetCost returns the total cost of the workflow run in tenths of a cent
func (w *WorkflowRunData) GetCost() int64 {
	if w == nil || w.WorkflowRun == nil {
		return 0
	}
	return w.Cost
}

// GetRunCompletedAt returns the time the workflow run was completed
func (w *WorkflowRunData) GetRunCompletedAt() time.Time {
	if w == nil || w.WorkflowRun == nil {
		return time.Time{}
	}
	return w.RunCompletedAt
}

// GetUsage returns the billing data for the workflow run
func (w *WorkflowRunData) GetUsage() *github.WorkflowRunUsage {
	if w == nil || w.WorkflowRun == nil {
		return nil
	}
	return w.Usage
}

// WorkflowRun gathers and processes a workflow run from GitHub or local disk.
func WorkflowRun(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	workflowRunID int64,
	options ...Option,
) (*WorkflowRunData, string, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	targetDir := filepath.Join(opts.DataDir, owner, repo, WorkflowRunsDataDir)
	targetFile := filepath.Join(targetDir, fmt.Sprintf("%d.json", workflowRunID))

	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to make data dir '%s': %w", WorkflowRunsDataDir, err)
	}

	log = log.With().Str("target_file", targetFile).Int64("workflow_run_id", workflowRunID).Logger()
	startTime := time.Now()

	// 1. Try loading from disk first
	if !opts.ForceUpdate {
		if data, err := loadWorkflowRunFromDisk(targetFile); err == nil {
			log.Debug().
				Str("duration", time.Since(startTime).String()).
				Str("source", "local file").
				Msg("Gathered workflow run data")
			return data, targetFile, nil
		}
	}

	// 2. Fetch from GitHub
	if client == nil {
		return nil, "", fmt.Errorf("github client is nil")
	}

	workflowRunData, err := fetchWorkflowRunFromGitHub(log, client, owner, repo, workflowRunID, opts, targetDir)
	if err != nil {
		return nil, "", err
	}

	// 3. Save to disk
	if err := saveWorkflowRunToDisk(workflowRunData, targetFile); err != nil {
		return nil, "", fmt.Errorf("failed to save workflow run data for '%d': %w", workflowRunID, err)
	}

	log.Debug().
		Str("duration", time.Since(startTime).String()).
		Str("source", "GitHub API").
		Msg("Gathered workflow run data")
	return workflowRunData, targetFile, nil
}

// loadWorkflowRunFromDisk loads a workflow run from local disk.
func loadWorkflowRunFromDisk(targetFile string) (*WorkflowRunData, error) {
	if _, err := os.Stat(targetFile); err != nil {
		return nil, err
	}

	workflowFileBytes, err := os.ReadFile(filepath.Clean(targetFile))
	if err != nil {
		return nil, fmt.Errorf("failed to open workflow run file: %w", err)
	}

	var data WorkflowRunData
	if err := json.Unmarshal(workflowFileBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow run data: %w", err)
	}

	return &data, nil
}

// saveWorkflowRunToDisk saves a workflow run to local disk.
func saveWorkflowRunToDisk(data *WorkflowRunData, targetFile string) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow run data to json: %w", err)
	}

	if err := os.WriteFile(targetFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write workflow run data to file: %w", err)
	}

	return nil
}

// fetchWorkflowRunFromGitHub fetches a workflow run from GitHub.
func fetchWorkflowRunFromGitHub(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	workflowRunID int64,
	opts *options,
	targetDir string,
) (*WorkflowRunData, error) {
	log.Debug().Msg("Fetching workflow run data from GitHub")

	ctx, cancel := ghCtx()
	workflowRun, resp, err := client.Rest.Actions.GetWorkflowRunByID(ctx, owner, repo, workflowRunID)
	cancel()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	data := &WorkflowRunData{
		WorkflowRun: workflowRun,
	}

	if opts.pullRequestData != nil {
		data.CorrespondingPRNum = opts.pullRequestData.GetNumber()
		data.CorrespondingPRCloseTime = opts.pullRequestData.GetClosedAt().Time
	}
	if opts.commitData != nil {
		data.CorrespondingCommitSHA = opts.commitData.GetSHA()
	}

	completed := workflowRun.GetStatus() == "completed"
	if !completed {
		log.Warn().
			Str("status", workflowRun.GetStatus()).
			Msg("Workflow run is not yet completed; billing and monitoring data will be unavailable")
	}

	var (
		eg                  errgroup.Group
		workflowRunJobs     []*github.WorkflowJob
		workflowBillingData *github.WorkflowRunUsage
		analyses            []*monitor.Analysis
	)

	if completed {
		eg.Go(func() error {
			var analysisErr error
			analyses, analysisErr = monitoringData(log, client, owner, repo, workflowRunID, targetDir)
			return analysisErr
		})

		eg.Go(func() error {
			if !opts.gatherCost {
				return nil
			}
			var billingErr error
			workflowBillingData, billingErr = billingData(client, owner, repo, workflowRunID)
			return billingErr
		})
	}

	eg.Go(func() error {
		var jobsErr error
		workflowRunJobs, jobsErr = jobsData(client, owner, repo, workflowRunID)
		return jobsErr
	})

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("failed to collect workflow data for '%d': %w", workflowRunID, err)
	}

	data.Usage = workflowBillingData
	processJobs(log, data, workflowRunJobs, workflowBillingData, opts.gatherCost)
	processAnalyses(log, data, analyses)

	return data, nil
}

// processJobs processes the jobs for a workflow run.
func processJobs(
	log zerolog.Logger,
	data *WorkflowRunData,
	jobs []*github.WorkflowJob,
	billingData *github.WorkflowRunUsage,
	gatherCost bool,
) {
	completed := data.GetStatus() == "completed"

	for _, job := range jobs {
		completedAt := job.GetCompletedAt().Time
		if !completedAt.IsZero() {
			if data.RunCompletedAt.IsZero() || completedAt.After(data.RunCompletedAt) {
				data.RunCompletedAt = completedAt
			}
		}

		var (
			runner string
			cost   int64
		)

		if completed && gatherCost {
			var billingErr error
			runner, cost, billingErr = calculateJobRunBilling(job.GetID(), billingData)
			if billingErr != nil {
				log.Warn().Err(billingErr).Int64("job_id", job.GetID()).Msg("failed to calculate cost for job")
			}
		}

		if runner == "" {
			runner = job.GetRunnerName()
		}
		if runner == "" {
			runner = getRunnerFromLabels(job.Labels)
		}

		data.Cost += cost
		data.Jobs = append(data.Jobs, &JobData{
			WorkflowJob: job,
			Runner:      runner,
			Cost:        cost,
		})
	}
}

// processAnalyses processes the analyses for a workflow run.
func processAnalyses(log zerolog.Logger, data *WorkflowRunData, analyses []*monitor.Analysis) {
	if data.GetStatus() != "completed" {
		return
	}

nextAnalysisLoop:
	for _, analysis := range analyses {
		for _, job := range data.Jobs {
			if analysis.JobName == job.GetName() {
				job.Analysis = analysis
				continue nextAnalysisLoop
			}
		}
		log.Warn().Str("monitoring_data_job_name", analysis.JobName).Msg("Found monitoring data for job but found no job name matches")
	}
}

// jobsData fetches all jobs for a workflow run from GitHub.
func jobsData(
	client *GitHubClient,
	owner, repo string,
	workflowRunID int64,
) ([]*github.WorkflowJob, error) {
	var (
		workflowJobs = []*github.WorkflowJob{}
		listOpts     = &github.ListWorkflowJobsOptions{
			Filter: "all",
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}
	)

	ctx, cancel := ghCtx()
	defer cancel()

	for job, err := range client.Rest.Actions.ListWorkflowJobsIter(ctx, owner, repo, workflowRunID, listOpts) {
		if err != nil {
			return nil, err
		}
		workflowJobs = append(workflowJobs, job)
	}

	sort.Slice(workflowJobs, func(i, j int) bool {
		return workflowJobs[i].GetStartedAt().Before(workflowJobs[j].GetStartedAt().Time)
	})
	return workflowJobs, nil
}

// billingData fetches the billing data for a workflow run from GitHub
func billingData(
	client *GitHubClient,
	owner, repo string,
	workflowRunID int64,
) (*github.WorkflowRunUsage, error) {
	ctx, cancel := ghCtx()
	usage, resp, err := client.Rest.Actions.GetWorkflowRunUsageByID(ctx, owner, repo, workflowRunID)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("failed to get billing data for workflow run '%d': %w", workflowRunID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	return usage, err
}

// calculateJobRunBilling calculates the cost of a job run based on the billing data
func calculateJobRunBilling(
	jobID int64,
	billingData *github.WorkflowRunUsage,
) (runner string, costInTenthsOfCents int64, err error) {
	if billingData == nil || billingData.GetBillable() == nil {
		return "", 0, fmt.Errorf("no billing data available")
	}
	for runner, billData := range *billingData.GetBillable() {
		if _, ok := rateByRunner[runner]; !ok {
			return "", 0, fmt.Errorf("no rate available for runner %s", runner)
		}
		for _, job := range billData.JobRuns {
			if int64(job.GetJobID()) == jobID {
				billableMinutes := job.GetDurationMS() / 1000 / 60
				costInTenthsOfCents = billableMinutes * rateByRunner[runner]
				return runner, costInTenthsOfCents, nil
			}
		}
	}
	// if we didn't find the job ID in billing data, it was free
	return "Free", 0, nil
}

func monitoringData(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	workflowRunID int64,
	targetDir string,
) ([]*monitor.Analysis, error) {
	var (
		listOpts = &github.ListOptions{
			PerPage: 100,
		}
		artifactsToDownload []*github.Artifact
		analyses            []*monitor.Analysis

		ctx, cancel   = ghCtx()
		artifactsIter = client.Rest.Actions.ListWorkflowRunArtifactsIter(ctx, owner, repo, workflowRunID, listOpts)
	)

	defer cancel()
	for artifact, err := range artifactsIter {
		if err != nil {
			return nil, fmt.Errorf("failed to list workflow run artifacts: %w", err)
		}
		if strings.HasSuffix(artifact.GetName(), "octometrics.monitor.log.jsonl") {
			artifactsToDownload = append(artifactsToDownload, artifact)
		}
	}

	for _, artifact := range artifactsToDownload {
		analysis, err := downloadAndAnalyzeArtifact(log, client, owner, repo, artifact, targetDir)
		if err != nil {
			return nil, err
		}
		analyses = append(analyses, analysis)
	}

	return analyses, nil
}

// readAllLimited reads from r until EOF. If the stream contains more than maxBytes, it returns an error.
func readAllLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, errors.New("readAllLimited: maxBytes must be non-negative")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("content exceeds maximum size of %d bytes", maxBytes)
	}
	return data, nil
}

// safeMonitorJSONLZipEntry returns true if zf is a plausible octometrics log path (no zip-slip).
func safeMonitorJSONLZipEntry(zf *zip.File) bool {
	if zf == nil {
		return false
	}
	name := zf.Name
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return false
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return false
	}
	base := path.Base(clean)
	return base == "octometrics.monitor.log.jsonl" || strings.HasSuffix(base, "-octometrics.monitor.log.jsonl")
}

// downloadAndAnalyzeArtifact fetches one monitoring artifact from GitHub, reads the zip from memory
// (avoiding on-disk zip paths that race when multiple workflow runs share a data directory), extracts
// the JSONL entry to a temp file, and runs monitor.Analyze.
func downloadAndAnalyzeArtifact(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	artifact *github.Artifact,
	targetDir string,
) (*monitor.Analysis, error) {
	ctx, cancel := ghCtx()
	artifactURL, resp, err := client.Rest.Actions.DownloadArtifact(ctx, owner, repo, artifact.GetID(), 5)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact: %w", err)
	}
	if resp.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("expected status code %d, got status code %d", http.StatusFound, resp.StatusCode)
	}
	log.Trace().
		Str("name", artifact.GetName()).
		Int64("id", artifact.GetID()).
		Str("url", artifactURL.String()).
		Msg("Downloading octometrics monitoring data")

	downloadResp, err := http.Get(artifactURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to download monitor data artifact: %w", err)
	}
	defer func() {
		if err := downloadResp.Body.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close monitor data artifact download response")
		}
	}()
	if downloadResp.StatusCode != http.StatusOK {
		bodyBytes, readErr := readAllLimited(downloadResp.Body, maxArtifactErrorBodyBytes)
		if readErr != nil {
			return nil, fmt.Errorf(
				"got unexpected status code %d downloading monitoring data artifact %d, and failed to read response body: %w",
				downloadResp.StatusCode,
				artifact.GetID(),
				readErr,
			)
		}
		return nil, fmt.Errorf(
			"got unexpected status code %d downloading monitoring data artifact %d, body: %s",
			downloadResp.StatusCode,
			artifact.GetID(),
			string(bodyBytes),
		)
	}

	zipBytes, err := readAllLimited(downloadResp.Body, maxMonitorZipDownloadSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read monitor data artifact body: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to open monitor data zip from download: %w", err)
	}

	if len(zr.File) > maxZipEntriesPerArtifact {
		return nil, fmt.Errorf(
			"artifact %d (%q) has too many zip entries (%d); refusing to process",
			artifact.GetID(),
			artifact.GetName(),
			len(zr.File),
		)
	}

	var jsonl *zip.File
	for _, f := range zr.File {
		if safeMonitorJSONLZipEntry(f) {
			jsonl = f
			break
		}
	}
	if jsonl == nil {
		return nil, fmt.Errorf(
			"artifact %d (%q) contains no octometrics.monitor.log.jsonl entry",
			artifact.GetID(),
			artifact.GetName(),
		)
	}

	rc, err := jsonl.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file in zip: %w", err)
	}
	defer func() {
		if err := rc.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close file in zip")
		}
	}()

	//nolint:gosec // pattern is fixed; dir is the workflow run data dir
	monitorFile, err := os.CreateTemp(targetDir, "octometrics-monitor-*.jsonl")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file to extract monitoring data to: %w", err)
	}
	tmpPath := monitorFile.Name()

	if _, err := io.Copy(monitorFile, io.LimitReader(rc, maxMonitorJSONLSize)); err != nil {
		if closeErr := monitorFile.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("failed to close monitor temp file after copy error")
		}
		if rmErr := os.Remove(tmpPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			log.Error().Str("path", tmpPath).Err(rmErr).Msg("failed to remove monitor temp file after copy error")
		}
		return nil, fmt.Errorf("failed to copy monitoring data to temp file: %w", err)
	}
	if err := monitorFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to close monitor temp file before analysis: %w", err)
	}

	analysis, analysisErr := monitor.Analyze(log, tmpPath)
	if analysisErr != nil {
		return nil, fmt.Errorf(
			"failed to analyze octometrics monitoring data file '%s', leaving file for debugging: %w",
			tmpPath,
			analysisErr,
		)
	}
	if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Error().Str("path", tmpPath).Err(err).Msg("failed to remove monitor data temp file")
	}
	return analysis, nil
}

func getRunnerFromLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	// Try to find a common runner label
	for _, label := range labels {
		l := strings.ToLower(label)
		if strings.Contains(l, "ubuntu") || strings.Contains(l, "windows") || strings.Contains(l, "macos") ||
			strings.Contains(l, "self-hosted") {
			return label
		}
	}
	// Fallback to the most descriptive label (usually the longest one if not a common one)
	// or just the first one if all else fails.
	best := labels[0]
	for _, label := range labels {
		if len(label) > len(best) {
			best = label
		}
	}
	return best
}
