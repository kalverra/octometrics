package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// FormatJSON serializes the WorkflowAudit to indented JSON.
func FormatJSON(audit *WorkflowAudit) ([]byte, error) {
	return json.MarshalIndent(audit, "", "  ")
}

// FormatMarkdown generates a clean, rich Markdown audit report suitable for terminal, PR comments, or step summaries.
func FormatMarkdown(audit *WorkflowAudit) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# 🔍 Octometrics Workflow Audit: %s\n\n", audit.Workflow.Name)
	fmt.Fprintf(&sb, "**Repository:** `%s/%s` | **Path:** `%s` | **Sample Size:** %d runs\n\n",
		audit.Workflow.Owner, audit.Workflow.Repo, audit.Workflow.Path, audit.SampleSize)
	if !audit.DateRange.Start.IsZero() && !audit.DateRange.End.IsZero() {
		fmt.Fprintf(&sb, "**Sample Period:** %s to %s\n\n",
			audit.DateRange.Start.Format("2006-01-02 15:04"),
			audit.DateRange.End.Format("2006-01-02 15:04"))
	}

	// 1. Executive Summary
	sb.WriteString("## 📊 Executive Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	fmt.Fprintf(&sb, "| Total Runs Sampled | %d |\n", audit.Summary.TotalRuns)
	fmt.Fprintf(&sb, "| Success Rate | %.1f%% (%d passed, %d failed) |\n",
		audit.Summary.SuccessRate, audit.Summary.SuccessRuns, audit.Summary.FailedRuns)
	fmt.Fprintf(&sb, "| Average Duration | %s |\n", audit.Summary.AvgDuration.Round(time.Second))
	fmt.Fprintf(&sb, "| P95 Duration | %s |\n", audit.Summary.P95Duration.Round(time.Second))
	fmt.Fprintf(&sb, "| Total Cost (Sampled) | $%.4f |\n", audit.Summary.TotalCostUSD)
	fmt.Fprintf(&sb, "| Average Cost / Run | $%.4f |\n\n", audit.Summary.AvgCostPerRunUSD)

	// 2. Recommendations
	if len(audit.Recommendations) > 0 {
		sb.WriteString("## 💡 Optimization & Rightsizing Recommendations\n\n")
		for i, rec := range audit.Recommendations {
			icon := "ℹ️"
			switch rec.Severity {
			case SeverityHigh:
				icon = "🚨"
			case SeverityMedium:
				icon = "⚠️"
			}

			fmt.Fprintf(&sb, "### %s %d. %s [%s]\n\n", icon, i+1, rec.Title, rec.Severity)
			fmt.Fprintf(&sb, "**Job:** `%s`", rec.JobName)
			if rec.EstimatedSavingsPercent > 0 {
				fmt.Fprintf(&sb, " | **Est. Savings:** ~%.0f%%", rec.EstimatedSavingsPercent)
				if rec.EstimatedSavingsPerRunUSD > 0 {
					fmt.Fprintf(&sb, " (~$%.4f/run)", rec.EstimatedSavingsPerRunUSD)
				}
			}
			sb.WriteString("\n\n")
			fmt.Fprintf(&sb, "%s\n\n", rec.Reason)

			if rec.ActionableYAML != "" {
				sb.WriteString("```yaml\n# Suggested Configuration:\n")
				sb.WriteString(rec.ActionableYAML)
				sb.WriteString("\n```\n\n")
			}
		}
	} else {
		sb.WriteString("## 💡 Optimization & Rightsizing Recommendations\n\n")
		sb.WriteString(
			"✅ No urgent sizing or cost anomalies detected. Current runner configuration is well-utilized.\n\n",
		)
	}

	// 3. Resource Utilization Breakdown
	sb.WriteString("## 📈 Resource Utilization Breakdown\n\n")
	sb.WriteString(
		"| Job | Runner | CPU Avg | CPU Peak | 1m Load Avg | RAM Provisioned | RAM Peak GB | RAM Peak % | Data Source |\n",
	)
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, j := range audit.Jobs {
		cpuAvgStr := "-"
		if j.Utilization.CPUAvgPercent > 0 {
			cpuAvgStr = fmt.Sprintf("%.1f%%", j.Utilization.CPUAvgPercent)
		}
		cpuPeakStr := "-"
		if j.Utilization.CPUPeakPercent > 0 {
			cpuPeakStr = fmt.Sprintf("%.1f%%", j.Utilization.CPUPeakPercent)
		}
		loadAvgStr := "-"
		if j.Utilization.CPULoad1mAvg > 0 {
			loadAvgStr = fmt.Sprintf("%.2f", j.Utilization.CPULoad1mAvg)
		}
		ramProvStr := "-"
		if j.Utilization.RAMProvisionedGB > 0 {
			ramProvStr = fmt.Sprintf("%.0f GB", j.Utilization.RAMProvisionedGB)
		}
		ramPeakGBStr := "-"
		if j.Utilization.MemoryPeakGB > 0 {
			ramPeakGBStr = fmt.Sprintf("%.1f GB", j.Utilization.MemoryPeakGB)
		}
		ramPeakPctStr := "-"
		if j.Utilization.MemoryPeakPercent > 0 {
			ramPeakPctStr = fmt.Sprintf("%.1f%%", j.Utilization.MemoryPeakPercent)
		}

		fmt.Fprintf(&sb, "| %s | `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
			j.Name,
			j.Runner,
			cpuAvgStr,
			cpuPeakStr,
			loadAvgStr,
			ramProvStr,
			ramPeakGBStr,
			ramPeakPctStr,
			j.Utilization.Source,
		)
	}
	sb.WriteString("\n")

	// 4. Job Performance Breakdown
	sb.WriteString("## ⏱️ Job Performance & Cost Breakdown\n\n")
	sb.WriteString("| Job | Runs | Avg Duration | P95 Duration | Min Duration | Max Duration | Avg Cost / Run |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, j := range audit.Jobs {
		fmt.Fprintf(&sb, "| %s | %d | %s | %s | %s | %s | $%.4f |\n",
			j.Name,
			j.RunCount,
			j.AvgDuration.Round(time.Second),
			j.P95Duration.Round(time.Second),
			j.MinDuration.Round(time.Second),
			j.MaxDuration.Round(time.Second),
			j.AvgCostPerRunUSD,
		)
	}
	sb.WriteString("\n")

	// 5. Sampled Runs List
	if len(audit.SampledRuns) > 0 {
		sb.WriteString("## 📋 Sampled Workflow Runs\n\n")
		sb.WriteString("| Run ID | Status | Conclusion | Branch | Duration | Cost | Created At |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for _, r := range audit.SampledRuns {
			fmt.Fprintf(&sb, "| [%d](%s) | %s | %s | `%s` | %s | $%.4f | %s |\n",
				r.ID,
				r.HTMLURL,
				r.Status,
				r.Conclusion,
				r.Branch,
				r.Duration.Round(time.Second),
				r.CostUSD,
				r.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatTable generates a terminal text representation with aligned borders.
func FormatTable(audit *WorkflowAudit) string {
	var sb strings.Builder

	sb.WriteString("========================================================================================\n")
	fmt.Fprintf(&sb, " OCTOMETRICS WORKFLOW AUDIT: %s (%s)\n", audit.Workflow.Name, audit.Workflow.Path)
	fmt.Fprintf(
		&sb,
		" Repo: %s/%s | Sample Size: %d runs\n",
		audit.Workflow.Owner,
		audit.Workflow.Repo,
		audit.SampleSize,
	)
	sb.WriteString("========================================================================================\n\n")

	sb.WriteString("--- 1. SUMMARY STATS ---\n")
	fmt.Fprintf(&sb, "Success Rate: %.1f%% (%d passed, %d failed)\n",
		audit.Summary.SuccessRate, audit.Summary.SuccessRuns, audit.Summary.FailedRuns)
	fmt.Fprintf(&sb, "Avg Duration: %s | P95 Duration: %s | Total Cost: $%.4f (Avg $%.4f/run)\n\n",
		audit.Summary.AvgDuration.Round(time.Second),
		audit.Summary.P95Duration.Round(time.Second),
		audit.Summary.TotalCostUSD,
		audit.Summary.AvgCostPerRunUSD,
	)

	if len(audit.Recommendations) > 0 {
		sb.WriteString("--- 2. RECOMMENDATIONS ---\n")
		for i, rec := range audit.Recommendations {
			fmt.Fprintf(&sb, "[%d] %s (%s)\n", i+1, rec.Title, rec.Severity)
			fmt.Fprintf(&sb, "    Job: %s | Est. Savings: %.0f%%\n", rec.JobName, rec.EstimatedSavingsPercent)
			fmt.Fprintf(&sb, "    Reason: %s\n", rec.Reason)
			if rec.ActionableYAML != "" {
				fmt.Fprintf(&sb, "    Config: %s\n", rec.ActionableYAML)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("--- 3. RESOURCE UTILIZATION & RUNNER SIZING ---\n")
	fmt.Fprintf(
		&sb,
		"%-24s | %-16s | %10s | %10s | %10s | %10s | %10s\n",
		"Job Name", "Runner", "CPU Avg", "1m Load", "RAM Prov", "RAM Peak", "RAM %",
	)
	sb.WriteString(strings.Repeat("-", 100) + "\n")
	for _, j := range audit.Jobs {
		cpuAvg := fmt.Sprintf("%.1f%%", j.Utilization.CPUAvgPercent)
		if j.Utilization.CPUAvgPercent == 0 {
			cpuAvg = "-"
		}
		loadAvg := fmt.Sprintf("%.2f", j.Utilization.CPULoad1mAvg)
		if j.Utilization.CPULoad1mAvg == 0 {
			loadAvg = "-"
		}
		ramProv := fmt.Sprintf("%.0f GB", j.Utilization.RAMProvisionedGB)
		if j.Utilization.RAMProvisionedGB == 0 {
			ramProv = "-"
		}
		ramPeak := fmt.Sprintf("%.1f GB", j.Utilization.MemoryPeakGB)
		if j.Utilization.MemoryPeakGB == 0 {
			ramPeak = "-"
		}
		ramPct := fmt.Sprintf("%.1f%%", j.Utilization.MemoryPeakPercent)
		if j.Utilization.MemoryPeakPercent == 0 {
			ramPct = "-"
		}

		name := j.Name
		if len(name) > 24 {
			name = name[:21] + "..."
		}
		runner := j.Runner
		if len(runner) > 16 {
			runner = runner[:13] + "..."
		}

		fmt.Fprintf(
			&sb,
			"%-24s | %-16s | %10s | %10s | %10s | %10s | %10s\n",
			name, runner, cpuAvg, loadAvg, ramProv, ramPeak, ramPct,
		)
	}
	sb.WriteString("\n")

	sb.WriteString("--- 4. JOB DURATIONS & COSTS ---\n")
	fmt.Fprintf(
		&sb,
		"%-24s | %6s | %14s | %14s | %14s\n",
		"Job Name", "Runs", "Avg Duration", "P95 Duration", "Avg Cost ($)",
	)
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	for _, j := range audit.Jobs {
		name := j.Name
		if len(name) > 24 {
			name = name[:21] + "..."
		}
		fmt.Fprintf(
			&sb,
			"%-24s | %6d | %14s | %14s | $%13.4f\n",
			name,
			j.RunCount,
			j.AvgDuration.Round(time.Second).String(),
			j.P95Duration.Round(time.Second).String(),
			j.AvgCostPerRunUSD,
		)
	}
	sb.WriteString("\n")

	return sb.String()
}

const htmlTemplateStr = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{ .Workflow.Name }} Audit | Octometrics</title>
    <link rel="stylesheet" href="/styles.css">
    <script src="/tables.js" defer></script>
</head>
<body>
    <div class="container">
        <header class="page-header">
            <h1>🔍 {{ .Workflow.Name }}</h1>
            <div class="metadata">
                <span class="badge"><span class="badge-label">Repo</span> {{ .Workflow.Owner }}/{{ .Workflow.Repo }}</span>
                <span class="badge"><span class="badge-label">Path</span> {{ .Workflow.Path }}</span>
                <span class="badge"><span class="badge-label">Sample Size</span> {{ .SampleSize }} runs</span>
                <span class="badge badge-cost"><span class="badge-label">Avg Cost / Run</span> ${{ printf "%.4f" .Summary.AvgCostPerRunUSD }}</span>
                <span class="badge"><span class="badge-label">Success Rate</span> {{ printf "%.1f" .Summary.SuccessRate }}%</span>
                <span class="badge"><span class="badge-label">Avg Duration</span> {{ .Summary.AvgDuration }}</span>
            </div>
        </header>

        {{ if .Recommendations }}
        <section class="section">
            <h2>💡 Optimization & Rightsizing Recommendations</h2>
            <div style="display: grid; gap: 1rem; margin-top: 1rem;">
                {{ range $i, $rec := .Recommendations }}
                <div style="background: var(--color-surface); border: 1px solid var(--color-surface-border); border-radius: var(--radius); padding: 1.25rem;">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
                        <h3 style="margin: 0; font-size: 1.1rem;">
                            {{ if eq $rec.Severity "HIGH" }}🚨{{ else if eq $rec.Severity "MEDIUM" }}⚠️{{ else }}ℹ️{{ end }}
                            {{ $rec.Title }}
                        </h3>
                        <div>
                            <span class="badge {{ if eq $rec.Severity "HIGH" }}badge-failure{{ else if eq $rec.Severity "MEDIUM" }}badge-cost{{ end }}">{{ $rec.Severity }}</span>
                            {{ if gt $rec.EstimatedSavingsPercent 0.0 }}
                            <span class="badge badge-cost">~{{ printf "%.0f" $rec.EstimatedSavingsPercent }}% savings</span>
                            {{ end }}
                        </div>
                    </div>
                    <p style="color: var(--color-text-secondary); margin: 0.5rem 0;">{{ $rec.Reason }}</p>
                    {{ if $rec.ActionableYAML }}
                    <pre style="background: var(--color-bg); border: 1px solid var(--color-surface-border); border-radius: var(--radius); padding: 0.75rem; margin: 0.5rem 0; font-family: var(--font-mono); font-size: 0.9rem; overflow-x: auto;"><code>{{ $rec.ActionableYAML }}</code></pre>
                    {{ end }}
                </div>
                {{ end }}
            </div>
        </section>
        {{ end }}

        <section class="section" style="margin-top: 2rem;">
            <h2>📈 Resource Utilization & Runner Sizing</h2>
            <div class="table-container" style="overflow-x: auto; margin-top: 1rem;">
                <table class="sortable" style="width: 100%; border-collapse: collapse;">
                    <thead>
                        <tr>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Job</th>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Runner</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">CPU Avg</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">CPU Peak</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">1m Load</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">RAM Prov</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">RAM Peak</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">RAM Peak %</th>
                            <th style="text-align: center; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Source</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{ range .Jobs }}
                        <tr style="border-bottom: 1px solid var(--color-surface-border);">
                            <td style="padding: 8px;"><strong>{{ .Name }}</strong></td>
                            <td style="padding: 8px;"><code>{{ .Runner }}</code></td>
                            <td style="padding: 8px; text-align: right;">{{ if gt .Utilization.CPUAvgPercent 0.0 }}{{ printf "%.1f" .Utilization.CPUAvgPercent }}%{{ else }}-{{ end }}</td>
                            <td style="padding: 8px; text-align: right;">{{ if gt .Utilization.CPUPeakPercent 0.0 }}{{ printf "%.1f" .Utilization.CPUPeakPercent }}%{{ else }}-{{ end }}</td>
                            <td style="padding: 8px; text-align: right;">{{ if gt .Utilization.CPULoad1mAvg 0.0 }}{{ printf "%.2f" .Utilization.CPULoad1mAvg }}{{ else }}-{{ end }}</td>
                            <td style="padding: 8px; text-align: right;">{{ if gt .Utilization.RAMProvisionedGB 0.0 }}{{ printf "%.0f" .Utilization.RAMProvisionedGB }} GB{{ else }}-{{ end }}</td>
                            <td style="padding: 8px; text-align: right;">{{ if gt .Utilization.MemoryPeakGB 0.0 }}{{ printf "%.1f" .Utilization.MemoryPeakGB }} GB{{ else }}-{{ end }}</td>
                            <td style="padding: 8px; text-align: right;">{{ if gt .Utilization.MemoryPeakPercent 0.0 }}{{ printf "%.1f" .Utilization.MemoryPeakPercent }}%{{ else }}-{{ end }}</td>
                            <td style="padding: 8px; text-align: center;"><span class="badge">{{ .Utilization.Source }}</span></td>
                        </tr>
                        {{ end }}
                    </tbody>
                </table>
            </div>
        </section>

        <section class="section" style="margin-top: 2rem;">
            <h2>⏱️ Job Durations & Cost</h2>
            <div class="table-container" style="overflow-x: auto; margin-top: 1rem;">
                <table class="sortable" style="width: 100%; border-collapse: collapse;">
                    <thead>
                        <tr>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Job</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Runs</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Avg Duration</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">P95 Duration</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Min Duration</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Max Duration</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Avg Cost / Run</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{ range .Jobs }}
                        <tr style="border-bottom: 1px solid var(--color-surface-border);">
                            <td style="padding: 8px;"><strong>{{ .Name }}</strong></td>
                            <td style="padding: 8px; text-align: right;">{{ .RunCount }}</td>
                            <td style="padding: 8px; text-align: right;">{{ .AvgDuration }}</td>
                            <td style="padding: 8px; text-align: right;">{{ .P95Duration }}</td>
                            <td style="padding: 8px; text-align: right;">{{ .MinDuration }}</td>
                            <td style="padding: 8px; text-align: right;">{{ .MaxDuration }}</td>
                            <td style="padding: 8px; text-align: right;">${{ printf "%.4f" .AvgCostPerRunUSD }}</td>
                        </tr>
                        {{ end }}
                    </tbody>
                </table>
            </div>
        </section>

        {{ if .SampledRuns }}
        <section class="section" style="margin-top: 2rem;">
            <h2>📋 Sampled Runs</h2>
            <div class="table-container" style="overflow-x: auto; margin-top: 1rem;">
                <table class="sortable" style="width: 100%; border-collapse: collapse;">
                    <thead>
                        <tr>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Run ID</th>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Status</th>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Conclusion</th>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Branch</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Duration</th>
                            <th style="text-align: right; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Cost</th>
                            <th style="text-align: left; padding: 8px; border-bottom: 2px solid var(--color-surface-border);">Date</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{ range .SampledRuns }}
                        <tr style="border-bottom: 1px solid var(--color-surface-border);">
                            <td style="padding: 8px;"><a href="{{ .HTMLURL }}" target="_blank">{{ .ID }}</a></td>
                            <td style="padding: 8px;">{{ .Status }}</td>
                            <td style="padding: 8px;"><span class="badge {{ if eq .Conclusion "success" }}badge-success{{ else if eq .Conclusion "failure" }}badge-failure{{ end }}">{{ .Conclusion }}</span></td>
                            <td style="padding: 8px;"><code>{{ .Branch }}</code></td>
                            <td style="padding: 8px; text-align: right;">{{ .Duration }}</td>
                            <td style="padding: 8px; text-align: right;">${{ printf "%.4f" .CostUSD }}</td>
                            <td style="padding: 8px;">{{ .CreatedAt.Format "2006-01-02 15:04" }}</td>
                        </tr>
                        {{ end }}
                    </tbody>
                </table>
            </div>
        </section>
        {{ end }}
    </div>
</body>
</html>
`

// FormatHTML renders the WorkflowAudit as an interactive standalone HTML string.
func FormatHTML(audit *WorkflowAudit) (string, error) {
	tmpl, err := template.New("audit_html").Parse(htmlTemplateStr)
	if err != nil {
		return "", fmt.Errorf("parse audit html template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, audit); err != nil {
		return "", fmt.Errorf("execute audit html template: %w", err)
	}

	return buf.String(), nil
}
