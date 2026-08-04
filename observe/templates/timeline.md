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

{{ $hasRunner := .HasRunner }}
{{ $hasCost := .HasCost }}
<details>
<summary>Runs ({{ len .Items }} items)</summary>

| Name{{ if $hasRunner }} | Runner{{ end }}{{ if $hasCost }} | Cost{{ end }} | Duration | Status |
|------{{ if $hasRunner }}|--------{{ end }}{{ if $hasCost }}|------{{ end }}|----------|--------|
{{ range .ItemsByDuration }}| {{ .Name }} {{ if $hasRunner }}| {{ .Runner }} {{ end }}{{ if $hasCost }}| {{ if .CostGathered }}${{ printf "%.2f" (divideBy1000 .Cost) }}{{ if .CostEstimate }} (est.){{ end }}{{ else }}—{{ end }} {{ end }}| {{ .Duration }} | {{ conclusionText .Conclusion }} |
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
