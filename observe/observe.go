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
	"github.com/kalverra/workflow-metrics/gather"
	"github.com/rs/zerolog/log"
)

const (
	OutputDir         = "observe_output"
	htmlOutputDir     = "observe_output/html"
	markdownOutputDir = "observe_output/md"
	templatesDir      = "observe/templates"
)

func All(client *github.Client) error {
	startTime := time.Now()
	err := generateAllHTMLObserveData(client)
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

func generateAllHTMLObserveData(client *github.Client) error {
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

		switch dataDir {
		case gather.WorkflowRunsDataDir:
			var workflowRunID int64
			workflowRunID, err = strconv.ParseInt(dataName, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse workflow run ID: %w", err)
			}
			err = WorkflowRun(client, owner, repo, workflowRunID, []string{"html"})
		case gather.PullRequestsDataDir:
			var pullRequestNumber int64
			pullRequestNumber, err = strconv.ParseInt(dataName, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse pull request number: %w", err)
			}
			err = PullRequest(client, owner, repo, int(pullRequestNumber), []string{"html"})
		case gather.CommitsDataDir:
			var commitSHA string
			commitSHA = dataName
			err = Commit(client, owner, repo, commitSHA, []string{"html"})
		}

		if err != nil {
			return fmt.Errorf("failed to generate HTML observe data: %w", err)
		}

		return nil
	})
}
