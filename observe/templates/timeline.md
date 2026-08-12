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
