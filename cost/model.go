package cost

import (
	"sort"
	"strings"
	"time"
)

// GenerateReport processes raw usage records, workflow specs, and optional Cloudzero actuals into a comprehensive report.
func GenerateReport(records []*UsageRecord, specs map[string]*WorkflowSpec, cz *CloudzeroResult) *Report {
	if len(records) == 0 {
		return &Report{
			ByRunnerType: make(map[RunnerType]*RunnerBreakdown),
		}
	}

	report := &Report{
		ByRunnerType: make(map[RunnerType]*RunnerBreakdown),
	}

	repoMap := make(map[string]*RepoBreakdown)
	wfMap := make(map[string]*WorkflowBreakdown)
	ghaSKUTotals := make(map[string]*skuAccumulator)

	repoRunsOnTheoretical := make(map[string]float64)
	wfRunsOnTheoretical := make(map[string]float64)
	wfRunnerSpec := make(map[string]string)

	var minDate, maxDate time.Time
	firstDate := true

	for _, rec := range records {
		if firstDate || rec.Date.Before(minDate) {
			minDate = rec.Date
		}
		if firstDate || rec.Date.After(maxDate) {
			maxDate = rec.Date
		}
		firstDate = false

		if report.Organization == "" && rec.Organization != "" {
			report.Organization = rec.Organization
		}

		report.TotalMinutes += rec.Quantity

		repoEntry, ok := repoMap[rec.Repository]
		if !ok {
			repoEntry = &RepoBreakdown{Repository: rec.Repository}
			repoMap[rec.Repository] = repoEntry
		}
		repoEntry.TotalMinutes += rec.Quantity

		key := rec.Repository + ":" + rec.WorkflowPath
		wfEntry, ok := wfMap[key]
		if !ok {
			wfEntry = &WorkflowBreakdown{
				Repository:   rec.Repository,
				WorkflowPath: rec.WorkflowPath,
				RunnerType:   rec.RunnerType,
			}
			wfMap[key] = wfEntry
		}
		wfEntry.Minutes += rec.Quantity

		if rec.RunnerType == RunnerTypeGHANative {
			accumulateGHANative(rec, report, repoEntry, wfEntry, ghaSKUTotals)
		} else {
			accumulateRunsOn(
				rec,
				repoEntry,
				wfEntry,
				specs,
				key,
				repoRunsOnTheoretical,
				wfRunsOnTheoretical,
				wfRunnerSpec,
			)
		}
	}

	report.DateStart = minDate
	report.DateEnd = maxDate

	apportionRunsOnCosts(report, repoMap, wfMap, repoRunsOnTheoretical, wfRunsOnTheoretical, cz)

	report.TotalComputeCost = report.TotalGHANetCost + report.TotalRunsOnCost

	report.ByRunnerType[RunnerTypeGHANative] = &RunnerBreakdown{
		RunnerName:    string(RunnerTypeGHANative),
		RunnerType:    RunnerTypeGHANative,
		Minutes:       report.TotalMinutes - sumRunsOnMinutes(records),
		NetAmount:     report.TotalGHANetCost,
		GrossAmount:   report.TotalGHAGrossCost,
		EstimatedCost: report.TotalGHANetCost,
	}
	report.ByRunnerType[RunnerTypeRunsOn] = &RunnerBreakdown{
		RunnerName:    string(RunnerTypeRunsOn),
		RunnerType:    RunnerTypeRunsOn,
		Minutes:       sumRunsOnMinutes(records),
		NetAmount:     0,
		GrossAmount:   0,
		EstimatedCost: report.TotalRunsOnCost,
	}

	report.ByRunner = buildRunnerBreakdowns(records, wfMap, wfRunnerSpec)

	for _, rp := range repoMap {
		report.ByRepo = append(report.ByRepo, rp)
	}
	sort.Slice(report.ByRepo, func(i, j int) bool {
		return report.ByRepo[i].TotalCost > report.ByRepo[j].TotalCost
	})

	for _, wf := range wfMap {
		report.ByWorkflow = append(report.ByWorkflow, wf)
	}
	sort.Slice(report.ByWorkflow, func(i, j int) bool {
		costI := report.ByWorkflow[i].NetAmount + report.ByWorkflow[i].EstimatedCost
		costJ := report.ByWorkflow[j].NetAmount + report.ByWorkflow[j].EstimatedCost
		return costI > costJ
	})

	report.Savings = calculateBlendedSavings(ghaSKUTotals)
	for _, s := range report.Savings {
		if s.EstimatedSavings > 0 {
			report.TotalProjectedSavings += s.EstimatedSavings
		}
	}

	return report
}

func accumulateGHANative(
	rec *UsageRecord,
	report *Report,
	repoEntry *RepoBreakdown,
	wfEntry *WorkflowBreakdown,
	ghaSKUTotals map[string]*skuAccumulator,
) {
	report.TotalGHANetCost += rec.NetAmount
	report.TotalGHAGrossCost += rec.GrossAmount

	repoEntry.GHAMinutes += rec.Quantity
	repoEntry.GHANetCost += rec.NetAmount
	repoEntry.GHAGrossCost += rec.GrossAmount
	repoEntry.TotalCost += rec.NetAmount

	wfEntry.NetAmount += rec.NetAmount
	wfEntry.GrossAmount += rec.GrossAmount
	wfEntry.Runner = rec.SKU

	skuAcc, ok := ghaSKUTotals[rec.SKU]
	if !ok {
		skuAcc = &skuAccumulator{SKU: rec.SKU}
		ghaSKUTotals[rec.SKU] = skuAcc
	}
	skuAcc.Minutes += rec.Quantity
	skuAcc.NetCost += rec.NetAmount
	skuAcc.GrossCost += rec.GrossAmount
}

func accumulateRunsOn(
	rec *UsageRecord,
	repoEntry *RepoBreakdown,
	wfEntry *WorkflowBreakdown,
	specs map[string]*WorkflowSpec,
	key string,
	repoRunsOnTheoretical, wfRunsOnTheoretical map[string]float64,
	wfRunnerSpec map[string]string,
) {
	specRunner := "16cpu-linux-x64"
	rate := 0.0090
	if spec, ok := specs[key]; ok && spec != nil {
		if spec.RunnerSpec != "" {
			specRunner = spec.RunnerSpec
		}
		if spec.RatePerMin > 0 {
			rate = spec.RatePerMin
		}
	}

	repoEntry.RunsOnMinutes += rec.Quantity
	wfEntry.Runner = specRunner
	wfRunnerSpec[key] = specRunner

	theoretical := rec.Quantity * rate
	repoRunsOnTheoretical[rec.Repository] += theoretical
	wfRunsOnTheoretical[key] += theoretical
}

func apportionRunsOnCosts(
	report *Report,
	repoMap map[string]*RepoBreakdown,
	wfMap map[string]*WorkflowBreakdown,
	repoRunsOnTheoretical, wfRunsOnTheoretical map[string]float64,
	cz *CloudzeroResult,
) {
	if cz != nil && len(cz.RepoCosts) > 0 {
		report.CloudzeroExact = true
		report.UntaggedRunsOnCost = cz.UntaggedCost

		for repo, repoEntry := range repoMap {
			if repoEntry.RunsOnMinutes <= 0 {
				continue
			}

			var repoActualCost float64
			if actual, found := cz.RepoCosts[repo]; found {
				repoActualCost = actual
			}

			repoEntry.RunsOnCost = repoActualCost
			repoEntry.TotalCost += repoActualCost
			report.TotalRunsOnCost += repoActualCost

			thTotal := repoRunsOnTheoretical[repo]
			for key, wf := range wfMap {
				if wf.Repository == repo && wf.RunnerType == RunnerTypeRunsOn {
					if thTotal > 0 {
						wf.EstimatedCost = (wfRunsOnTheoretical[key] / thTotal) * repoActualCost
					} else {
						wf.EstimatedCost = 0
					}
				}
			}
		}

		report.TotalRunsOnCost += cz.UntaggedCost
	} else {
		for repo, repoEntry := range repoMap {
			if repoEntry.RunsOnMinutes <= 0 {
				continue
			}
			cost := repoRunsOnTheoretical[repo]
			repoEntry.RunsOnCost = cost
			repoEntry.TotalCost += cost
			report.TotalRunsOnCost += cost
		}
		for key, wf := range wfMap {
			if wf.RunnerType == RunnerTypeRunsOn {
				wf.EstimatedCost = wfRunsOnTheoretical[key]
			}
		}
	}
}

func buildRunnerBreakdowns(
	records []*UsageRecord,
	wfMap map[string]*WorkflowBreakdown,
	wfRunnerSpec map[string]string,
) []*RunnerBreakdown {
	runnerMap := make(map[string]*RunnerBreakdown)
	for _, rec := range records {
		if rec.RunnerType == RunnerTypeGHANative {
			rEntry, ok := runnerMap[rec.SKU]
			if !ok {
				rEntry = &RunnerBreakdown{
					RunnerName: rec.SKU,
					RunnerType: RunnerTypeGHANative,
				}
				runnerMap[rec.SKU] = rEntry
			}
			rEntry.Minutes += rec.Quantity
			rEntry.NetAmount += rec.NetAmount
			rEntry.GrossAmount += rec.GrossAmount
		}
	}
	for _, wf := range wfMap {
		if wf.RunnerType == RunnerTypeRunsOn {
			spec := wf.Runner
			rEntry, ok := runnerMap[spec]
			if !ok {
				rEntry = &RunnerBreakdown{
					RunnerName: spec,
					RunnerType: RunnerTypeRunsOn,
				}
				runnerMap[spec] = rEntry
			}
			rEntry.EstimatedCost += wf.EstimatedCost
		}
	}
	for _, rec := range records {
		if rec.RunnerType == RunnerTypeRunsOn {
			key := rec.Repository + ":" + rec.WorkflowPath
			spec := wfRunnerSpec[key]
			if rEntry, ok := runnerMap[spec]; ok {
				rEntry.Minutes += rec.Quantity
			}
		}
	}

	var list []*RunnerBreakdown
	for _, rb := range runnerMap {
		list = append(list, rb)
	}
	sort.Slice(list, func(i, j int) bool {
		costI := list[i].NetAmount + list[i].EstimatedCost
		costJ := list[j].NetAmount + list[j].EstimatedCost
		return costI > costJ
	})
	return list
}

type skuAccumulator struct {
	SKU       string
	Minutes   float64
	NetCost   float64
	GrossCost float64
}

func sumRunsOnMinutes(records []*UsageRecord) float64 {
	var total float64
	for _, r := range records {
		if r.RunnerType == RunnerTypeRunsOn {
			total += r.Quantity
		}
	}
	return total
}

func calculateBlendedSavings(totals map[string]*skuAccumulator) []*SavingsEstimate {
	var results []*SavingsEstimate

	for sku, acc := range totals {
		equivRunner, runsOnRate, perSecondFactor := getBlendedRunsOnEquivalent(sku)
		if equivRunner == "" {
			continue
		}

		billableMins := acc.Minutes * perSecondFactor
		estCost := billableMins * runsOnRate
		savings := acc.NetCost - estCost
		var pct float64
		if acc.NetCost > 0 {
			pct = (savings / acc.NetCost) * 100.0
		}

		results = append(results, &SavingsEstimate{
			SKU:                 sku,
			EquivalentRunner:    equivRunner,
			Minutes:             acc.Minutes,
			CurrentNetCost:      acc.NetCost,
			CurrentGrossCost:    acc.GrossCost,
			EstimatedRunsOnCost: estCost,
			EstimatedSavings:    savings,
			SavingsPercent:      pct,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CurrentNetCost > results[j].CurrentNetCost
	})

	return results
}

// getBlendedRunsOnEquivalent returns equivalent runner, empirical rate ($/min), and duration multiplier.
func getBlendedRunsOnEquivalent(sku string) (string, float64, float64) {
	switch {
	case sku == "actions_linux":
		// Standard 2-core: Empirical 2-core Cloudzero rate is $0.0031/min; with 15% per-second billing reduction
		return "2cpu-linux-arm64-8g / 2cpu-linux-x64", 0.0031, 0.85
	case sku == "actions_linux_4_core":
		return "4cpu-linux-x64", 0.0050, 0.85
	case sku == "actions_linux_8_core":
		return "8cpu-linux-x64", 0.0079, 0.85
	case sku == "actions_linux_16_core":
		return "16cpu-linux-x64", 0.0090, 0.85
	case sku == "actions_linux_32_core":
		return "32cpu-linux-x64", 0.0130, 0.85
	case sku == "actions_linux_64_core":
		return "64cpu-linux-x64", 0.0260, 0.85
	case sku == "actions_linux_64_core_arm":
		return "64cpu-linux-arm64", 0.0220, 0.85
	case sku == "actions_linux_2_core_advanced":
		return "2cpu-linux-x64", 0.0031, 0.85
	case strings.Contains(sku, "arm"):
		return "2cpu-linux-arm64", 0.0031, 0.85
	default:
		return "", 0, 1.0
	}
}
