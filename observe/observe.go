package observe

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2/spinner"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/config"
)

//go:embed templates/*.html templates/*.md templates/*.css templates/*.js
var templateFS embed.FS

// Output directory constants for rendered observations.
const (
	// OutputDir is the base directory for all observation output.
	OutputDir         = "observe_output"
	defaultHTMLOutput = "observe_output/html"
	markdownOutputDir = "observe_output/md"
)

var activeHTMLOutputDir = defaultHTMLOutput

func effectiveHTMLOutputDir(custom string) string {
	if custom != "" {
		return custom
	}
	return defaultHTMLOutput
}

func setActiveHTMLOutputDir(custom string) {
	activeHTMLOutputDir = effectiveHTMLOutputDir(custom)
}

var (
	// htmlTemplate is the cached template for HTML rendering
	htmlTemplate *template.Template
	// mdTemplate is the cached template for Markdown rendering
	mdTemplate *template.Template
)

func sharedFuncMap() template.FuncMap {
	return template.FuncMap{
		"sanitizeMermaidName": sanitizeMermaidName,
		"commitRunLink":       commitRunLink,
		"divideBy1000":        func(v int64) float64 { return float64(v) / 1000.0 },
		"joinStrings":         strings.Join,
		"formatDelta":         formatDelta,
		"formatCostDelta":     formatCostDelta,
		"conclusionText":      conclusionText,
		"shortSHA":            shortSHA,
	}
}

func formatCostDelta(costDelta int64) string {
	if costDelta == 0 {
		return "$0.00"
	}
	val := float64(costDelta) / 1000.0
	if val < 0 {
		return fmt.Sprintf("-$%.2f", -val)
	}
	return fmt.Sprintf("+$%.2f", val)
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

func conclusionText(conclusion string) string {
	switch conclusion {
	case "crit":
		return "failure"
	case "done":
		return "cancelled"
	case "active":
		return "in progress"
	default:
		return "success"
	}
}

func templateForFormat(outputType string) (tmpl *template.Template, observationName, compareName string) {
	switch outputType {
	case "md":
		return mdTemplate, "observation_md", "compare_md"
	default:
		return htmlTemplate, "observation_html", "compare_html"
	}
}

func outputDirForFormat(outputType string) string {
	if outputType == "md" {
		return markdownOutputDir
	}
	return activeHTMLOutputDir
}

// WriteStaticAssets writes static CSS and JS files to the given output directory.
func WriteStaticAssets(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	styles, err := templateFS.ReadFile("templates/styles.css")
	if err != nil {
		return fmt.Errorf("failed to read styles.css: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "styles.css"), styles, 0600); err != nil {
		return fmt.Errorf("failed to write styles.css: %w", err)
	}

	mermaidJS, err := templateFS.ReadFile("templates/mermaid-init.js")
	if err != nil {
		return fmt.Errorf("failed to read mermaid-init.js: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "mermaid-init.js"), mermaidJS, 0600); err != nil {
		return fmt.Errorf("failed to write mermaid-init.js: %w", err)
	}

	exportJS, err := templateFS.ReadFile("templates/export-png.js")
	if err != nil {
		return fmt.Errorf("failed to read export-png.js: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "export-png.js"), exportJS, 0600); err != nil {
		return fmt.Errorf("failed to write export-png.js: %w", err)
	}

	searchJS, err := templateFS.ReadFile("templates/search.js")
	if err != nil {
		return fmt.Errorf("failed to read search.js: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "search.js"), searchJS, 0600); err != nil {
		return fmt.Errorf("failed to write search.js: %w", err)
	}

	// Clean up legacy index.html files in outputDir
	_ = filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == "index.html" {
			//nolint:gosec // safe removal of legacy index.html
			_ = os.Remove(path)
		}
		return nil
	})

	return nil
}

func init() {
	var err error

	htmlFuncs := sharedFuncMap()
	htmlFuncs["mermaidDiagram"] = func(s string) template.HTML { return template.HTML(s) }
	htmlFuncs["deltaClass"] = func(d time.Duration) string {
		if d > 0 {
			return "delta-slower"
		}
		if d < 0 {
			return "delta-faster"
		}
		return ""
	}
	htmlFuncs["deltaCostClass"] = func(costDelta int64) string {
		if costDelta > 0 {
			return "delta-slower"
		}
		if costDelta < 0 {
			return "delta-faster"
		}
		return ""
	}
	htmlFuncs["formatTime"] = func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Format("2006-01-02 15:04:05")
	}
	htmlFuncs["conclusionBadge"] = func(conclusion string) template.HTML {
		text := conclusionText(conclusion)
		cssClass := strings.ReplaceAll(text, " ", "-")
		return template.HTML(fmt.Sprintf(`<span class="rt-badge rt-%s">%s</span>`, cssClass, text))
	}

	htmlTemplate, err = template.New("observation_html").Funcs(htmlFuncs).
		ParseFS(templateFS, "templates/*.html")
	if err != nil {
		panic(fmt.Errorf("failed to parse HTML templates: %w", err))
	}

	mdTemplate, err = template.New("observation_md").Funcs(sharedFuncMap()).
		ParseFS(templateFS, "templates/*.md")
	if err != nil {
		panic(fmt.Errorf("failed to parse Markdown templates: %w", err))
	}
}

// Option manipulates how the observe command works
type Option func(*options)

// options contains the options for the observe command
type options struct {
	outputDir        string
	gatherOptions    []gather.Option
	excludeWorkflows []string
	includeWorkflows []string
}

func defaultOptions() *options {
	return &options{
		gatherOptions: []gather.Option{},
		outputDir:     OutputDir,
	}
}

// WithCustomOutputDir sets the output directory for the observe command.
// This is useful for testing and debugging purposes.
func WithCustomOutputDir(outputDir string) Option {
	return func(o *options) {
		o.outputDir = outputDir
	}
}

// WithGatherOptions sets the gather options for the observe command.
// Observe uses gather to get data, so you can pass options to gather from here.
func WithGatherOptions(opts ...gather.Option) Option {
	return func(o *options) {
		o.gatherOptions = opts
	}
}

// ExcludeWorkflows sets workflow display names to omit from observations.
func ExcludeWorkflows(names []string) Option {
	return func(o *options) {
		o.excludeWorkflows = names
	}
}

// IncludeWorkflows sets workflow display names to include in observations.
// When set, only workflows matching these names are observed.
func IncludeWorkflows(names []string) Option {
	return func(o *options) {
		o.includeWorkflows = names
	}
}

// shouldIncludeWorkflow returns true if the workflow name should be included
// based on the observe options. Exclude takes precedence over include.
func shouldIncludeWorkflow(name string, opts *options) bool {
	if opts == nil {
		return true
	}
	if slices.Contains(opts.excludeWorkflows, name) {
		return false
	}
	if len(opts.includeWorkflows) == 0 {
		return true
	}
	return slices.Contains(opts.includeWorkflows, name)
}

// Observation represents a single observation of a PR, commit, or workflow run, or job.
// It contains all the data used to render the observation in the different formats.
type Observation struct {
	ID         string
	Name       string
	GitHubLink string
	Owner      string
	Repo       string
	DataType   string
	State      string
	Actor      string
	Runner     string
	Cost       int64 // Cost in tenths of a cent
	// CostEstimate is true when Cost includes estimated costs (e.g. runs-on runners)
	CostEstimate bool
	// CostGathered is true when cost data was gathered (billing API or log parsing)
	CostGathered bool

	// Branch protection: required status checks for the default branch
	RequiredWorkflows       []string
	BranchProtectionWarning bool

	// Data used to show job, workflow, and commit runs
	TimelineData   []*Timeline
	MonitoringData *Monitoring
	// FlowChart is a Mermaid flowchart TD string showing job dependencies
	FlowChart string

	// Data used to render a Pull Request with multiple commits
	CommitData []*gather.CommitData
}

// Render writes the observation to a file in the specified output format (html or md).
func (o *Observation) Render(
	log zerolog.Logger,
	outputType string,
	opts ...Option,
) (observationFile string, err error) {
	observeOpts := defaultOptions()
	for _, opt := range opts {
		opt(observeOpts)
	}
	baseDir := observeOpts.outputDir
	if outputType == "md" {
		baseDir = markdownOutputDir
	}
	if o.ID == "" {
		log.Warn().Msg("Observation ID is empty, skipping rendering")
		return "", nil
	}

	observationFile = filepath.Join(
		baseDir,
		o.Owner,
		o.Repo,
		o.DataType+"s",
		fmt.Sprintf("%s.%s", o.ID, outputType),
	)
	log = log.With().
		Str("observation_id", o.ID).
		Str("observation_name", o.Name).
		Str("observation_github_link", o.GitHubLink).
		Str("observation_owner", o.Owner).
		Str("observation_repo", o.Repo).
		Str("observation_data_type", o.DataType).
		Str("output_type", outputType).
		Str("observation_file", observationFile).
		Logger()

	var (
		start = time.Now()
		buf   bytes.Buffer
	)

	sort.Slice(o.TimelineData, func(i, j int) bool {
		return o.TimelineData[i].StartTime.Before(o.TimelineData[j].StartTime)
	})

	buf, err = o.renderToFormat(outputType)
	if err != nil {
		return "", fmt.Errorf("failed to render observation to %s: %w", outputType, err)
	}

	//nolint:gosec // directory path built safely
	err = os.MkdirAll(filepath.Dir(observationFile), 0750)
	if err != nil {
		return "", fmt.Errorf("failed to create observation file directory: %w", err)
	}
	//nolint:gosec // observation file path built safely
	err = os.WriteFile(observationFile, buf.Bytes(), 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write observation file: %w", err)
	}
	log.Trace().
		Str("duration", time.Since(start).String()).
		Msg("Rendered observation")
	return observationFile, nil
}

// RenderString renders the observation to a string in the specified format ("html" or "md").
func (o *Observation) RenderString(
	_ zerolog.Logger,
	outputType string,
) (string, error) {
	sort.Slice(o.TimelineData, func(i, j int) bool {
		return o.TimelineData[i].StartTime.Before(o.TimelineData[j].StartTime)
	})
	buf, err := o.renderToFormat(outputType)
	if err != nil {
		return "", fmt.Errorf("failed to render observation to %s: %w", outputType, err)
	}
	return buf.String(), nil
}

func (o *Observation) renderToFormat(outputType string) (bytes.Buffer, error) {
	tmpl, name, _ := templateForFormat(outputType)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, o); err != nil {
		return buf, err
	}
	return buf, nil
}

// Interactive generates downloaded data in HTML lazily on demand and serves it on a local server.
// If initialPath is non-empty, the target entity is rendered before the browser opens.
func Interactive(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	initialPath, dataDir string,
	opts ...Option,
) error {
	startTime := time.Now()
	observeOpts := defaultOptions()
	for _, opt := range opts {
		opt(observeOpts)
	}
	setActiveHTMLOutputDir(observeOpts.outputDir)

	if err := WriteStaticAssets(activeHTMLOutputDir); err != nil {
		return fmt.Errorf("failed to write static assets: %w", err)
	}

	handler := NewOnDemandHandler(log, client, dataDir, activeHTMLOutputDir, opts...)

	if initialPath != "" {
		parts := strings.Split(strings.Trim(initialPath, "/"), "/")
		if len(parts) == 4 {
			owner, repo, category, filename := parts[0], parts[1], parts[2], parts[3]
			ext := filepath.Ext(filename)
			id := strings.TrimSuffix(filename, ext)
			format := strings.TrimPrefix(ext, ".")
			if format == "" {
				format = "html"
			}
			if err := handler.renderEntity(ctx, owner, repo, category, id, format); err != nil {
				log.Warn().Err(err).Str("path", initialPath).Msg("failed synchronous pre-warm for initialPath")
			}
		}
	}

	log.Info().
		Str("url", "http://localhost:8080"+initialPath).
		Str("built_observations_dur", time.Since(startTime).String()).
		Str("html_dir", activeHTMLOutputDir).
		Str("md_dir", markdownOutputDir).
		Msg("Observing data...")
	fmt.Println("Observe data at http://localhost:8080")
	fmt.Printf("Markdown files written to %s/\n", markdownOutputDir)

	return ServeHTMLWithHandler(log, initialPath, handler)
}

// ServeHTML starts a local HTTP server for the HTML output directory using OnDemandHandler
// and opens the browser to the specified initial path.
func ServeHTML(log zerolog.Logger, initialPath string) error {
	handler := NewOnDemandHandler(log, nil, config.DefaultDataDir(), activeHTMLOutputDir)
	return ServeHTMLWithHandler(log, initialPath, handler)
}

// ServeHTMLWithHandler starts a local HTTP server with the given handler.
func ServeHTMLWithHandler(log zerolog.Logger, initialPath string, handler http.Handler) error {
	var (
		baseURL    = "http://localhost:8080"
		browserURL = baseURL + initialPath
	)

	//nolint:gosec // I don't care
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		return fmt.Errorf("failed to listen on :8080: %w", err)
	}

	// Wait a moment for server to start before opening browser
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = openBrowser(log, browserURL)
	}()

	//nolint:gosec // I don't care
	return http.Serve(l, handler)
}

func openBrowser(log zerolog.Logger, url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}

	if runtime.GOOS == "windows" {
		cmd = "explorer"
	}

	args = append(args, url)
	//nolint:gosec // I don't care
	if err := exec.Command(cmd, args...).Run(); err != nil {
		log.Error().Err(err).Msg("Failed to open browser")
		return err
	}
	return nil
}

// All generates observations for all gathered data in the specified output formats.
func All(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	outputTypes []string,
	dataDir string,
	opts ...Option,
) error {
	var (
		startTime = time.Now()
		err       error
	)
	spinnerErr := spinner.New().
		Title("Building observations").
		Action(func() {
			err = generateAllObserveData(ctx, log, client, outputTypes, dataDir, opts...)
		}).
		Run()
	if err != nil {
		return err
	}
	if spinnerErr != nil {
		return spinnerErr
	}

	fmt.Printf("Observations built (%s) ✅\n", time.Since(startTime).Round(10*time.Millisecond).String())
	return nil
}

func generateAllObserveData(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	outputTypes []string,
	dataDir string,
	opts ...Option,
) error {
	observeOpts := defaultOptions()
	for _, opt := range opts {
		opt(observeOpts)
	}
	setActiveHTMLOutputDir(observeOpts.outputDir)
	if err := WriteStaticAssets(activeHTMLOutputDir); err != nil {
		return fmt.Errorf("failed to write static assets: %w", err)
	}

	bpCache := make(map[string]*gather.BranchProtectionResult)

	var (
		jsonLoadDur  time.Duration
		renderDur    time.Duration
		filesWritten int
		filesSkipped int
	)

	err := filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == ".DS_Store" {
			//nolint:gosec // I don't care
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove .DS_Store file: %w", err)
			}
			return nil
		}

		if filepath.Ext(path) != ".json" {
			log.Debug().Str("path", path).Msg("Skipping non-JSON file")
			return nil
		}

		relPath, relErr := filepath.Rel(dataDir, path)
		if relErr != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", path, relErr)
		}
		pathComponents := strings.Split(relPath, string(filepath.Separator))
		if len(pathComponents) != 4 {
			log.Debug().Str("path", path).Msg("Skipping path not matching owner/repo/category/file.json")
			return nil
		}
		var (
			owner    = pathComponents[0]
			repo     = pathComponents[1]
			dataCat  = pathComponents[2]
			dataName = strings.TrimSuffix(pathComponents[3], ".json")
		)

		loadStart := time.Now()
		var observations []*Observation
		observations, err = loadObservationsFromJSON(
			ctx,
			log,
			client,
			owner,
			repo,
			dataCat,
			dataName,
			observeOpts,
			opts...)
		jsonLoadDur += time.Since(loadStart)

		if err != nil {
			return fmt.Errorf("failed to generate observe data: %w", err)
		}

		repoKey := owner + "/" + repo
		if _, ok := bpCache[repoKey]; !ok {
			bp, bpErr := gather.BranchProtection(ctx, log, client, owner, repo)
			if bpErr != nil {
				log.Warn().Err(bpErr).
					Str("owner", owner).Str("repo", repo).
					Msg("Failed to fetch branch protection; continuing without it")
				bp = &gather.BranchProtectionResult{}
			}
			bpCache[repoKey] = bp
		}
		applyBranchProtection(observations, bpCache[repoKey])

		for _, outputType := range outputTypes {
			for _, obs := range observations {
				if obs == nil {
					return fmt.Errorf("found a nil observation, this should never happen")
				}
				targetPath := filepath.Join(
					outputDirForFormat(outputType),
					obs.Owner,
					obs.Repo,
					obs.DataType+"s",
					fmt.Sprintf("%s.%s", obs.ID, outputType),
				)
				if stat, statErr := os.Stat(targetPath); statErr == nil {
					if srcStat, sErr := os.Stat(path); sErr == nil && stat.ModTime().After(srcStat.ModTime()) {
						filesSkipped++
						continue
					}
				}
				filesWritten++
				renderStart := time.Now()
				_, err = obs.Render(log, outputType, opts...)
				renderDur += time.Since(renderStart)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	log.Debug().
		Str("json_load_dur", jsonLoadDur.String()).
		Str("render_dur", renderDur.String()).
		Int("files_written", filesWritten).
		Int("files_skipped", filesSkipped).
		Msg("Observe phase timings")

	return nil
}

// applyBranchProtection attaches branch protection data to a set of observations
// and marks timeline items whose names match a required status check.
func applyBranchProtection(observations []*Observation, bp *gather.BranchProtectionResult) {
	if bp == nil {
		return
	}
	for _, obs := range observations {
		if obs == nil {
			continue
		}
		if bp.PermissionDenied {
			obs.BranchProtectionWarning = true
			continue
		}
		obs.RequiredWorkflows = bp.RequiredChecks
		for _, td := range obs.TimelineData {
			for i := range td.Items {
				td.Items[i].IsRequired = isRequiredCheck(td.Items[i].Name, bp.RequiredChecks)
			}
		}
	}
}

// isRequiredCheck returns true if itemName matches any required check.
// Matches on exact equality, or when one is a prefix of the other
// separated by " / " (GitHub Actions uses "WorkflowName / JobName").
func isRequiredCheck(itemName string, requiredChecks []string) bool {
	for _, rc := range requiredChecks {
		if itemName == rc {
			return true
		}
		if strings.HasPrefix(itemName, rc+" / ") || strings.HasPrefix(rc, itemName+" / ") {
			return true
		}
	}
	return false
}

func loadObservationsFromJSON(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo, dataCat, dataName string,
	observeOpts *options,
	opts ...Option,
) ([]*Observation, error) {
	var observations []*Observation
	switch dataCat {
	case gather.WorkflowRunsDataDir:
		workflowRunID, err := strconv.ParseInt(dataName, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow run ID: %w", err)
		}
		wfData, _, err := gather.WorkflowRun(ctx, log, client, owner, repo, workflowRunID, observeOpts.gatherOptions...)
		if err != nil {
			return nil, fmt.Errorf("failed to load workflow run data: %w", err)
		}
		observation, err := workflowRunObservation(wfData)
		if err != nil {
			return nil, fmt.Errorf("failed to generate workflow run observation: %w", err)
		}
		if !shouldIncludeWorkflow(observation.Name, observeOpts) {
			log.Debug().
				Str("workflow", observation.Name).
				Int64("workflow_run_id", workflowRunID).
				Msg("Excluding workflow from visualization")
			return nil, nil
		}
		observations = append(observations, observation)
		jobRuns, jobErr := jobRunObservations(wfData)
		if jobErr != nil {
			return nil, fmt.Errorf("failed to generate job runs: %w", jobErr)
		}
		observations = append(observations, jobRuns...)
	case gather.PullRequestsDataDir:
		pullRequestNumber, err := strconv.ParseInt(dataName, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pull request number: %w", err)
		}
		observation, err := PullRequest(ctx, log, client, owner, repo, int(pullRequestNumber), opts...)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	case gather.CommitsDataDir:
		observation, err := Commit(ctx, log, client, owner, repo, dataName, opts...)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, nil
}
