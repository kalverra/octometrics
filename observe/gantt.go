package observe

import (
	"bytes"
	"fmt"
	htmlTemplate "html/template"
	"os"
	"path/filepath"
	"time"
)

type ganttData struct {
	ID           string
	Name         string
	Link         string
	DateFormat   string
	AxisFormat   string
	GoDateFormat string
	Items        []ganttItem
	Owner        string
	Repo         string
	DataType     string
}

type ganttItem struct {
	Name       string
	MermaidID  string
	StartTime  time.Time
	Duration   time.Duration
	Conclusion string
	Link       string
}

func renderGantt(ganttData *ganttData, outputTypes []string) error {
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

		tmpl, err := htmlTemplate.New(fmt.Sprintf("gantt_%s", outputType)).ParseFiles(
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

		outputFile := filepath.Join(
			baseOutputDir,
			ganttData.Owner,
			ganttData.Repo,
			ganttData.DataType+"s",
			fmt.Sprintf("%s.%s", ganttData.ID, outputType),
		)
		err = os.MkdirAll(filepath.Dir(outputFile), 0755)
		if err != nil {
			return fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
		}
		err = os.WriteFile(outputFile, rendered.Bytes(), 0644)
		if err != nil {
			return fmt.Errorf("failed to write %s file: %w", outputType, err)
		}
	}
	return nil
}

func ganttDetermineDateFormat(start, end time.Time) (mermaidDateFormat, mermaidAxisFormat, goDateFormat string) {
	diff := end.Sub(start)
	if diff.Hours() > 24 {
		return "YYYY-MM-DD HH:mm:ss", "%Y-%m-%d %H:%M:%S", "2006-01-02 15:04:05"
	}
	if diff > time.Hour {
		return "HH:mm:ss", "%H:%M:%S", "15:04:05"
	}
	return "mm:ss", "%M:%S", "04:05"
}
