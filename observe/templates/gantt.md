{{- /* Go Template file */ -}}

{{ define "gantt_md" }}

# [{{ .Name }}]({{ .Link }})

```mermaid
gantt
    dateFormat {{ .DateFormat }}
    axisFormat {{ .AxisFormat}}

    {{ $dateFormat := .GoDateFormat }}
    {{ range .Items }}
    {{ .Name }} :{{ .MermaidID }}, {{ .StartTime.Format $dateFormat }}, {{ .Duration.Seconds }}s{{ end }}
```

{{ end }}
