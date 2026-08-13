{{- /* Go Template file */ -}}

{{ define "timeline_md" }}
{{ if .Items }}
{{ if le (len .Items) 15 }}
```mermaid
gantt
    dateFormat {{ .DateFormat }}
    axisFormat {{ .AxisFormat }}
    {{ $dateFormat := .GoDateFormat }}
    {{ range .Items }}
    {{ sanitizeMermaidName .Name }} :{{ if .Conclusion }}{{ .Conclusion }},{{ end }} {{ .ID }}, {{ .StartTime.Format $dateFormat }}, {{ .Duration.Seconds }}s{{ end }}
```
{{ end }}

{{ $hasRunner := .HasRunner }}
{{ $hasCost := .HasCost }}
{{ $hasLogPath := .HasLogPath }}
{{ $hasQueue := .HasQueueTime }}
{{ $allSuccess := .AllSuccess }}
### Runs ({{ len .Items }} items)

| Name | Job ID{{ if $hasRunner }} | Runner{{ end }}{{ if $hasQueue }} | Queue Time{{ end }}{{ if $hasCost }} | Cost{{ end }} | Duration |{{ if not $allSuccess }} Status |{{ end }}{{ if $hasLogPath }} Log Path |{{ end }}
|------|--------{{ if $hasRunner }}|--------{{ end }}{{ if $hasQueue }}|------------{{ end }}{{ if $hasCost }}|------{{ end }}|----------|{{ if not $allSuccess }}--------|{{ end }}{{ if $hasLogPath }}----------|{{ end }}
{{ range .ItemsByDuration }}| {{ .Name }} | `{{ .JobID }}` {{ if $hasRunner }}| {{ .Runner }} {{ end }}{{ if $hasQueue }}| {{ .QueueDuration }} {{ end }}{{ if $hasCost }}| {{ if .CostGathered }}${{ printf "%.2f" (divideBy1000 .Cost) }}{{ if .CostEstimate }} (est.){{ end }}{{ else }}—{{ end }} {{ end }}| {{ .Duration }} |{{ if not $allSuccess }} {{ conclusionText .Conclusion }} |{{ end }}{{ if $hasLogPath }} {{ if .LogPath }}{{ .LogPath }}{{ else }}—{{ end }} |{{ end }}
{{ end }}
{{ end }}
{{ if .StepSummaries }}
### Step Aggregation Across Matrix
| Step Name | Count | Total | % Total | Median | Max |
|---|---|---|---|---|---|
{{ range .StepSummaries }}{{ if ge .PctTotal 1.0 }}| {{ .Name }} | {{ .Count }} | {{ .TotalDuration }} | {{ printf "%.1f" .PctTotal }}% | {{ .MedianDuration }} | {{ .MaxDuration }} |
{{ end }}{{ end }}
{{ end }}

{{ if .CriticalPath }}
### Critical Path
Total Duration: {{ .CriticalPath.TotalDuration }} (Queue: {{ .CriticalPath.TotalQueue }}, Execution: {{ .CriticalPath.TotalExecution }})

| Job Name | Duration | Queue Time | Slack |
|---|---|---|---|
{{ range .CriticalPath.CriticalNodes }}| `{{ .JobName }}` | {{ .Duration }} | {{ .QueueTime }} | {{ .Slack }} |
{{ end }}

{{ if .CriticalPath.NearCriticalNodes }}
#### Near-Critical Jobs (Slack ≤ 60s)
| Job Name | Duration | Queue Time | Slack |
|---|---|---|---|
{{ range .CriticalPath.NearCriticalNodes }}| `{{ .JobName }}` | {{ .Duration }} | {{ .QueueTime }} | {{ .Slack }} |
{{ end }}
{{ end }}
{{ end }}

{{ if .SlowestJobSteps }}
### Top Slowest Jobs Step Breakdown
{{ range .SlowestJobSteps }}
#### {{ .JobName }} ({{ .Duration }})

{{ $hasNonSuccess := .HasNonSuccessSteps }}
| Step Name | Duration |{{ if $hasNonSuccess }} Status |{{ end }}
|---|---|{{ if $hasNonSuccess }}---|{{ end }}
{{ range .Steps }}| {{ .Name }} | {{ .Duration }} |{{ if $hasNonSuccess }} {{ .Conclusion }} |{{ end }}
{{ end }}
{{ if gt .MinorStepsCount 0 }}| *{{ .MinorStepsCount }} minor steps (≤1s)* | *{{ .MinorStepsTotal }}* |{{ if $hasNonSuccess }} - |{{ end }}
{{ end }}
{{ end }}
{{ end }}
{{ if .QueuedItems }}
### Queued

{{ range .QueuedItems }}- {{ . }}
{{ end }}
{{ end }}
{{ if .SkippedItems }}
### Skipped

{{ range .SkippedItems }}- {{ . }}
{{ end }}
{{ end }}
{{ if .PostTimelineItems }}
### Post-Timeline

{{ range .PostTimelineItems }}- {{ .Name }} ({{ .Time.Format "2006-01-02T15:04:05" }})
{{ end }}
{{ end }}
{{ end }}
