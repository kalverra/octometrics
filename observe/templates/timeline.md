{{- /* Go Template file */ -}}

{{ define "timeline_md" }}

### Took {{ .TotalDuration }}

```mermaid
gantt
    dateFormat {{ .DateFormat }}
    axisFormat {{ .AxisFormat}}

    {{ $dateFormat := .GoDateFormat }}
    {{ range .Items }}
    {{ sanitizeMermaidName .Name }} :{{ .ID }}, {{ .StartTime.Format $dateFormat }}, {{ .Duration.Seconds }}s{{ end }}
```
{{ if .SkippedItems }}
### Skipped Items
{{ range .SkippedItems }}
* {{ .Name }}{{ end }}
{{ end }}


{{ end }}
