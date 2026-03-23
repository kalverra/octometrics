{{- /* Go Template file */ -}}

{{ define "timeline_md" }}
{{ if .Items }}
```mermaid
gantt
    dateFormat {{ .DateFormat }}
    axisFormat {{ .AxisFormat }}
    {{ $dateFormat := .GoDateFormat }}
    {{ range .Items }}
    {{ sanitizeMermaidName .Name }} :{{ if .Conclusion }}{{ .Conclusion }},{{ end }} {{ .ID }}, {{ .StartTime.Format $dateFormat }}, {{ .Duration.Seconds }}s{{ end }}
```

<details>
<summary>Runtimes ({{ len .Items }} items)</summary>

| Name | Duration | Status |
|------|----------|--------|
{{ range .ItemsByDuration }}| {{ .Name }} | {{ .Duration }} | {{ conclusionText .Conclusion }} |
{{ end }}

</details>
{{ end }}
{{ if .QueuedItems }}
<details>
<summary>Queued</summary>

{{ range .QueuedItems }}- {{ . }}
{{ end }}

</details>
{{ end }}
{{ if .SkippedItems }}
<details>
<summary>Skipped</summary>

{{ range .SkippedItems }}- {{ . }}
{{ end }}

</details>
{{ end }}
{{ if .PostTimelineItems }}
<details>
<summary>Post-Timeline</summary>

{{ range .PostTimelineItems }}- {{ .Name }} ({{ .Time.Format "2006-01-02T15:04:05" }})
{{ end }}

</details>
{{ end }}
{{ end }}
