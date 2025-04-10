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
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

//go:embed templates/*.html templates/*.md
var templateFS embed.FS

const (
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
	}).ParseFS(templateFS, "templates/*.html")
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

	// Data used to show job, workflow, and commit runs
	TimelineData   *timelineData
	MonitoringData *monitoringData

	// Data used to render a Pull Request with multiple commits
	CommitData []*gather.CommitData
}

func (o *Observation) Render(log zerolog.Logger, outputType string) (observationFile string, err error) {
	var baseDir string
	switch outputType {
	case "html":
		baseDir = htmlOutputDir
	case "md":
		baseDir = markdownOutputDir
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
		if err := o.TimelineData.process(); err != nil {
			return "", fmt.Errorf("failed to process timeline data: %w", err)
		}
	}

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
func Interactive(log zerolog.Logger, client *github.Client) error {
	startTime := time.Now()
	err := All(log, client, []string{"html"})
	if err != nil {
		return fmt.Errorf("failed to generate all HTML observe data: %w", err)
	}
	var (
		url = "http://localhost:8080"
		dir = http.Dir(htmlOutputDir)
		fs  = http.FileServer(dir)
	)
	http.Handle("/", fs)

	log.Info().
		Str("url", url).
		Str("built_observations_dur", time.Since(startTime).String()).
		Str("dir", htmlOutputDir).
		Msg("Observing data...")

	go func() {
		interruptChan := make(chan os.Signal, 1)
		signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

		err := openBrowser(url)
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

func All(log zerolog.Logger, client *github.Client, outputTypes []string) error {
	return generateAllObserveData(log, client, outputTypes)
}

func generateAllObserveData(log zerolog.Logger, client *github.Client, outputTypes []string) error {
	return filepath.WalkDir(gather.DataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".json" {
			return fmt.Errorf("file %s is not a JSON file", path)
		}

		// Get owner, repo, and data name from the file path
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

		for _, outputType := range outputTypes {
			for _, observation := range observations {
				_, err = observation.Render(log, outputType)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}
