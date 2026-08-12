{{- /* Go Template file */ -}}

{{ define "observation_md" }}
# [{{ .Name }}]({{ .GitHubLink }})

| | |
|---|---|
| **State** | {{ .State }} |
| **Actor** | {{ .Actor }} |
{{ if not .CostGathered }}| **Cost** | not gathered |
{{ else }}| **Cost{{ if .CostEstimate }} (est.){{ end }}** | ${{ printf "%.2f" (divideBy1000 .Cost) }} |
{{ end }}{{ if .RequiredWorkflows }}| **Required** | {{ joinStrings .RequiredWorkflows ", " }} |
{{ end }}{{ if .LogsDir }}| **Logs** | {{ .LogsDir }} |
{{ end }}
{{ if .BranchProtectionWarning }}
> **Warning:** Required workflows could not be loaded (insufficient permissions).
{{ end }}

{{ if .CriticalPath }}{{ if .CriticalPath.MedianQueueFinding }}
> **Queue Time Finding:** {{ .CriticalPath.MedianQueueFinding }}
{{ end }}{{ end }}

{{ if not .TimelineData }}
{{ if .StepSummaries }}
## Step Aggregation Across Matrix
| Step Name | Count | Total | % Total | Median | Max |
|---|---|---|---|---|---|
{{ range .StepSummaries }}{{ if ge .PctTotal 1.0 }}| {{ .Name }} | {{ .Count }} | {{ .TotalDuration }} | {{ printf "%.1f" .PctTotal }}% | {{ .MedianDuration }} | {{ .MaxDuration }} |
{{ end }}{{ end }}
{{ end }}

{{ if .CriticalPath }}
## Critical Path
Total Duration: {{ .CriticalPath.TotalDuration }} (Queue: {{ .CriticalPath.TotalQueue }}, Execution: {{ .CriticalPath.TotalExecution }})

| Job Name | Duration | Queue Time | Slack |
|---|---|---|---|
{{ range .CriticalPath.CriticalNodes }}| `{{ .JobName }}` | {{ .Duration }} | {{ .QueueTime }} | {{ .Slack }} |
{{ end }}

{{ if .CriticalPath.NearCriticalNodes }}
### Near-Critical Jobs (Slack ≤ 60s)
| Job Name | Duration | Queue Time | Slack |
|---|---|---|---|
{{ range .CriticalPath.NearCriticalNodes }}| `{{ .JobName }}` | {{ .Duration }} | {{ .QueueTime }} | {{ .Slack }} |
{{ end }}
{{ end }}
{{ end }}

{{ if .SlowestJobSteps }}
## Top Slowest Jobs Step Breakdown
{{ range .SlowestJobSteps }}
### {{ .JobName }} ({{ .Duration }})
{{ if .IsOverheadHeavy }}> ⚠️ **Overhead Warning:** Setup & runner overhead ({{ .RunnerOverheadTotal }} + {{ .TestSetupTotal }}) exceeds test execution ({{ .TestExecutionTotal }}).
{{ end }}
- **Runner Overhead:** {{ .RunnerOverheadTotal }}
- **Test Setup:** {{ .TestSetupTotal }}
- **Test Execution:** {{ .TestExecutionTotal }}

{{ $hasNonSuccess := .HasNonSuccessSteps }}
| Step Name | Duration | Category |{{ if $hasNonSuccess }} Status |{{ end }}
|---|---|---|{{ if $hasNonSuccess }}---|{{ end }}
{{ range .Steps }}| {{ .Name }} | {{ .Duration }} | {{ .Category }} |{{ if $hasNonSuccess }} {{ .Conclusion }} |{{ end }}
{{ end }}
{{ if gt .MinorStepsCount 0 }}| *{{ .MinorStepsCount }} minor steps (≤1s)* | *{{ .MinorStepsTotal }}* | *minor* |{{ if $hasNonSuccess }} - |{{ end }}
{{ end }}
{{ end }}
{{ end }}
{{ end }}

{{ range .TimelineData }}
## {{ .Event }} — {{ .Duration }}, {{ if not $.CostGathered }}cost not gathered{{ else }}${{ printf "%.2f" (divideBy1000 $.Cost) }}{{ if $.CostEstimate }} (est.){{ end }}{{ end }} ({{ .RealStartTime.Format "2006-01-02T15:04:05" }} to {{ .RealEndTime.Format "2006-01-02T15:04:05" }})

{{ template "timeline_md" . }}
{{ if $.MonitoringData }}{{ range $.MonitoringData.Charts }}
### {{ .Title }}

```mermaid
{{ .Diagram }}
```
{{ end }}{{ end }}
{{ end }}
{{ if .CommitData }}
{{ template "pull_request_md" .CommitData }}
{{ end }}
{{ if .FlowChart }}
## Job dependencies

```mermaid
{{ .FlowChart }}
```

{{ end }}
{{ end }}
