{{- /* Go Template file */ -}}

{{ define "pull_request_md" }}

<details>
<summary><strong>Commits</strong></summary>

{{ range . }}
{{ $parsed := parseCommitMsg .GetCommit.GetMessage }}
- `{{ slice .GetSHA 0 7 }}` {{ if $parsed.Type }}**{{ $parsed.Type }}**{{ if $parsed.Scope }}({{ $parsed.Scope }}){{ end }}: {{ else if $parsed.Tag }}**{{ $parsed.Tag }}** {{ end }}{{ $parsed.Summary }} ({{ .GetCommit.GetAuthor.GetDate.Time.Format "Jan 2, 2006 15:04" }}) &mdash; ⏱ {{ formatDuration .GetDuration }}, {{ if not .GetCostGathered }}cost not gathered{{ else }}${{ printf "%.2f" (divideBy1000 .GetCost) }}{{ if .GetCostEstimate }} (est.){{ end }}{{ end }}
{{ if .GetMergeQueueEvents }}{{ range .GetMergeQueueEvents }}  - Added to merge queue {{ .AddedTime.Format "Jan 2 15:04" }} by {{ .AddedActor }}
{{ if .RemovedTime }}  - Removed from merge queue {{ .RemovedTime.Format "Jan 2 15:04" }} by {{ .RemovedActor }} — {{ .RemovedReason }}
{{ end }}{{ end }}{{ end }}{{ end }}

</details>

{{ end }}
