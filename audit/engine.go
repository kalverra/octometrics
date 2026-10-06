// Package audit provides multi-run workflow profiling, resource utilization auditing, and runner rightsizing recommendations.
package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/monitor"
)

// auditGatherConcurrency bounds how many workflow runs are gathered in parallel.
// Each gather already fetches job logs concurrently, so keep this modest.
const auditGatherConcurrency = 4

// RunAudit performs an audit on a workflow across recent runs.
func RunAudit(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	opts Options,
) (*WorkflowAudit, error) {
	if opts.RunsCount <= 0 {
		opts.RunsCount = 10
	}
	if opts.Status == "" {
		opts.Status = "completed"
	}
	if opts.CPUThresholdPercent <= 0 {
		opts.CPUThresholdPercent = 30.0
	}
	if opts.RAMThresholdPercent <= 0 {
		opts.RAMThresholdPercent = 50.0
	}

	// 1. Discover run IDs
	runIDs, err := resolveAuditRunIDs(ctx, log, client, opts)
	if err != nil {
		return nil, fmt.Errorf("discover workflow runs: %w", err)
	}

	if len(runIDs) == 0 {
		return nil, fmt.Errorf("no runs found for workflow %q", opts.Workflow)
	}

	// 2. Gather run data for each run ID (bounded concurrency, order preserved)
	gatherOpts := []gather.Option{
		gather.CustomDataFolder(opts.DataDir),
		gather.WithCost(),
		gather.WithDownloadLogs(true),
	}

	results := make([]*gather.WorkflowRunData, len(runIDs))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(auditGatherConcurrency)
	for i, id := range runIDs {
		eg.Go(func() error {
			runData, _, gatherErr := gather.WorkflowRun(
				egCtx,
				log,
				client,
				opts.Owner,
				opts.Repo,
				id,
				gatherOpts...,
			)
			if gatherErr != nil {
				log.Debug().Err(gatherErr).Int64("run_id", id).Msg("failed to gather workflow run, skipping")
				return nil
			}
			if runData != nil {
				// Ensure metrics are populated for every job
				for _, job := range runData.Jobs {
					gather.EnsureJobMetrics(egCtx, log, client, opts.Owner, opts.Repo, job, id, opts.DataDir)
				}
				results[i] = runData
			}
			return nil
		})
	}
	_ = eg.Wait()

	var runs []*gather.WorkflowRunData
	for _, r := range results {
		if r != nil {
			runs = append(runs, r)
		}
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("failed to load any run data for workflow %q", opts.Workflow)
	}

	return BuildAuditReport(runs, opts), nil
}

func resolveAuditRunIDs(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	opts Options,
) ([]int64, error) {
	if len(opts.IncludeRuns) > 0 {
		var runIDs []int64
		for _, id := range opts.IncludeRuns {
			if !slices.Contains(opts.ExcludeRuns, id) {
				runIDs = append(runIDs, id)
			}
		}
		if opts.RunsCount > 0 && len(runIDs) > opts.RunsCount {
			runIDs = runIDs[:opts.RunsCount]
		}
		return runIDs, nil
	}

	fetchLimit := opts.RunsCount
	if len(opts.ExcludeRuns) > 0 {
		fetchLimit += len(opts.ExcludeRuns)
	}
	discovered, err := gather.ListWorkflowRuns(
		ctx,
		log,
		client,
		opts.Owner,
		opts.Repo,
		opts.Workflow,
		opts.Branch,
		opts.Status,
		fetchLimit,
		opts.DataDir,
	)
	if err != nil {
		return nil, fmt.Errorf("discover workflow runs: %w", err)
	}

	var runIDs []int64
	for _, id := range discovered {
		if !slices.Contains(opts.ExcludeRuns, id) {
			runIDs = append(runIDs, id)
			if len(runIDs) >= opts.RunsCount {
				break
			}
		}
	}
	return runIDs, nil
}

// BuildAuditReport aggregates gathered run data and executes recommendation rules.
func BuildAuditReport(runs []*gather.WorkflowRunData, opts Options) *WorkflowAudit {
	audit := &WorkflowAudit{
		SampleSize: len(runs),
	}

	if len(runs) == 0 {
		return audit
	}

	firstRun := runs[0]
	wfPath := firstRun.GetPath()
	wfName := firstRun.GetName()
	audit.Workflow = WorkflowInfo{
		Name:    wfName,
		Path:    wfPath,
		Owner:   opts.Owner,
		Repo:    opts.Repo,
		HTMLURL: firstRun.GetHTMLURL(),
	}
	if audit.Workflow.Owner == "" {
		audit.Workflow.Owner = firstRun.GetOwner()
	}
	if audit.Workflow.Repo == "" {
		audit.Workflow.Repo = firstRun.GetRepo()
	}

	var (
		totalDuration   time.Duration
		durations       []time.Duration
		totalCostUSD    float64
		successCount    int
		failedCount     int
		earliestTime    time.Time
		latestTime      time.Time
		sampledRunsList []SampledRun
	)

	// Aggregate run-level metrics
	for _, r := range runs {
		status := r.GetStatus()
		conclusion := r.GetConclusion()
		switch conclusion {
		case "success":
			successCount++
		case "failure", "cancelled", "timed_out":
			failedCount++
		}

		startTime := r.GetCreatedAt().Time
		if startTime.IsZero() {
			startTime = r.GetRunStartedAt().Time
		}
		if !startTime.IsZero() {
			if earliestTime.IsZero() || startTime.Before(earliestTime) {
				earliestTime = startTime
			}
			if latestTime.IsZero() || startTime.After(latestTime) {
				latestTime = startTime
			}
		}

		endTime := r.RunCompletedAt
		if endTime.IsZero() && !r.GetUpdatedAt().IsZero() {
			endTime = r.GetUpdatedAt().Time
		}
		runDur := time.Duration(0)
		if !endTime.IsZero() && !startTime.IsZero() && endTime.After(startTime) {
			runDur = endTime.Sub(startTime)
		}

		runCostUSD := float64(r.GetCost()) / 1000.0
		totalCostUSD += runCostUSD
		if runDur > 0 {
			totalDuration += runDur
			durations = append(durations, runDur)
		}

		sampledRunsList = append(sampledRunsList, SampledRun{
			ID:         r.GetID(),
			Status:     status,
			Conclusion: conclusion,
			Branch:     r.GetHeadBranch(),
			Duration:   runDur,
			CostUSD:    runCostUSD,
			CreatedAt:  startTime,
			HTMLURL:    r.GetHTMLURL(),
		})
	}

	slices.Sort(durations)
	avgDur := time.Duration(0)
	minDur := time.Duration(0)
	maxDur := time.Duration(0)
	p95Dur := time.Duration(0)
	if len(durations) > 0 {
		avgDur = totalDuration / time.Duration(len(durations))
		minDur = durations[0]
		maxDur = durations[len(durations)-1]
		p95Idx := max(int(math.Ceil(0.95*float64(len(durations))))-1, 0)
		p95Dur = durations[p95Idx]
	}

	successRate := 0.0
	if len(runs) > 0 {
		successRate = (float64(successCount) / float64(len(runs))) * 100.0
	}

	audit.DateRange = DateRange{
		Start: earliestTime,
		End:   latestTime,
	}
	audit.Summary = RunSummaryStats{
		TotalRuns:        len(runs),
		SuccessRuns:      successCount,
		FailedRuns:       failedCount,
		SuccessRate:      successRate,
		AvgDuration:      avgDur,
		MinDuration:      minDur,
		MaxDuration:      maxDur,
		P95Duration:      p95Dur,
		TotalCostUSD:     totalCostUSD,
		AvgCostPerRunUSD: totalCostUSD / float64(len(runs)),
	}
	audit.SampledRuns = sampledRunsList

	// 3. Aggregate per-job metrics
	audit.Jobs = aggregateJobAudits(runs, opts)

	// 4. Generate overall and per-job recommendations
	audit.Recommendations = generateRecommendations(audit.Jobs, audit.Summary, opts)

	return audit
}

type jobStatsAccumulator struct {
	name          string
	runner        string
	labels        []string
	runCount      int
	durations     []time.Duration
	costsUSD      []float64
	cpuAvgs       []float64
	cpuPeaks      []float64
	cpuLoad1mAvg  []float64
	cpuLoad1mPeak []float64
	cpuLoad5mAvg  []float64
	cpuLoad5mPeak []float64
	memAvgs       []float64
	memPeaks      []float64
	netAvgsMB     []float64
	netPeaksMB    []float64
	sources       []string
}

func aggregateJobAudits(runs []*gather.WorkflowRunData, _ Options) []JobAudit {
	accMap := make(map[string]*jobStatsAccumulator)
	var jobOrder []string

	for _, run := range runs {
		for _, job := range run.Jobs {
			if job != nil {
				recordJobRun(accMap, &jobOrder, job)
			}
		}
	}

	results := make([]JobAudit, 0, len(jobOrder))
	for _, name := range jobOrder {
		results = append(results, buildJobAudit(accMap[name]))
	}
	return results
}

func recordJobRun(accMap map[string]*jobStatsAccumulator, jobOrder *[]string, job *gather.JobData) {
	jobName := job.GetName()
	acc, ok := accMap[jobName]
	if !ok {
		acc = &jobStatsAccumulator{
			name:   jobName,
			runner: job.GetRunner(),
			labels: job.Labels,
		}
		accMap[jobName] = acc
		*jobOrder = append(*jobOrder, jobName)
	}
	acc.runCount++

	// Duration
	started := job.GetStartedAt().Time
	completed := job.GetCompletedAt().Time
	if !started.IsZero() && !completed.IsZero() && completed.After(started) {
		acc.durations = append(acc.durations, completed.Sub(started))
	}

	// Cost
	costUSD := float64(job.GetCost()) / 1000.0
	acc.costsUSD = append(acc.costsUSD, costUSD)

	// Resource metrics
	if m := job.GetRunsOnMetrics(); m != nil {
		recordRunsOnMetrics(acc, m)
	} else if a := job.GetAnalysis(); a != nil {
		recordAnalysisMetrics(acc, a)
	}
}

func recordRunsOnMetrics(acc *jobStatsAccumulator, m *gather.RunsOnJobMetrics) {
	acc.sources = append(acc.sources, "runs-on")
	if m.CPUUtilCorePct != nil {
		acc.cpuAvgs = append(acc.cpuAvgs, m.CPUUtilCorePct.Avg)
		acc.cpuPeaks = append(acc.cpuPeaks, m.CPUUtilCorePct.Max)
	}
	if m.CPULoad1m != nil {
		acc.cpuLoad1mAvg = append(acc.cpuLoad1mAvg, m.CPULoad1m.Avg)
		acc.cpuLoad1mPeak = append(acc.cpuLoad1mPeak, m.CPULoad1m.Max)
	}
	if m.CPULoad5m != nil {
		acc.cpuLoad5mAvg = append(acc.cpuLoad5mAvg, m.CPULoad5m.Avg)
		acc.cpuLoad5mPeak = append(acc.cpuLoad5mPeak, m.CPULoad5m.Max)
	}
	if m.MemoryUtilPct != nil {
		acc.memAvgs = append(acc.memAvgs, m.MemoryUtilPct.Avg)
		acc.memPeaks = append(acc.memPeaks, m.MemoryUtilPct.Max)
	}
	if m.NetworkIOMB != nil {
		acc.netAvgsMB = append(acc.netAvgsMB, m.NetworkIOMB.Avg)
		acc.netPeaksMB = append(acc.netPeaksMB, m.NetworkIOMB.Max)
	}
}

func recordAnalysisMetrics(acc *jobStatsAccumulator, a *monitor.Analysis) {
	acc.sources = append(acc.sources, "octometrics-action")
	if len(a.CPUMeasurements) > 0 {
		var totalCPU, peakCPU float64
		var count int
		for _, coreList := range a.CPUMeasurements {
			for _, m := range coreList {
				totalCPU += m.UsedPercent
				if m.UsedPercent > peakCPU {
					peakCPU = m.UsedPercent
				}
				count++
			}
		}
		if count > 0 {
			acc.cpuAvgs = append(acc.cpuAvgs, totalCPU/float64(count))
			acc.cpuPeaks = append(acc.cpuPeaks, peakCPU)
		}
	}
	if len(a.MemoryMeasurements) > 0 && a.SystemInfo != nil && a.SystemInfo.Memory != nil &&
		a.SystemInfo.Memory.Total > 0 {
		totalRAM := float64(a.SystemInfo.Memory.Total)
		var totalMemUsed, peakMemUsed float64
		for _, m := range a.MemoryMeasurements {
			totalMemUsed += float64(m.Used)
			if float64(m.Used) > peakMemUsed {
				peakMemUsed = float64(m.Used)
			}
		}
		avgMemUsed := totalMemUsed / float64(len(a.MemoryMeasurements))
		acc.memAvgs = append(acc.memAvgs, (avgMemUsed/totalRAM)*100.0)
		acc.memPeaks = append(acc.memPeaks, (peakMemUsed/totalRAM)*100.0)
	}
}

func buildJobAudit(acc *jobStatsAccumulator) JobAudit {
	specs := gather.ExtractRunnerSpecs(acc.labels, acc.runner)

	// Durations
	slices.Sort(acc.durations)
	avgDur := time.Duration(0)
	minDur := time.Duration(0)
	maxDur := time.Duration(0)
	p95Dur := time.Duration(0)
	if len(acc.durations) > 0 {
		var totDur time.Duration
		for _, d := range acc.durations {
			totDur += d
		}
		avgDur = totDur / time.Duration(len(acc.durations))
		minDur = acc.durations[0]
		maxDur = acc.durations[len(acc.durations)-1]
		p95Idx := max(int(math.Ceil(0.95*float64(len(acc.durations))))-1, 0)
		p95Dur = acc.durations[p95Idx]
	}

	// Costs
	var totCost float64
	for _, c := range acc.costsUSD {
		totCost += c
	}
	avgCost := 0.0
	if len(acc.costsUSD) > 0 {
		avgCost = totCost / float64(len(acc.costsUSD))
	}

	// Utilization
	source := "none"
	if slices.Contains(acc.sources, "runs-on") {
		source = "runs-on"
	} else if slices.Contains(acc.sources, "octometrics-action") {
		source = "octometrics-action"
	}

	memAvgPct := averageSlice(acc.memAvgs)
	memPeakPct := maxSlice(acc.memPeaks)

	var memAvgGB float64
	var memPeakGB float64
	if specs.ProvisionedRAMGB > 0 {
		memAvgGB = specs.ProvisionedRAMGB * (memAvgPct / 100.0)
		memPeakGB = specs.ProvisionedRAMGB * (memPeakPct / 100.0)
	}

	util := ResourceUtilization{
		CPUCoresProvisioned: specs.ProvisionedCPU,
		CPUAvgPercent:       averageSlice(acc.cpuAvgs),
		CPUPeakPercent:      maxSlice(acc.cpuPeaks),
		CPULoad1mAvg:        averageSlice(acc.cpuLoad1mAvg),
		CPULoad1mPeak:       maxSlice(acc.cpuLoad1mPeak),
		CPULoad5mAvg:        averageSlice(acc.cpuLoad5mAvg),
		CPULoad5mPeak:       maxSlice(acc.cpuLoad5mPeak),
		RAMProvisionedGB:    specs.ProvisionedRAMGB,
		MemoryAvgPercent:    memAvgPct,
		MemoryPeakPercent:   memPeakPct,
		MemoryAvgGB:         memAvgGB,
		MemoryPeakGB:        memPeakGB,
		NetworkIOAvgMB:      averageSlice(acc.netAvgsMB),
		NetworkIOPeakMB:     maxSlice(acc.netPeaksMB),
		Source:              source,
	}

	return JobAudit{
		Name:             acc.name,
		RunCount:         acc.runCount,
		AvgDuration:      avgDur,
		MinDuration:      minDur,
		MaxDuration:      maxDur,
		P95Duration:      p95Dur,
		TotalCostUSD:     totCost,
		AvgCostPerRunUSD: avgCost,
		Runner:           acc.runner,
		RunnerSpecs:      specs,
		Utilization:      util,
	}
}

func averageSlice(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func maxSlice(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	maxVal := vals[0]
	for _, v := range vals[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

func generateRecommendations(jobs []JobAudit, overall RunSummaryStats, opts Options) []Recommendation {
	var recs []Recommendation
	for i := range jobs {
		job := &jobs[i]
		hasRightsizing := false
		if rec := evaluateJobRightsizing(job, opts); rec != nil {
			recs = append(recs, *rec)
			hasRightsizing = true
		}
		if rec := evaluateJobOOM(job); rec != nil {
			recs = append(recs, *rec)
		}
		if rec := evaluateJobSpot(job); rec != nil {
			recs = append(recs, *rec)
		}
		// Skip the Graviton suggestion when a rightsizing downgrade is already recommended:
		// the downgrade changes the instance first, so re-audit afterwards for a cleaner signal.
		if !hasRightsizing {
			if rec := evaluateJobGraviton(job); rec != nil {
				recs = append(recs, *rec)
			}
		}
		if rec := evaluateJobBottleneck(job, overall); rec != nil {
			recs = append(recs, *rec)
		}
	}
	return recs
}

func evaluateJobRightsizing(job *JobAudit, opts Options) *Recommendation {
	specs := job.RunnerSpecs
	util := job.Utilization
	standardRAMTiers := []float64{4, 8, 16, 32, 64, 128}

	recCPU := specs.ProvisionedCPU
	recRAM := specs.ProvisionedRAMGB
	hasCPUChange := false
	hasRAMChange := false
	var reasonParts []string

	// RAM Rightsizing check
	if specs.ProvisionedRAMGB > 0 && util.MemoryPeakPercent > 0 && util.MemoryPeakPercent < opts.RAMThresholdPercent {
		safeRAMNeeded := util.MemoryPeakGB * 1.3
		for _, tier := range standardRAMTiers {
			if tier >= safeRAMNeeded && tier < specs.ProvisionedRAMGB {
				recRAM = tier
				hasRAMChange = true
				headroomPct := ((tier - util.MemoryPeakGB) / tier) * 100.0
				reasonParts = append(
					reasonParts,
					fmt.Sprintf(
						"Peak memory was %.1f GB (%.1f%% of %.0f GB provisioned). Downgrading to %.0f GB RAM leaves %.0f%% safety headroom.",
						util.MemoryPeakGB,
						util.MemoryPeakPercent,
						specs.ProvisionedRAMGB,
						tier,
						headroomPct,
					),
				)
				break
			}
		}
	}

	// CPU Rightsizing check
	if specs.ProvisionedCPU > 2 && (util.CPUAvgPercent > 0 || util.CPULoad1mAvg > 0) {
		isUnderutilized := (util.CPUAvgPercent > 0 && util.CPUAvgPercent < opts.CPUThresholdPercent) ||
			(util.CPULoad1mAvg > 0 && util.CPULoad1mAvg < float64(specs.ProvisionedCPU)/2.0)
		if isUnderutilized {
			targetCPU := specs.ProvisionedCPU / 2
			if targetCPU >= 2 {
				recCPU = targetCPU
				hasCPUChange = true
				reasonParts = append(
					reasonParts,
					fmt.Sprintf(
						"Average CPU utilization was %.1f%% (1m load average %.2f on %d vCPUs). Downgrading to %d vCPUs provides ample compute capacity.",
						util.CPUAvgPercent,
						util.CPULoad1mAvg,
						specs.ProvisionedCPU,
						targetCPU,
					),
				)
			}
		}
	}

	if !hasCPUChange && !hasRAMChange {
		return nil
	}

	savingsPct, savingsPerRun := calculateRightsizingSavings(job, recCPU)
	curConfig, recConfig, actionableYAML := formatRightsizingConfigs(specs, recCPU, recRAM)

	severity := SeverityMedium
	if savingsPct >= 40.0 {
		severity = SeverityHigh
	}

	return &Recommendation{
		Type:                      RecommendationTypeRunnerRightsizing,
		Severity:                  severity,
		JobName:                   job.Name,
		Title:                     fmt.Sprintf("Downgrade runner for %q: %s -> %s", job.Name, curConfig, recConfig),
		Reason:                    strings.Join(reasonParts, " "),
		CurrentConfig:             curConfig,
		RecommendedConfig:         recConfig,
		ActionableYAML:            actionableYAML,
		EstimatedSavingsPercent:   savingsPct,
		EstimatedSavingsPerRunUSD: savingsPerRun,
	}
}

func calculateRightsizingSavings(job *JobAudit, recCPU int) (float64, float64) {
	specs := job.RunnerSpecs
	curRate := int64(0)
	targetRate := int64(0)
	if specs.IsRunsOn {
		curKey := fmt.Sprintf("%dcpu-linux-%s", specs.ProvisionedCPU, specs.Architecture)
		targetKey := fmt.Sprintf("%dcpu-linux-%s", recCPU, specs.Architecture)
		curRate, _ = gather.RunsOnRate(curKey)
		targetRate, _ = gather.RunsOnRate(targetKey)
	}

	if curRate > 0 && targetRate > 0 && curRate > targetRate {
		savingsPct := (float64(curRate-targetRate) / float64(curRate)) * 100.0
		durationMin := job.AvgDuration.Minutes()
		savingsPerRun := (float64(curRate-targetRate) * durationMin) / 1000.0
		return savingsPct, savingsPerRun
	}

	coreSavings := (float64(specs.ProvisionedCPU-recCPU) / float64(specs.ProvisionedCPU)) * 100.0
	savingsPct := coreSavings * 0.8
	savingsPerRun := job.AvgCostPerRunUSD * (savingsPct / 100.0)
	return savingsPct, savingsPerRun
}

func formatRightsizingConfigs(specs gather.RunnerSpecs, recCPU int, recRAM float64) (string, string, string) {
	if specs.IsRunsOn {
		fam := specs.Family
		if fam == "" {
			fam = "m6i"
		}
		curConfig := fmt.Sprintf("cpu=%d/ram=%.0f/family=%s", specs.ProvisionedCPU, specs.ProvisionedRAMGB, fam)
		recConfig := fmt.Sprintf("cpu=%d/ram=%.0f/family=%s", recCPU, recRAM, fam)
		actionableYAML := fmt.Sprintf("runs-on: [\"runs-on=.../cpu=%d/ram=%.0f/family=%s\"]", recCPU, recRAM, fam)
		return curConfig, recConfig, actionableYAML
	}

	curConfig := fmt.Sprintf("%d vCPU / %.0f GB RAM", specs.ProvisionedCPU, specs.ProvisionedRAMGB)
	recConfig := fmt.Sprintf("%d vCPU / %.0f GB RAM", recCPU, recRAM)
	actionableYAML := fmt.Sprintf("runs-on: ubuntu-24.04-%dcore", recCPU)
	return curConfig, recConfig, actionableYAML
}

func evaluateJobOOM(job *JobAudit) *Recommendation {
	specs := job.RunnerSpecs
	util := job.Utilization
	if specs.ProvisionedRAMGB > 0 && util.MemoryPeakPercent > 85.0 {
		return &Recommendation{
			Type:     RecommendationTypeRunnerRightsizing,
			Severity: SeverityHigh,
			JobName:  job.Name,
			Title:    "Risk of Out-of-Memory (OOM) on " + job.Name,
			Reason: fmt.Sprintf(
				"Peak memory usage reached %.1f%% of %.0f GB provisioned RAM. Consider upgrading RAM to prevent job failure.",
				util.MemoryPeakPercent,
				specs.ProvisionedRAMGB,
			),
		}
	}
	return nil
}

func evaluateJobSpot(job *JobAudit) *Recommendation {
	specs := job.RunnerSpecs
	if specs.IsRunsOn && !specs.Spot {
		return &Recommendation{
			Type:     RecommendationTypeCostOptimization,
			Severity: SeverityMedium,
			JobName:  job.Name,
			Title: fmt.Sprintf(
				"Enable Spot instances on %q for ~60%% additional savings",
				job.Name,
			),
			Reason:                    "Runner is configured with on-demand instances (spot=false). For pull requests and non-critical workflows, spot instances provide significant savings with minimal interruption risk.",
			CurrentConfig:             "spot=false",
			RecommendedConfig:         "spot=true",
			ActionableYAML:            "runs-on: [\"runs-on=.../spot=true\"]",
			EstimatedSavingsPercent:   60.0,
			EstimatedSavingsPerRunUSD: job.AvgCostPerRunUSD * 0.60,
		}
	}
	return nil
}

func evaluateJobGraviton(job *JobAudit) *Recommendation {
	specs := job.RunnerSpecs
	if specs.Architecture == "x64" {
		return &Recommendation{
			Type:                    RecommendationTypeCostOptimization,
			Severity:                SeverityInfo,
			JobName:                 job.Name,
			Title:                   fmt.Sprintf("Evaluate ARM64 / Graviton runners for %q", job.Name),
			Reason:                  "Current runner is x86_64. If workload compiles or runs on ARM64, AWS Graviton instances typically offer 15-25% better price/performance.",
			CurrentConfig:           "arch=x64",
			RecommendedConfig:       "arch=arm64",
			EstimatedSavingsPercent: 20.0,
		}
	}
	return nil
}

func evaluateJobBottleneck(job *JobAudit, overall RunSummaryStats) *Recommendation {
	if overall.AvgDuration > 0 && job.AvgDuration > 0 {
		pctOfRun := (job.AvgDuration.Seconds() / overall.AvgDuration.Seconds()) * 100.0
		if pctOfRun >= 60.0 && job.AvgDuration > 3*time.Minute {
			return &Recommendation{
				Type:     RecommendationTypePerformanceBottleneck,
				Severity: SeverityMedium,
				JobName:  job.Name,
				Title: fmt.Sprintf(
					"Performance Bottleneck: %q accounts for %.0f%% of workflow time",
					job.Name,
					pctOfRun,
				),
				Reason: fmt.Sprintf(
					"Job average duration is %s out of total workflow duration %s. Optimizing this job (e.g. caching dependencies, parallel test shards) will have the highest impact on CI turnaround time.",
					job.AvgDuration.Round(time.Second),
					overall.AvgDuration.Round(time.Second),
				),
			}
		}
	}
	return nil
}
