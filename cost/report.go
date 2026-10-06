package cost

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatJSON serializes a Report to indented JSON.
func FormatJSON(report *Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// FormatTable returns a cleanly aligned terminal output string for the report.
func FormatTable(report *Report) string {
	var sb strings.Builder

	sb.WriteString("========================================================================================\n")
	fmt.Fprintf(&sb, " OVERALL CI RUNNER SPENDING: %s\n", report.Organization)
	fmt.Fprintf(&sb, " Period: %s to %s\n", report.DateStart.Format("2006-01-02"), report.DateEnd.Format("2006-01-02"))
	sb.WriteString("========================================================================================\n\n")

	sb.WriteString("--- 1. OVERALL RUNNER TYPE BREAKDOWN ---\n")
	fmt.Fprintf(
		&sb,
		"%-20s | %14s | %10s | %12s | %12s\n",
		"Runner Type",
		"Total Minutes",
		"% Time",
		"Net Spend",
		"Gross Spend",
	)
	sb.WriteString(strings.Repeat("-", 76) + "\n")

	gha := report.ByRunnerType[RunnerTypeGHANative]
	if gha != nil {
		pct := 0.0
		if report.TotalMinutes > 0 {
			pct = (gha.Minutes / report.TotalMinutes) * 100.0
		}
		fmt.Fprintf(&sb, "%-20s | %14.0f | %9.1f%% | $%11.2f | $%11.2f\n",
			gha.RunnerName, gha.Minutes, pct, gha.NetAmount, gha.GrossAmount)
	}

	ro := report.ByRunnerType[RunnerTypeRunsOn]
	if ro != nil {
		pct := 0.0
		if report.TotalMinutes > 0 {
			pct = (ro.Minutes / report.TotalMinutes) * 100.0
		}
		label := ro.RunnerName + " (est)"
		if report.CloudzeroExact {
			label = ro.RunnerName + " (Cloudzero)"
		}
		fmt.Fprintf(&sb, "%-20s | %14.0f | %9.1f%% | $%11.2f | %12s\n",
			label, ro.Minutes, pct, ro.EstimatedCost, "$0.00 (GH)")
	}
	sb.WriteString(strings.Repeat("-", 76) + "\n")
	fmt.Fprintf(&sb, "%-20s | %14.0f | %10s | $%11.2f | $%11.2f\n\n",
		"TOTAL CI COMPUTE", report.TotalMinutes, "100.0%", report.TotalComputeCost, report.TotalGHAGrossCost)

	// 2. Breakdown by Runner
	sb.WriteString("--- 2. BREAKDOWN BY RUNNER SKU / INSTANCE TYPE (TOP 15) ---\n")
	fmt.Fprintf(
		&sb,
		"%-32s | %-12s | %12s | %12s | %12s\n",
		"Runner / SKU",
		"Type",
		"Minutes",
		"Net Cost",
		"Gross Cost",
	)
	sb.WriteString(strings.Repeat("-", 90) + "\n")
	limit := min(len(report.ByRunner), 15)
	for i := range limit {
		r := report.ByRunner[i]
		cost := r.NetAmount
		gross := r.GrossAmount
		if r.RunnerType == RunnerTypeRunsOn {
			cost = r.EstimatedCost
			gross = 0
		}
		fmt.Fprintf(&sb, "%-32s | %-12s | %12.0f | $%11.2f | $%11.2f\n",
			r.RunnerName, r.RunnerType, r.Minutes, cost, gross)
	}
	sb.WriteString("\n")

	// 3. Breakdown by Repository
	sb.WriteString("--- 3. BREAKDOWN BY REPOSITORY (TOP 15) ---\n")
	fmt.Fprintf(
		&sb,
		"%-32s | %12s | %12s | %12s | %12s\n",
		"Repository",
		"Total Minutes",
		"GHA Net ($)",
		"Runs-On ($)",
		"Total Cost ($)",
	)
	sb.WriteString(strings.Repeat("-", 90) + "\n")
	limit = min(len(report.ByRepo), 15)
	for i := range limit {
		rp := report.ByRepo[i]
		fmt.Fprintf(&sb, "%-32s | %12.0f | $%11.2f | $%11.2f | $%11.2f\n",
			rp.Repository, rp.TotalMinutes, rp.GHANetCost, rp.RunsOnCost, rp.TotalCost)
	}
	sb.WriteString("\n")

	// 4. Breakdown by Workflow
	sb.WriteString("--- 4. TOP WORKFLOWS BY COMPUTE COST (TOP 15) ---\n")
	fmt.Fprintf(
		&sb,
		"%-24s | %-38s | %-12s | %10s | %10s\n",
		"Repository",
		"Workflow Path",
		"Runner",
		"Minutes",
		"Cost ($)",
	)
	sb.WriteString(strings.Repeat("-", 104) + "\n")
	limit = min(len(report.ByWorkflow), 15)
	for i := range limit {
		wf := report.ByWorkflow[i]
		cost := wf.NetAmount + wf.EstimatedCost
		shortPath := wf.WorkflowPath
		if len(shortPath) > 38 {
			shortPath = "..." + shortPath[len(shortPath)-35:]
		}
		fmt.Fprintf(&sb, "%-24s | %-38s | %-12s | %10.0f | $%9.2f\n",
			wf.Repository, shortPath, wf.Runner, wf.Minutes, cost)
	}
	sb.WriteString("\n")

	// 5. Savings Projections
	sb.WriteString("--- 5. POTENTIAL SAVINGS FROM MIGRATING GHA WORKFLOWS TO RUNS-ON ---\n")
	fmt.Fprintf(&sb, "%-28s | %-32s | %10s | %10s | %10s | %8s\n",
		"GHA Runner SKU", "Runs-On Equivalent", "GHA Net", "Runs-On", "Savings", "% Saved")
	sb.WriteString(strings.Repeat("-", 108) + "\n")
	for _, s := range report.Savings {
		fmt.Fprintf(&sb, "%-28s | %-32s | $%9.2f | $%9.2f | $%9.2f | %7.1f%%\n",
			s.SKU, s.EquivalentRunner, s.CurrentNetCost, s.EstimatedRunsOnCost, s.EstimatedSavings, s.SavingsPercent)
	}
	sb.WriteString(strings.Repeat("-", 108) + "\n")
	fmt.Fprintf(&sb, "TOTAL PROJECTED SAVINGS: $%10.2f\n\n", report.TotalProjectedSavings)

	return sb.String()
}

// FormatMarkdown outputs the report formatted as GitHub-flavored Markdown.
func FormatMarkdown(report *Report) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# GitHub Actions CI Runner Cost & Savings Report: %s\n\n", report.Organization)
	fmt.Fprintf(&sb, "> **Period**: %s to %s | **Total CI Compute Minutes**: %0.0f\n\n",
		report.DateStart.Format("2006-01-02"), report.DateEnd.Format("2006-01-02"), report.TotalMinutes)

	sb.WriteString("## 1. Overall Runner Spending Summary\n\n")
	sb.WriteString("| Runner Type | Total Minutes | % Minutes | Net Spend ($) | Gross Spend ($) | Notes |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")

	gha := report.ByRunnerType[RunnerTypeGHANative]
	if gha != nil {
		pct := 0.0
		if report.TotalMinutes > 0 {
			pct = (gha.Minutes / report.TotalMinutes) * 100.0
		}
		fmt.Fprintf(&sb, "| **%s** | %0.0f | %.1f%% | **$%.2f** | $%.2f | Exact GitHub billing |\n",
			gha.RunnerName, gha.Minutes, pct, gha.NetAmount, gha.GrossAmount)
	}

	ro := report.ByRunnerType[RunnerTypeRunsOn]
	if ro != nil {
		pct := 0.0
		if report.TotalMinutes > 0 {
			pct = (ro.Minutes / report.TotalMinutes) * 100.0
		}
		note := "Estimated AWS/Runs-On compute"
		costSuffix := " *(est)*"
		if report.CloudzeroExact {
			note = "Actual Cloudzero AWS billing"
			costSuffix = ""
		}
		fmt.Fprintf(
			&sb,
			"| **%s** | %0.0f | %.1f%% | **$%.2f**%s | $0.00 (to GH) | %s |\n",
			ro.RunnerName,
			ro.Minutes,
			pct,
			ro.EstimatedCost,
			costSuffix,
			note,
		)
	}
	fmt.Fprintf(&sb, "| **Total CI Compute** | **%0.0f** | **100.0%%** | **$%.2f** | **$%.2f** | |\n\n",
		report.TotalMinutes, report.TotalComputeCost, report.TotalGHAGrossCost)

	sb.WriteString("## 2. Spend by Runner SKU / Instance Type\n\n")
	sb.WriteString("| Runner / SKU | Type | Minutes | Net Cost ($) | Gross Cost ($) |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	limit := min(len(report.ByRunner), 20)
	for i := range limit {
		r := report.ByRunner[i]
		cost := r.NetAmount
		gross := r.GrossAmount
		if r.RunnerType == RunnerTypeRunsOn {
			cost = r.EstimatedCost
			gross = 0
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %0.0f | $%.2f | $%.2f |\n",
			r.RunnerName, r.RunnerType, r.Minutes, cost, gross)
	}
	sb.WriteString("\n")

	sb.WriteString("## 3. Top Repositories by Spend\n\n")
	sb.WriteString("| Repository | Total Minutes | GHA Net ($) | Runs-On ($) | Total Cost ($) |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	limit = min(len(report.ByRepo), 20)
	for i := range limit {
		rp := report.ByRepo[i]
		fmt.Fprintf(&sb, "| `%s` | %0.0f | $%.2f | $%.2f | $%.2f |\n",
			rp.Repository, rp.TotalMinutes, rp.GHANetCost, rp.RunsOnCost, rp.TotalCost)
	}
	sb.WriteString("\n")

	sb.WriteString("## 4. Top Workflows by Compute Spend\n\n")
	sb.WriteString("| Repository | Workflow Path | Runner | Minutes | Total Cost ($) |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	limit = min(len(report.ByWorkflow), 25)
	for i := range limit {
		wf := report.ByWorkflow[i]
		cost := wf.NetAmount + wf.EstimatedCost
		fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` | %0.0f | $%.2f |\n",
			wf.Repository, wf.WorkflowPath, wf.Runner, wf.Minutes, cost)
	}
	sb.WriteString("\n")

	sb.WriteString("## 5. Potential Savings from Migrating to Runs-On\n\n")
	sb.WriteString(
		"| GHA Runner SKU | Runs-On Equivalent | Minutes | Current Net ($) | Runs-On ($) | Estimated Savings ($) | % Saved |\n",
	)
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, s := range report.Savings {
		fmt.Fprintf(
			&sb,
			"| `%s` | `%s` | %0.0f | $%.2f | $%.2f | **$%.2f** | %.1f%% |\n",
			s.SKU,
			s.EquivalentRunner,
			s.Minutes,
			s.CurrentNetCost,
			s.EstimatedRunsOnCost,
			s.EstimatedSavings,
			s.SavingsPercent,
		)
	}
	fmt.Fprintf(&sb, "\n**Total Projected Monthly Savings**: **$%.2f**\n", report.TotalProjectedSavings)

	return sb.String()
}
