package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/monitor"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const WorkflowRunsDataDir = "workflow_runs"

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
}

func (j *JobData) GetRunner() string {
	if j == nil || j.WorkflowJob == nil {
		return ""
	}
	return j.Runner
}

func (j *JobData) GetCost() int64 {
	if j == nil || j.WorkflowJob == nil {
		return 0
	}
	return j.Cost
}

// WorkflowRunData wraps standard GitHub WorkflowRun data with additional fields
// to help with data visualization and cost calculation
type WorkflowRunData struct {
	*github.WorkflowRun
	Jobs                []*JobData               `json:"jobs"`
	Cost                int64                    `json:"cost"`
	RunCompletedAt      time.Time                `json:"completed_at"`
	MonitorObservations *monitor.Observations    `json:"monitor_observations,omitempty"`
	Usage               *github.WorkflowRunUsage `json:"usage,omitempty"`
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

// WorkflowRun gathers all metrics for a completed workflow run
func WorkflowRun(
	log zerolog.Logger,
	client *github.Client,
	owner, repo string,
	workflowRunID int64,
	options ...Option,
) (workflowRunData *WorkflowRunData, targetFile string, err error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	var (
		targetDir  = filepath.Join(opts.DataDir, owner, repo, WorkflowRunsDataDir)
		fileExists = false
	)
	workflowRunData = &WorkflowRunData{}
	targetFile = filepath.Join(targetDir, fmt.Sprintf("%d.json", workflowRunID))

	log = log.With().
		Str("target_file", targetFile).
		Logger()

	err = os.MkdirAll(targetDir, 0700)
	if err != nil {
		return nil, "", fmt.Errorf("failed to make data dir '%s': %w", WorkflowRunsDataDir, err)
	}

	if _, err := os.Stat(targetFile); err == nil {
		fileExists = true
	}

	startTime := time.Now()

	log.Debug().Msg("Gathering workflow run data")

	if !opts.ForceUpdate && fileExists {
		log.Debug().Msg("Reading workflow run data from file")
		//nolint:gosec // I don't care
		workflowFileBytes, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open workflow run file: %w", err)
		}
		err = json.Unmarshal(workflowFileBytes, &workflowRunData)
		log.Debug().
			Str("duration", time.Since(startTime).String()).
			Msg("Gathered workflow run data")
		return workflowRunData, targetFile, err
	}

	if client == nil {
		return nil, "", fmt.Errorf("GitHub client is nil")
	}

	log.Debug().Msg("Fetching workflow run data from GitHub")

	ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
	workflowRun, resp, err := client.Actions.GetWorkflowRunByID(ctx, owner, repo, workflowRunID)
	cancel()
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	if workflowRun == nil {
		return nil, "", fmt.Errorf("workflow run '%d' not found on GitHub", workflowRunID)
	}
	if workflowRun.GetStatus() != "completed" {
		return nil, "", fmt.Errorf("workflow run '%d' is still in progress", workflowRunID)
	}

	workflowRunData.WorkflowRun = workflowRun

	// TODO: Check for octometrics artifact

	var (
		eg                  errgroup.Group
		workflowRunJobs     []*github.WorkflowJob
		workflowBillingData *github.WorkflowRunUsage
	)

	eg.Go(func() error {
		var jobsErr error
		workflowRunJobs, jobsErr = jobsData(log, client, owner, repo, workflowRunID)
		return jobsErr
	})

	eg.Go(func() error {
		var billingErr error
		workflowBillingData, billingErr = billingData(log, client, owner, repo, workflowRunID)
		return billingErr
	})

	if err := eg.Wait(); err != nil {
		return nil, "", fmt.Errorf("failed to collect job and/or billing data for workflow run '%d': %w", workflowRunID, err)
	}
	workflowRunData.Usage = workflowBillingData

	for _, job := range workflowRunJobs {
		// Calculate completed at for the workflow. GitHub API only gives "UpdatedAt" for workflows
		if workflowRunData.RunCompletedAt.IsZero() {
			workflowRunData.RunCompletedAt = job.GetCompletedAt().Time
		} else if job.GetCompletedAt().After(workflowRunData.RunCompletedAt) {
			workflowRunData.RunCompletedAt = job.GetCompletedAt().Time
		}

		runner, cost, err := calculateJobRunBilling(job.GetID(), workflowBillingData)
		if err != nil {
			return nil, "", fmt.Errorf("failed to calculate cost for job '%d': %w", job.GetID(), err)
		}
		workflowRunData.Cost += cost
		workflowRunData.Jobs = append(workflowRunData.Jobs, &JobData{
			WorkflowJob: job,
			Runner:      runner,
			Cost:        cost,
		})
	}

	data, err := json.Marshal(workflowRunData)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal workflow run data to json for workflow run '%d': %w", workflowRunID, err)
	}
	err = os.WriteFile(targetFile, data, 0600)
	if err != nil {
		return nil, "", fmt.Errorf("failed to write workflow run data to file for workflow run '%d': %w", workflowRunID, err)
	}

	log.Debug().
		Str("duration", time.Since(startTime).String()).
		Msg("Gathered workflow run data")
	return workflowRunData, targetFile, nil
}

// jobsData fetches all jobs for a workflow run from GitHub
func jobsData(
	log zerolog.Logger,
	client *github.Client,
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
		resp *github.Response
	)

	for { // Paginate through all jobs
		var (
			err  error
			jobs *github.Jobs
		)

		ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
		jobs, resp, err = client.Actions.ListWorkflowJobs(ctx, owner, repo, workflowRunID, listOpts)
		cancel()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}

		workflowJobs = append(workflowJobs, jobs.Jobs...)
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}
	sort.Slice(workflowJobs, func(i, j int) bool {
		return workflowJobs[i].GetStartedAt().Before(workflowJobs[j].GetStartedAt().Time)
	})
	return workflowJobs, nil
}

// billingData fetches the billing data for a workflow run from GitHub
func billingData(
	log zerolog.Logger,
	client *github.Client,
	owner, repo string,
	workflowRunID int64,
) (*github.WorkflowRunUsage, error) {
	ctx, cancel := context.WithTimeoutCause(ghCtx, timeoutDur, errGitHubTimeout)
	usage, resp, err := client.Actions.GetWorkflowRunUsageByID(ctx, owner, repo, workflowRunID)
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
func calculateJobRunBilling(jobID int64, billingData *github.WorkflowRunUsage) (runner string, costInTenthsOfCents int64, err error) {
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
