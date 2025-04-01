{{- /* Go Template file */ -}}

{{ define "gantt_md" }}

# [{{ .Name }}]({{ .Link }})

## Took {{ .TotalDuration }}

```mermaid
gantt
    dateFormat {{ .DateFormat }}
    axisFormat {{ .AxisFormat}}

    {{ $dateFormat := .GoDateFormat }}
    {{ range .Items }}
    {{ saniMermaidName .Name }} :{{ mermaidID .MermaidID }}, {{ .StartTime.Format $dateFormat }}, {{ .Duration.Seconds }}s{{ end }}
```

{{ end }}
