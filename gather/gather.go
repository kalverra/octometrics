package gather

import (
	"context"
	"errors"
	"time"

	"github.com/google/go-github/v70/github"
)

const (
	timeoutDur = 10 * time.Second

	DataDir = "data"
)

var (
	ghCtx            = context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	errGitHubTimeout = errors.New("github API timeout")
)

// Option is a function that modifies GatherOptions to change how data is gathered from GitHub.
type Option func(*options)

// options contains options for gathering data,
// manipulated by GatherOption functions.
type options struct {
	ForceUpdate bool
	DataDir     string
}

func defaultOptions() *options {
	return &options{
		ForceUpdate: false,
		DataDir:     DataDir,
	}
}

// ForceUpdate foces any gather option to call the GitHub API
// even if the data is already present in the local data directory.
func ForceUpdate() Option {
	return func(o *options) {
		o.ForceUpdate = true
	}
}

// CustomDataFolder sets the data directory to a custom folder.
// This is useful for testing or if you want to use a different folder
// than the default one.
func CustomDataFolder(folder string) Option {
	return func(o *options) {
		o.DataDir = folder
	}
}
