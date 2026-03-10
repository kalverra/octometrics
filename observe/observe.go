package observe

import (
	"bytes"
	"embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

//go:embed templates/*.html templates/*.md templates/*.css
var templateFS embed.FS

// Output directory constants for rendered observations.
const (
	// OutputDir is the base directory for all observation output.
	OutputDir         = "observe_output"
	htmlOutputDir     = "observe_output/html"
	markdownOutputDir = "observe_output/md"
)

var (
	// htmlTemplate is the cached template for HTML rendering
	htmlTemplate *template.Template
	// mdTemplate is the cached template for Markdown rendering
	mdTemplate *template.Template
)

func init() {
	var err error
	// Initialize HTML template
	htmlTemplate, err = template.New("observation_html").Funcs(template.FuncMap{
		"sanitizeMermaidName": sanitizeMermaidName,
		"commitRunLink":       commitRunLink,
		"divideBy1000": func(v int64) float64 {
			return float64(v) / 1000.0
		},
		"joinStrings": strings.Join,
	}).ParseFS(templateFS, "templates/*.html", "templates/*.css")
	if err != nil {
		panic(fmt.Errorf("failed to parse HTML templates: %w", err))
	}

	// Initialize Markdown template
	mdTemplate, err = template.New("observation_md").Funcs(template.FuncMap{
		"sanitizeMermaidName": sanitizeMermaidName,
	}).ParseFS(templateFS, "templates/*.md")
	if err != nil {
		panic(fmt.Errorf("failed to parse Markdown templates: %w", err))
	}
}

// Option manipulates how the observe command works
type Option func(*options)

// options contains the options for the observe command
type options struct {
	outputDir     string
	gatherOptions []gather.Option
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

// IndexPage is the data passed to the index_html template for navigation pages.
type IndexPage struct {
	Title       string
	Breadcrumbs []Breadcrumb
	Dirs        []IndexDir
	Items       []IndexItem
}

// Breadcrumb is a single segment of the breadcrumb navigation.
type Breadcrumb struct {
	Name string
	Path string
}

// IndexDir is a subdirectory entry in an index page.
type IndexDir struct {
	Name  string
	Path  string
	Count int
}

// IndexItem is an observation entry in an index page.
type IndexItem struct {
	Name  string
	Path  string
	State string
	Actor string
}

func renderIndex(targetFile string, page IndexPage) error {
	var buf bytes.Buffer
	if err := htmlTemplate.ExecuteTemplate(&buf, "index_html", page); err != nil {
		return fmt.Errorf("failed to render index page: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetFile), 0750); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}
	return os.WriteFile(targetFile, buf.Bytes(), 0600)
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
	Cost       int64 // Cost in tenths of a cent

	// Branch protection: required status checks for the default branch
	RequiredWorkflows       []string
	BranchProtectionWarning bool

	// Data used to show job, workflow, and commit runs
	TimelineData   []*timelineData
	MonitoringData *monitoringData

	// Data used to render a Pull Request with multiple commits
	CommitData []*gather.CommitData
}

// Render writes the observation to a file in the specified output format (html or md).
func (o *Observation) Render(log zerolog.Logger, outputType string) (observationFile string, err error) {
	var baseDir string
	switch outputType {
	case "html":
		baseDir = htmlOutputDir
	case "md":
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

	if _, err := os.Stat(observationFile); err == nil {
		log.Trace().
			Msg("Observation file already exists")
		return observationFile, nil
	}

	var (
		start = time.Now()
		buf   bytes.Buffer
	)

	if o.TimelineData != nil {
		for _, timelineData := range o.TimelineData {
			if err := timelineData.process(); err != nil {
				return "", fmt.Errorf("failed to process timeline data: %w", err)
			}
		}
	}

	sort.Slice(o.TimelineData, func(i, j int) bool {
		return o.TimelineData[i].StartTime.Before(o.TimelineData[j].StartTime)
	})

	switch outputType {
	case "html":
		buf, err = o.renderHTML()
	case "md":
		buf, err = o.renderMarkdown()
	}
	if err != nil {
		return "", fmt.Errorf("failed to render observation to %s: %w", outputType, err)
	}

	err = os.MkdirAll(filepath.Dir(observationFile), 0750)
	if err != nil {
		return "", fmt.Errorf("failed to create observation file directory: %w", err)
	}
	err = os.WriteFile(observationFile, buf.Bytes(), 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write observation file: %w", err)
	}
	log.Trace().
		Str("duration", time.Since(start).String()).
		Msg("Rendered observation")
	return observationFile, nil
}

// renderHTML renders the observation to an HTML format
func (o *Observation) renderHTML() (bytes.Buffer, error) {
	var buf bytes.Buffer

	err := htmlTemplate.ExecuteTemplate(&buf, "observation_html", o)
	if err != nil {
		return buf, err
	}
	return buf, nil
}

// renderMarkdown renders the observation to a Markdown format
func (o *Observation) renderMarkdown() (bytes.Buffer, error) {
	var buf bytes.Buffer

	err := mdTemplate.ExecuteTemplate(&buf, "observation_md", o)
	if err != nil {
		return buf, err
	}
	if buf.Len() == 0 {
		return buf, fmt.Errorf("no data to render")
	}
	return buf, nil
}

// Interactive generates all downloaded data in HTML and serves it on a local server.
// If initialPath is non-empty, the browser opens directly to that path (e.g. "/owner/repo/workflow_runs/123.html").
func Interactive(log zerolog.Logger, client *gather.GitHubClient, initialPath string) error {
	startTime := time.Now()
	err := All(log, client, []string{"html"})
	if err != nil {
		return fmt.Errorf("failed to generate all HTML observe data: %w", err)
	}
	var (
		baseURL    = "http://localhost:8080"
		browserURL = baseURL + initialPath
		dir        = http.Dir(htmlOutputDir)
		fs         = http.FileServer(dir)
	)
	http.Handle("/", fs)

	log.Info().
		Str("url", browserURL).
		Str("built_observations_dur", time.Since(startTime).String()).
		Str("dir", htmlOutputDir).
		Msg("Observing data...")
	fmt.Println("Observe data at http://localhost:8080")

	go func() {
		interruptChan := make(chan os.Signal, 1)
		signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

		err := openBrowser(browserURL)
		if err != nil {
			log.Error().Err(err).Msg("Failed to open browser")
		}

		<-interruptChan
		log.Info().Msg("Shutting down server")
		os.Exit(0)
	}()
	//nolint:gosec // I don't care
	return http.ListenAndServe(":8080", nil)
}

func openBrowser(url string) error {
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
	return exec.Command(cmd, args...).Run()
}

// All generates observations for all gathered data in the specified output formats.
func All(log zerolog.Logger, client *gather.GitHubClient, outputTypes []string) error {
	return generateAllObserveData(log, client, outputTypes)
}

type categoryKey struct {
	owner, repo, category string
}

func generateAllObserveData(log zerolog.Logger, client *gather.GitHubClient, outputTypes []string) error {
	collected := make(map[categoryKey][]IndexItem)
	bpCache := make(map[string]*gather.BranchProtectionResult)

	err := filepath.WalkDir(gather.DataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == ".DS_Store" {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove .DS_Store file: %w", err)
			}
			return nil
		}

		if filepath.Ext(path) != ".json" {
			return fmt.Errorf("file %s is not a JSON file", path)
		}

		pathComponents := strings.Split(path, string(filepath.Separator))
		if len(pathComponents) != 5 {
			return fmt.Errorf("unexpected path format: %s", path)
		}
		var (
			owner        = pathComponents[1]
			repo         = pathComponents[2]
			dataDir      = pathComponents[3]
			dataName     = strings.TrimSuffix(pathComponents[4], ".json")
			observation  *Observation
			observations []*Observation
		)

		switch dataDir {
		case gather.WorkflowRunsDataDir:
			var workflowRunID int64
			workflowRunID, err = strconv.ParseInt(dataName, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse workflow run ID: %w", err)
			}
			observation, err = WorkflowRun(log, client, owner, repo, workflowRunID)
			observations = append(observations, observation)
			if err != nil {
				return fmt.Errorf("failed to generate workflow run observation: %w", err)
			}
			jobRuns, err := JobRuns(log, client, owner, repo, workflowRunID)
			if err != nil {
				return fmt.Errorf("failed to generate job runs: %w", err)
			}
			observations = append(observations, jobRuns...)
		case gather.PullRequestsDataDir:
			var pullRequestNumber int64
			pullRequestNumber, err = strconv.ParseInt(dataName, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse pull request number: %w", err)
			}
			observation, err = PullRequest(log, client, owner, repo, int(pullRequestNumber))
			observations = append(observations, observation)
		case gather.CommitsDataDir:
			var commitSHA string
			commitSHA = dataName
			observation, err = Commit(log, client, owner, repo, commitSHA)
			observations = append(observations, observation)
		}

		if err != nil {
			return fmt.Errorf("failed to generate observe data: %w", err)
		}

		repoKey := owner + "/" + repo
		if _, ok := bpCache[repoKey]; !ok {
			bp, bpErr := gather.BranchProtection(log, client, owner, repo)
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
				_, err = obs.Render(log, outputType)
				if err != nil {
					return err
				}
			}
		}

		for _, obs := range observations {
			if obs == nil {
				continue
			}
			key := categoryKey{obs.Owner, obs.Repo, obs.DataType + "s"}
			collected[key] = append(collected[key], IndexItem{
				Name:  obs.Name,
				Path:  obs.ID + ".html",
				State: obs.State,
				Actor: obs.Actor,
			})
		}

		return nil
	})
	if err != nil {
		return err
	}

	return generateIndexPages(collected)
}

func generateIndexPages(collected map[categoryKey][]IndexItem) error {
	homeBreadcrumb := Breadcrumb{Name: "Home", Path: "/"}

	repos := make(map[string]map[string]int)
	for key, items := range collected {
		repoPath := key.owner + "/" + key.repo
		if repos[repoPath] == nil {
			repos[repoPath] = make(map[string]int)
		}
		repos[repoPath][key.category] += len(items)
	}

	// Root index: list all owner/repo combos
	rootDirs := make([]IndexDir, 0, len(repos))
	for repoPath, categories := range repos {
		total := 0
		for _, count := range categories {
			total += count
		}
		rootDirs = append(rootDirs, IndexDir{
			Name:  repoPath,
			Path:  "/" + repoPath + "/",
			Count: total,
		})
	}
	sort.Slice(rootDirs, func(i, j int) bool { return rootDirs[i].Name < rootDirs[j].Name })
	err := renderIndex(filepath.Join(htmlOutputDir, "index.html"), IndexPage{
		Title:       "Octometrics",
		Breadcrumbs: []Breadcrumb{homeBreadcrumb},
		Dirs:        rootDirs,
	})
	if err != nil {
		return fmt.Errorf("failed to render root index: %w", err)
	}

	repoCategoryDisplay := map[string]string{
		"workflow_runs": "Workflow Runs",
		"pull_requests": "Pull Requests",
		"surveys":       "Surveys",
	}

	// Repo indexes: list primary categories for each owner/repo
	for repoPath, categories := range repos {
		parts := strings.SplitN(repoPath, "/", 2)
		owner, repo := parts[0], parts[1]

		catDirs := make([]IndexDir, 0, len(repoCategoryDisplay))
		for cat, displayName := range repoCategoryDisplay {
			count := categories[cat]
			if count == 0 {
				continue
			}
			catDirs = append(catDirs, IndexDir{
				Name:  displayName,
				Path:  "/" + repoPath + "/" + cat + "/",
				Count: count,
			})
		}
		sort.Slice(catDirs, func(i, j int) bool { return catDirs[i].Name < catDirs[j].Name })

		err := renderIndex(filepath.Join(htmlOutputDir, owner, repo, "index.html"), IndexPage{
			Title: repoPath,
			Breadcrumbs: []Breadcrumb{
				homeBreadcrumb,
				{Name: repoPath, Path: "/" + repoPath + "/"},
			},
			Dirs: catDirs,
		})
		if err != nil {
			return fmt.Errorf("failed to render repo index for %s: %w", repoPath, err)
		}
	}

	// Category indexes: list observations for each owner/repo/category
	for key, items := range collected {
		repoPath := key.owner + "/" + key.repo
		catPath := repoPath + "/" + key.category

		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

		err := renderIndex(filepath.Join(htmlOutputDir, key.owner, key.repo, key.category, "index.html"), IndexPage{
			Title: key.category,
			Breadcrumbs: []Breadcrumb{
				homeBreadcrumb,
				{Name: repoPath, Path: "/" + repoPath + "/"},
				{Name: key.category, Path: "/" + catPath + "/"},
			},
			Items: items,
		})
		if err != nil {
			return fmt.Errorf("failed to render category index for %s: %w", catPath, err)
		}
	}

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
