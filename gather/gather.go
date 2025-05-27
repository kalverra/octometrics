package gather

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const (
	//nolint:gosec // Env var for getting token
	GitHubTokenEnvVar = "GITHUB_TOKEN"
	//nolint:gosec // This is a mock token for testing purposes
	MockGitHubToken = "mock_github_token"
	timeoutDur      = 10 * time.Second
	DataDir         = "data"
)

var (
	ghCtx = context.WithValue(
		context.Background(),
		github.SleepUntilPrimaryRateLimitResetWhenRateLimited,
		true,
	)
	errGitHubTimeout = errors.New("github API timeout")
)

// Option is a function that modifies GatherOptions to change how data is gathered from GitHub.
type Option func(*options)

// options contains options for gathering data,
// manipulated by GatherOption functions.
type options struct {
	ForceUpdate bool
	DataDir     string

	// Optional data params to pass things down the stack so that e.g. a workflow can easily know what PR it belongs to
	pullRequestData *PullRequestData
	commitData      *CommitData
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

// withPullRequestData passes optional prData down the stack of data
func withPullRequestData(prData *PullRequestData) Option {
	return func(o *options) {
		o.pullRequestData = prData
	}
}

// withCommitData passes optional commitData down the stack of data
func withCommitData(commitData *CommitData) Option {
	return func(o *options) {
		o.commitData = commitData
	}
}

type GitHubClient struct {
	Rest    *github.Client
	GraphQL *githubv4.Client
}

// NewGitHubClient creates a new GitHub API and GraphQL client with the provided token and logger.
// If optionalCustomClient is provided, it will be used as the base client for both REST and GraphQL.
func NewGitHubClient(
	logger zerolog.Logger,
	githubToken string,
	optionalNext http.RoundTripper,
) (*GitHubClient, error) {
	switch {
	case githubToken != "":
		logger.Debug().Msg("Using GitHub token from flag")
	case os.Getenv(GitHubTokenEnvVar) != "":
		githubToken = os.Getenv(GitHubTokenEnvVar)
		logger.Debug().Msg("Using GitHub token from environment variable")
	default:
		logger.Warn().Msg("GitHub token not provided, will likely hit rate limits quickly")
	}

	var (
		err    error
		next   http.RoundTripper
		client = &GitHubClient{}
	)

	if optionalNext != nil {
		next = optionalNext
	}

	onRateLimitHit := func(ctx *github_ratelimit.CallbackContext) {
		l := logger.Warn()
		if ctx.Request != nil {
			l = l.Str("request_url", ctx.Request.URL.String())
		}
		if ctx.Response != nil {
			l = l.Int("status", ctx.Response.StatusCode)
		}
		if ctx.SleepUntil != nil {
			l = l.Time("sleep_until", *ctx.SleepUntil)
		}
		if ctx.TotalSleepTime != nil {
			l = l.Str("total_sleep_time", ctx.TotalSleepTime.String())
		}
		l.Msg("GitHub API rate limit hit, sleeping until limit reset")
	}

	baseClient, err := github_ratelimit.NewRateLimitWaiterClient(
		gitHubClientRoundTripper("REST", logger, next),
		github_ratelimit.WithLimitDetectedCallback(onRateLimitHit),
	)
	if err != nil {
		return nil, err
	}

	client.Rest = github.NewClient(baseClient)
	if githubToken != "" {
		client.Rest = client.Rest.WithAuthToken(githubToken)
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	graphqlClient := oauth2.NewClient(context.Background(), src)
	graphqlClient.Transport = gitHubClientRoundTripper("GraphQL", logger, graphqlClient.Transport)
	client.GraphQL = githubv4.NewClient(graphqlClient)

	return client, nil
}

// gitHubClientRoundTripper returns a RoundTripper that logs requests and responses to the GitHub API.
// You can pass a custom RoundTripper to use a different transport, or nil to use the default transport.
func gitHubClientRoundTripper(clientType string, logger zerolog.Logger, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	return &loggingTransport{
		transport:  next,
		logger:     logger,
		clientType: clientType,
	}
}

type loggingTransport struct {
	transport  http.RoundTripper
	logger     zerolog.Logger
	clientType string
}

// RoundTrip logs the request and response details.
func (lt *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	logger := lt.logger.With().
		Str("client_type", lt.clientType).
		Str("method", req.Method).
		Str("request_url", req.URL.String()).
		Str("user_agent", req.Header.Get("User-Agent")).
		Logger()

	resp, err := lt.transport.RoundTrip(req)
	duration := time.Since(start)

	logger = logger.With().
		Int("status", resp.StatusCode).
		Str("duration", duration.String()).
		Logger()

	if err != nil || resp.StatusCode != http.StatusOK {
		// Probably a rate limit error, let the rate limit library handle it
		return resp, err
	}

	// Process rate limit headers
	callsRemainingStr := resp.Header.Get("X-RateLimit-Remaining")
	if callsRemainingStr == "" {
		callsRemainingStr = "0"
	}
	callLimitStr := resp.Header.Get("X-RateLimit-Limit")
	if callLimitStr == "" {
		callLimitStr = "0"
	}
	callsUsedStr := resp.Header.Get("X-RateLimit-Used")
	if callsUsedStr == "" {
		callsUsedStr = "0"
	}
	limitResetStr := resp.Header.Get("X-RateLimit-Reset")
	if limitResetStr == "" {
		limitResetStr = "0"
	}
	callsRemaining, err := strconv.Atoi(callsRemainingStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert callsRemaining header to int: %w", err)
	}
	callLimit, err := strconv.Atoi(callLimitStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert callLimit header to int: %w", err)
	}
	callsUsed, err := strconv.Atoi(callsUsedStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert callsUsed header to int: %w", err)
	}
	limitReset, err := strconv.Atoi(limitResetStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert limitReset header to int: %w", err)
	}
	limitResetTime := time.Unix(int64(limitReset), 0)

	logger = logger.With().
		Int("calls_remaining", callsRemaining).
		Int("call_limit", callLimit).
		Int("calls_used", callsUsed).
		Time("limit_reset", limitResetTime).
		Str("response_url", resp.Request.URL.String()).
		Logger()

	mockRequest := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ") == MockGitHubToken

	if !mockRequest && callsRemaining <= 50 && callsRemaining%10 == 0 {
		logger.Warn().Msg("GitHub API request nearing rate limit")
	}

	logger.Trace().Msg("GitHub API request completed")
	return resp, nil
}
