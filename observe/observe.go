package observe

import (
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
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/gather"
	"github.com/rs/zerolog"
)

const (
	OutputDir         = "observe_output"
	htmlOutputDir     = "observe_output/html"
	markdownOutputDir = "observe_output/md"
	templatesDir      = "observe/templates"
)

// Option manipulates how the observe command works
type Option func(*options)

// options contains the options for the observe command
type options struct {
	outputDir     string
	outputTypes   []string
	gatherOptions []gather.Option
}

func defaultOptions() *options {
	return &options{
		gatherOptions: []gather.Option{},
		outputDir:     OutputDir,
		outputTypes:   []string{"html", "md"},
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

// WithOutputTypes sets the output types for the observe command.
func WithOutputTypes(outputTypes []string) Option {
	return func(o *options) {
		o.outputTypes = outputTypes
	}
}

// All generates all downloaded data in HTML and serves it on a local server.
func All(log zerolog.Logger, client *github.Client) error {
	startTime := time.Now()
	err := generateAllHTMLObserveData(log, client)
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

	cmdArgs := append(args, url)
	return exec.Command(cmd, cmdArgs...).Run()
}

func generateAllHTMLObserveData(log zerolog.Logger, client *github.Client) error {
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
		owner := pathComponents[1]
		repo := pathComponents[2]
		dataDir := pathComponents[3]
		dataName := strings.TrimSuffix(pathComponents[4], ".json")

		outputOpt := WithOutputTypes([]string{"html"})
		switch dataDir {
		case gather.WorkflowRunsDataDir:
			var workflowRunID int64
			workflowRunID, err = strconv.ParseInt(dataName, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse workflow run ID: %w", err)
			}
			err = WorkflowRun(log, client, owner, repo, workflowRunID, outputOpt)
		case gather.PullRequestsDataDir:
			var pullRequestNumber int64
			pullRequestNumber, err = strconv.ParseInt(dataName, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse pull request number: %w", err)
			}
			err = PullRequest(log, client, owner, repo, int(pullRequestNumber), outputOpt)
		case gather.CommitsDataDir:
			var commitSHA string
			commitSHA = dataName
			err = Commit(log, client, owner, repo, commitSHA, outputOpt)
		}

		if err != nil {
			return fmt.Errorf("failed to generate HTML observe data: %w", err)
		}

		return nil
	})
}
