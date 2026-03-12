package report

import (
	"fmt"

	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/monitor"
)

// Options configures the report command.
type Options struct {
	MonitorFile string
	SkipSummary bool
	SkipComment bool
}

// Run generates a monitoring report, writes it to GITHUB_STEP_SUMMARY, and posts it as a PR comment.
func Run(log zerolog.Logger, opts Options) error {
	analysis, err := monitor.Analyze(log, opts.MonitorFile)
	if err != nil {
		return fmt.Errorf("failed to analyze monitor data: %w", err)
	}

	gha, err := detectGitHubActions()
	if err != nil {
		log.Warn().Err(err).Msg("GitHub Actions not detected, printing report to stdout only")
		fmt.Println(buildReport(analysis, nil, nil))
		return nil
	}

	if gha.JobName == "" && analysis.JobName != "" {
		log.Debug().Str("job_name", analysis.JobName).Msg("Using job name from monitor data as fallback")
		gha.JobName = analysis.JobName
	}

	var steps []*github.TaskStep
	steps, err = fetchJobSteps(log, gha)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch job steps, continuing without step timeline")
	}

	markdown := buildReport(analysis, steps, gha)
	fmt.Print(markdown)

	if !opts.SkipSummary {
		if err := writeSummary(gha.StepSummary, markdown); err != nil {
			log.Error().Err(err).Msg("Failed to write step summary")
		} else {
			log.Info().Msg("Wrote step summary")
		}
	}

	if !opts.SkipComment {
		if err := postComment(log, gha, markdown); err != nil {
			log.Error().Err(err).Msg("Failed to post comment")
		}
	}

	return nil
}
