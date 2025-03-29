package observe

import (
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/rs/zerolog/log"
)

const (
	outputDir         = "observe_output"
	htmlOutputDir     = "observe_output/html"
	markdownOutputDir = "observe_output/md"
	templatesDir      = "observe/templates"
)

func Serve(openFileLink string) error {
	dir := http.Dir(htmlOutputDir)
	fs := http.FileServer(dir)
	http.Handle("/", fs)
	url := "http://localhost:8080" + openFileLink
	log.Info().Str("url", url).Str("dir", htmlOutputDir).Msg("Serving files")

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
