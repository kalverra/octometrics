package gather

import (
	"context"
	"errors"
	"time"

	"github.com/google/go-github/v70/github"
)

const (
	timeoutDur = 10 * time.Second

	dataDir = "data"
)

var (
	ghCtx            = context.WithValue(context.Background(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)
	errGitHubTimeout = errors.New("github API timeout")
)
