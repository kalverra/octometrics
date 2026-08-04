{{- /* Go Template file */ -}}

{{ define "compare_md" }}
# Comparison: {{ .Left.Name }} vs {{ .Right.Name }}

| | Before | After |
|---|---|---|
| **Name** | [{{ .Left.Name }}]({{ .Left.GitHubLink }}) | [{{ .Right.Name }}]({{ .Right.GitHubLink }}) |
| **ID** | {{ if eq .Left.DataType "commit" }}{{ shortSHA .Left.ID }}{{ else }}#{{ .Left.ID }}{{ end }} | {{ if eq .Right.DataType "commit" }}{{ shortSHA .Right.ID }}{{ else }}#{{ .Right.ID }}{{ end }} |
| **State** | {{ .Left.State }} | {{ .Right.State }} |
| **Run at** | {{ if not .Summary.LeftStartedAt.IsZero }}{{ .Summary.LeftStartedAt.Format "Jan 2, 2006 15:04" }}{{ else }}-{{ end }} | {{ if not .Summary.RightStartedAt.IsZero }}{{ .Summary.RightStartedAt.Format "Jan 2, 2006 15:04" }}{{ else }}-{{ end }} |
{{ if or .Summary.LeftCost .Summary.RightCost }}| **Cost** | ${{ printf "%.2f" (divideBy1000 .Summary.LeftCost) }} | ${{ printf "%.2f" (divideBy1000 .Summary.RightCost) }} |
{{ end }}
{{ range .EventPairs }}
<details open>
<summary><strong>{{ .Event }}</strong> — {{ .LeftDuration }} → {{ .RightDuration }} ({{ formatDelta .DurationDelta }})</summary>

| Metric | Before | After | Delta | % Delta |
|---|---|---|---|---|
| Duration | {{ .LeftDuration }} | {{ .RightDuration }} | {{ formatDelta .DurationDelta }} | {{ .DurationDeltaPercent }} |
{{ if or .LeftCost .RightCost }}| Cost | ${{ printf "%.2f" (divideBy1000 .LeftCost) }} | ${{ printf "%.2f" (divideBy1000 .RightCost) }} | ${{ printf "%.2f" (divideBy1000 .CostDelta) }} | {{ .CostDeltaPercent }} |
{{ end }}

{{ if .CombinedGantt }}
### Combined Timeline

{{ template "compare_gantt_md" .CombinedGantt }}
{{ else }}
{{ if .Left }}
### Before — {{ .Left.StartTime.Format "2006-01-02T15:04:05" }} to {{ .Left.EndTime.Format "2006-01-02T15:04:05" }} ({{ .Left.Duration }})

{{ template "timeline_md" .Left }}
{{ else }}
### Before

_No runs for this event._
{{ end }}

{{ if .Right }}
### After — {{ .Right.StartTime.Format "2006-01-02T15:04:05" }} to {{ .Right.EndTime.Format "2006-01-02T15:04:05" }} ({{ .Right.Duration }})

{{ template "timeline_md" .Right }}
{{ else }}
### After

_No runs for this event._
{{ end }}
{{ end }}

{{ if .Items }}
<details open>
<summary>Comparison ({{ len .Items }} matched)</summary>

| Name | Before Duration | After Duration | Delta | Before Status | After Status |
|------|---------------|----------------|-------|-------------|--------------|
{{ range .Items }}| {{ .Name }} | {{ .LeftDuration }} | {{ .RightDuration }} | {{ formatDelta .DurationDelta }} | {{ conclusionText .LeftConclusion }} | {{ conclusionText .RightConclusion }} |
{{ end }}

</details>
{{ end }}

{{ if .OnlyLeft }}
<details>
<summary>Only in Before ({{ len .OnlyLeft }})</summary>

| Name | Duration | Status |
|------|----------|--------|
{{ range .OnlyLeft }}| {{ .Name }} | {{ .Duration }} | {{ conclusionText .Conclusion }} |
{{ end }}

</details>
{{ end }}

{{ if .OnlyRight }}
<details>
<summary>Only in After ({{ len .OnlyRight }})</summary>

| Name | Duration | Status |
|------|----------|--------|
{{ range .OnlyRight }}| {{ .Name }} | {{ .Duration }} | {{ conclusionText .Conclusion }} |
{{ end }}

</details>
{{ end }}

{{ if $.MonitoringPairs }}{{ range $.MonitoringPairs }}
<details>
<summary>{{ .Title }}</summary>

{{ if .LeftDiagram }}
**Before:**

```mermaid
{{ .LeftDiagram }}
```
{{ else }}
**Before:** _No data_
{{ end }}

{{ if .RightDiagram }}
**After:**

```mermaid
{{ .RightDiagram }}
```
{{ else }}
**After:** _No data_
{{ end }}

</details>
{{ end }}{{ end }}

</details>
{{ end }}

{{ if not .EventPairs }}
_No timeline items to compare._
{{ end }}
{{ end }}

{{ define "compare_gantt_md" }}
{{ if .Sections }}
```mermaid
gantt
    dateFormat {{ .DateFormat }}
    axisFormat {{ .AxisFormat }}
    {{ $fmt := .GoDateFormat }}
    {{ range .Sections }}
    section {{ .Label }}
    {{ range .Tasks }}
    {{ sanitizeMermaidName .Name }} :{{ if .Conclusion }}{{ .Conclusion }},{{ end }} {{ .ID }}, {{ .StartTime.Format $fmt }}, {{ .Duration.Seconds }}s
    {{ end }}
    {{ end }}
```
{{ end }}
{{ end }}
