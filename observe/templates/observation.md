{{- /* Go Template file */ -}}

{{ define "observation_md" }}
# [{{ .Name }}]({{ .GitHubLink }})

| | |
|---|---|
| **State** | {{ .State }} |
| **Actor** | {{ .Actor }} |
{{ if not .CostGathered }}| **Cost** | not gathered |
{{ else }}| **Cost{{ if .CostEstimate }} (est.){{ else }} (exact){{ end }}** | ${{ printf "%.2f" (divideBy1000 .Cost) }} |
{{ end }}{{ if .RequiredWorkflows }}| **Required** | {{ joinStrings .RequiredWorkflows ", " }} |
{{ end }}
{{ if .BranchProtectionWarning }}
> **Warning:** Required workflows could not be loaded (insufficient permissions).
{{ end }}
{{ range .TimelineData }}
<details open>
<summary><strong>{{ .Event }}</strong> — {{ .Duration }}, {{ if not $.CostGathered }}cost not gathered{{ else }}${{ printf "%.2f" (divideBy1000 $.Cost) }}{{ if $.CostEstimate }} (est.){{ else }} (exact){{ end }}{{ end }} ({{ .StartTime.Format "2006-01-02T15:04:05" }} to {{ .EndTime.Format "2006-01-02T15:04:05" }})</summary>

{{ template "timeline_md" . }}
{{ if $.MonitoringData }}{{ range $.MonitoringData.Charts }}
<details>
<summary>{{ .Title }}</summary>

```mermaid
{{ .Diagram }}
```

</details>
{{ end }}{{ end }}

</details>
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
