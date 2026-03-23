{{- /* Go Template file */ -}}

{{ define "observation_md" }}
# [{{ .Name }}]({{ .GitHubLink }})

| | |
|---|---|
| **State** | {{ .State }} |
| **Actor** | {{ .Actor }} |
{{ if .Cost }}| **Cost** | ${{ printf "%.2f" (divideBy1000 .Cost) }} |
{{ end }}{{ if .RequiredWorkflows }}| **Required** | {{ joinStrings .RequiredWorkflows ", " }} |
{{ end }}
{{ if .BranchProtectionWarning }}
> **Warning:** Required workflows could not be loaded (insufficient permissions).
{{ end }}
{{ range .TimelineData }}
<details open>
<summary><strong>{{ .Event }}</strong> — {{ .StartTime.Format "2006-01-02T15:04:05" }} to {{ .EndTime.Format "2006-01-02T15:04:05" }} ({{ .Duration }})</summary>

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
{{ end }}
