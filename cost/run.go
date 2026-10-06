package cost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// Options specifies configuration for running the cost report.
type Options struct {
	CSVPath          string
	CloudzeroCSVPath string
	OrgFilter        string
	Format           string
	OutputFile       string
	DataDir          string
}

// RunReport executes the full cost analysis pipeline: parsing, enriching, modeling, and outputting.
func RunReport(ctx context.Context, log zerolog.Logger, client *gather.GitHubClient, opts Options) error {
	if opts.CSVPath == "" {
		return errors.New("usage report CSV path is required")
	}

	file, err := os.Open(opts.CSVPath)
	if err != nil {
		return fmt.Errorf("failed to open usage CSV: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	log.Info().Str("csv", opts.CSVPath).Str("org", opts.OrgFilter).Msg("Parsing GitHub Actions usage report")
	records, err := ParseCSV(file, opts.OrgFilter)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}
	log.Info().Int("records", len(records)).Msg("Parsed usage records")

	// Parse Cloudzero Runs-On actuals if path provided or if runs-on-costs.csv exists
	var czResult *CloudzeroResult
	czPath := opts.CloudzeroCSVPath
	if czPath == "" {
		if _, statErr := os.Stat("runs-on-costs.csv"); statErr == nil {
			czPath = "runs-on-costs.csv"
		}
	}
	if czPath != "" {
		if czFile, openErr := os.Open(filepath.Clean(czPath)); openErr == nil {
			defer func() { _ = czFile.Close() }()
			czParsed, parseErr := ParseCloudzeroCSV(czFile)
			if parseErr == nil && czParsed != nil {
				czResult = czParsed
				log.Info().
					Str("cloudzero_csv", czPath).
					Float64("total_cost", czResult.TotalCost).
					Msg("Loaded Cloudzero Runs-On actual billing")
			}
		}
	}

	// Collect unique self-hosted workflows to resolve runner specs
	uniqueWorkflows := make(map[string]string)
	org := opts.OrgFilter
	for _, rec := range records {
		if rec.RunnerType == RunnerTypeRunsOn {
			key := rec.Repository + ":" + rec.WorkflowPath
			uniqueWorkflows[key] = rec.Repository
			if org == "" && rec.Organization != "" {
				org = rec.Organization
			}
		}
	}

	log.Info().
		Int("unique_self_hosted_workflows", len(uniqueWorkflows)).
		Msg("Resolving runner specs for self-hosted workflows")
	specs := ResolveWorkflowSpecs(ctx, log, client, opts.DataDir, org, uniqueWorkflows)

	report := GenerateReport(records, specs, czResult)

	var output string
	switch strings.ToLower(opts.Format) {
	case "json":
		bytes, jsonErr := FormatJSON(report)
		if jsonErr != nil {
			return fmt.Errorf("failed to format JSON report: %w", jsonErr)
		}
		output = string(bytes)
	case "markdown", "md":
		output = FormatMarkdown(report)
	default:
		output = FormatTable(report)
	}

	if opts.OutputFile != "" {
		if writeErr := os.WriteFile(opts.OutputFile, []byte(output), 0o600); writeErr != nil {
			return fmt.Errorf("failed to write report to file '%s': %w", opts.OutputFile, writeErr)
		}
		log.Info().Str("output_file", opts.OutputFile).Msg("Cost report saved successfully")
	} else {
		fmt.Println(output)
	}

	return nil
}
