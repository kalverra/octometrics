package observe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// SurveyObservation holds the data needed to render a survey percentile page.
type SurveyObservation struct {
	Owner        string
	Repo         string
	Event        string
	Since        time.Time
	Until        time.Time
	TotalRuns    int
	TotalCommits int
	Percentiles  []*PercentileSection
}

// PercentileSection is a single percentile (p50/p75/p95) with its timeline data.
type PercentileSection struct {
	Label        string
	SHA          string
	Duration     time.Duration
	Conclusion   string
	TimelineData []*timelineData
}

// SurveyFromFile loads a SurveyResult JSON and builds a SurveyObservation,
// using already-gathered commit data for detailed Gantt timelines.
func SurveyFromFile(
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	surveyFile string,
	opts ...Option,
) (*SurveyObservation, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	data, err := os.ReadFile(filepath.Clean(surveyFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read survey file: %w", err)
	}

	var result gather.SurveyResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal survey result: %w", err)
	}

	obs := &SurveyObservation{
		Owner:        result.Owner,
		Repo:         result.Repo,
		Event:        result.Event,
		Since:        result.Since,
		Until:        result.Until,
		TotalRuns:    result.TotalRuns,
		TotalCommits: len(result.Commits),
	}

	for _, label := range []string{"p50", "p75", "p95"} {
		cs, ok := result.Percentiles[label]
		if !ok {
			continue
		}

		section := &PercentileSection{
			Label:      label,
			SHA:        cs.SHA,
			Duration:   cs.Duration,
			Conclusion: cs.Conclusion,
		}

		commitData, err := gather.Commit(log, client, owner, repo, cs.SHA, options.gatherOptions...)
		if err != nil {
			log.Warn().Err(err).Str("label", label).Str("sha", cs.SHA).
				Msg("Could not load detailed commit data, falling back to survey summary")
			section.TimelineData = buildSurveyFallbackTimeline(cs)
		} else {
			workflowRuns := make([]*gather.WorkflowRunData, 0, len(commitData.WorkflowRunIDs))
			for _, wfID := range commitData.WorkflowRunIDs {
				wfData, _, wfErr := gather.WorkflowRun(log, client, owner, repo, wfID, options.gatherOptions...)
				if wfErr != nil {
					log.Warn().Err(wfErr).Int64("workflow_run_id", wfID).Msg("Failed to load workflow run")
					continue
				}
				workflowRuns = append(workflowRuns, wfData)
			}
			section.TimelineData = buildCommitTimelineData(commitData, workflowRuns)
		}

		obs.Percentiles = append(obs.Percentiles, section)
	}

	return obs, nil
}

// buildSurveyFallbackTimeline creates a lightweight timeline from survey summary data
// when detailed commit data is not available.
func buildSurveyFallbackTimeline(cs *gather.CommitSurvey) []*timelineData {
	items := make([]timelineItem, 0, len(cs.WorkflowRuns))
	for _, wf := range cs.WorkflowRuns {
		items = append(items, timelineItem{
			Name:       wf.Name,
			ID:         fmt.Sprint(wf.ID),
			StartTime:  wf.StartTime,
			Duration:   wf.Duration,
			Conclusion: conclusionToGanttStatus(wf.Conclusion),
		})
	}
	return []*timelineData{{
		Event: cs.Event,
		Items: items,
	}}
}

// RenderSurvey writes a survey observation to HTML.
func RenderSurvey(log zerolog.Logger, obs *SurveyObservation, outputDir string) (string, error) {
	for _, p := range obs.Percentiles {
		for _, td := range p.TimelineData {
			if err := td.process(); err != nil {
				return "", fmt.Errorf("failed to process timeline for %s: %w", p.Label, err)
			}
		}
	}

	targetDir := filepath.Join(outputDir, "html", obs.Owner, obs.Repo, "surveys")
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create survey output dir: %w", err)
	}

	eventLabel := obs.Event
	if eventLabel == "" {
		eventLabel = "all"
	}
	fileName := fmt.Sprintf("%s_%s_%s.html",
		eventLabel,
		obs.Since.Format("2006-01-02"),
		obs.Until.Format("2006-01-02"),
	)
	targetFile := filepath.Join(targetDir, fileName)

	var buf bytes.Buffer
	if err := htmlTemplate.ExecuteTemplate(&buf, "survey_html", obs); err != nil {
		return "", fmt.Errorf("failed to render survey template: %w", err)
	}

	if err := os.WriteFile(targetFile, buf.Bytes(), 0600); err != nil {
		return "", fmt.Errorf("failed to write survey file: %w", err)
	}

	log.Debug().Str("file", targetFile).Msg("Rendered survey observation")
	return targetFile, nil
}
