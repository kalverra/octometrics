package observe

import (
	"bytes"
	"fmt"
	htmlTemplate "html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog/log"
)

type ganttData struct {
	ID       string
	Name     string
	Link     string
	Items    []ganttItem
	Owner    string
	Repo     string
	DataType string
	Cost     int64 // Cost in tenths of a cent

	// Set by the renderer
	StartTime    time.Time
	EndTime      time.Time
	GoDateFormat string
	DateFormat   string
	AxisFormat   string
	Duration     time.Duration
}

type ganttItem struct {
	Name       string
	StartTime  time.Time
	Duration   time.Duration
	Conclusion string
	Link       string
}

func renderGantt(ganttData *ganttData, outputTypes []string) error {
	if ganttData == nil {
		return fmt.Errorf("ganttData is nil")
	}
	if len(ganttData.Items) == 0 {
		log.Warn().Str("name", ganttData.Name).Str("id", ganttData.ID).Msg("No items to render in Gantt chart")
		return nil
	}

	sort.Slice(ganttData.Items, func(i, j int) bool {
		if ganttData.Items[i].StartTime.Equal(ganttData.Items[j].StartTime) {
			return ganttData.Items[i].Duration < ganttData.Items[j].Duration
		}
		return ganttData.Items[i].StartTime.Before(ganttData.Items[j].StartTime)
	})

	// Determine the total duration of the Gantt chart
	startTime := ganttData.Items[0].StartTime
	endTime := ganttData.Items[0].StartTime.Add(ganttData.Items[0].Duration)
	for _, item := range ganttData.Items {
		if item.StartTime.Before(startTime) {
			startTime = item.StartTime
		}
		if item.StartTime.Add(item.Duration).After(endTime) {
			endTime = item.StartTime.Add(item.Duration)
		}
	}
	ganttData.Duration = endTime.Sub(startTime)
	ganttData.StartTime = startTime
	ganttData.EndTime = endTime

	// Adjust the start time of each item so that you start at 0
	newStartTime := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	startTimeDiff := newStartTime.Sub(startTime)
	for i := range ganttData.Items {
		ganttData.Items[i].StartTime = ganttData.Items[i].StartTime.Add(startTimeDiff)
	}

	ganttData.DateFormat, ganttData.AxisFormat, ganttData.GoDateFormat = ganttDetermineDateFormat(
		startTime,
		endTime,
	)

	for _, outputType := range outputTypes {
		var baseOutputDir string
		switch outputType {
		case "html":
			baseOutputDir = htmlOutputDir
		case "md":
			baseOutputDir = markdownOutputDir
		default:
			return fmt.Errorf("unknown output type '%s'", outputType)
		}
		outputFile := filepath.Join(
			baseOutputDir,
			ganttData.Owner,
			ganttData.Repo,
			ganttData.DataType+"s",
			fmt.Sprintf("%s.%s", ganttData.ID, outputType),
		)
		if _, err := os.Stat(outputFile); err == nil {
			log.Trace().Msg("Gantt chart already exists, skipping")
			continue
		}

		tmpl, err := htmlTemplate.New(fmt.Sprintf("gantt_%s", outputType)).Funcs(template.FuncMap{
			"saniMermaidName": saniMermaidName,
			"mermaidID":       mermaidID,
		}).ParseFiles(
			filepath.Join(templatesDir, fmt.Sprintf("gantt.%s", outputType)),
		)
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}

		var rendered bytes.Buffer
		err = tmpl.Execute(&rendered, ganttData)
		if err != nil {
			return fmt.Errorf("failed to execute template: %w", err)
		}

		err = os.MkdirAll(filepath.Dir(outputFile), 0700)
		if err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		err = os.WriteFile(outputFile, rendered.Bytes(), 0600)
		if err != nil {
			return fmt.Errorf("failed to write %s file: %w", outputType, err)
		}
	}
	return nil
}

func ganttDetermineDateFormat(start, end time.Time) (mermaidDateFormat, mermaidAxisFormat, goDateFormat string) {
	if end.Sub(start) >= time.Hour {
		return "HH:mm:ss", "%H:%M:%S", "15:04:05"
	}
	return "mm:ss", "%M:%S", "04:05"
}

func saniMermaidName(s string) string {
	s = strings.ReplaceAll(s, ":", "#colon;")
	s = strings.ReplaceAll(s, ",", "<comma>")
	return s
}

func mermaidID(s string) string {
	return strings.ReplaceAll(saniMermaidName(s), " ", "_")
}
