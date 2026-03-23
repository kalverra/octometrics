{{- /* Go Template file */ -}}

{{ define "pull_request_md" }}

<details>
<summary><strong>Commits</strong></summary>

{{ range . }}
- `{{ slice .GetSHA 0 7 }}` {{ .GetCommit.GetMessage }} ({{ .GetCommit.GetAuthor.GetDate.Time.Format "Jan 2, 2006 15:04" }})
{{ if .GetMergeQueueEvents }}{{ range .GetMergeQueueEvents }}  - Added to merge queue {{ .AddedTime.Format "Jan 2 15:04" }} by {{ .AddedActor }}
{{ if .RemovedTime }}  - Removed from merge queue {{ .RemovedTime.Format "Jan 2 15:04" }} by {{ .RemovedActor }} — {{ .RemovedReason }}
{{ end }}{{ end }}{{ end }}{{ end }}

</details>

{{ end }}
