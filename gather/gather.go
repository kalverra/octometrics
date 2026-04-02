package gather

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
)

// GitHub API constants for authentication, timeouts, and data storage.
const (
	//nolint:gosec // This is a mock token for testing purposes
	MockGitHubToken = "mock_github_token"
	timeoutDur      = 30 * time.Second
	DataDir         = "data"
)

var (
	errGitHubTimeout = errors.New("github API timeout")
)

// ghCtx returns a standard context to use for GitHub API calls.
// The per-request timeout is applied at the transport layer (loggingTransport.RoundTrip)
// so that paginated iterators get a fresh timeout for each page.
func ghCtx() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	return context.WithValue(
		ctx,
		github.BypassRateLimitCheck,
		true,
	), cancel
}

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
	gatherCost      bool
}

func defaultOptions() *options {
	return &options{
		ForceUpdate: false,
		DataDir:     DataDir,
		gatherCost:  false,
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

// WithCost enables gathering cost data for workflow runs
func WithCost() Option {
	return func(o *options) {
		o.gatherCost = true
	}
}

// GitHubClient wraps GitHub REST and GraphQL clients for API access.
type GitHubClient struct {
	Rest    *github.Client
	GraphQL *githubv4.Client
}

// NewGitHubClient creates a new GitHub API and GraphQL client with the provided token and logger.
func NewGitHubClient(
	logger zerolog.Logger,
	githubToken string,
	optionalNext http.RoundTripper,
) (*GitHubClient, error) {
	var (
		next   http.RoundTripper
		client = &GitHubClient{}
	)

	if optionalNext != nil {
		next = optionalNext
	}

	rateLimiter := github_ratelimit.NewClient(gitHubClientRoundTripper("REST", logger, next),
		github_primary_ratelimit.WithLimitDetectedCallback(func(ctx *github_primary_ratelimit.CallbackContext) {
			logger.Warn().
				Str("category", string(ctx.Category)).
				Time("reset_time", *ctx.ResetTime).
				Msg("Primary rate limit hit")
		}),
		github_secondary_ratelimit.WithLimitDetectedCallback(func(ctx *github_secondary_ratelimit.CallbackContext) {
			logger.Warn().
				Time("reset_time", *ctx.ResetTime).
				Msg("Secondary rate limit hit")
		}),
	)

	client.Rest = github.NewClient(rateLimiter)
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

// Range gathers all workflow runs for a repository within a given time range.
func Range(
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	since, until time.Time,
	event string,
	opts ...Option,
) error {
	if client == nil {
		return fmt.Errorf("github client is nil")
	}

	log.Info().
		Time("since", since).
		Time("until", until).
		Str("event", event).
		Msg("Gathering workflow runs in range")

	// GitHub API expects created filter in format YYYY-MM-DD..YYYY-MM-DD
	createdFilter := fmt.Sprintf("%s..%s", since.Format("2006-01-02"), until.Format("2006-01-02"))

	if event == "all" {
		event = ""
	}

	var (
		allRuns  []*github.WorkflowRun
		listOpts = &github.ListWorkflowRunsOptions{
			Created: createdFilter,
			Event:   event,
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}
	)

	ctx, cancel := ghCtx()
	defer cancel()

	for run, err := range client.Rest.Actions.ListRepositoryWorkflowRunsIter(ctx, owner, repo, listOpts) {
		if err != nil {
			return fmt.Errorf("failed to list workflow runs: %w", err)
		}
		allRuns = append(allRuns, run)
	}

	log.Info().Int("count", len(allRuns)).Msg("Found workflow runs to gather")

	var eg errgroup.Group
	// Limit concurrency to avoid hitting rate limits too fast even with the rate limiter
	eg.SetLimit(10)

	for _, run := range allRuns {
		runID := run.GetID()
		eg.Go(func() error {
			_, _, err := WorkflowRun(log, client, owner, repo, runID, opts...)
			if err != nil {
				log.Error().Err(err).Int64("workflow_run_id", runID).Msg("Failed to gather workflow run")
				// We don't return error here to allow other runs to be gathered
				return nil
			}
			return nil
		})
	}

	return eg.Wait()
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

// cancelOnClose wraps a ReadCloser to cancel a context when the body is closed.
// This ensures per-request timeouts in RoundTrip don't cancel while go-github
// is still reading the response body.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// RoundTrip logs the request and response details.
// Each request gets its own timeout so paginated iterators don't share a single deadline.
func (lt *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeoutCause(req.Context(), timeoutDur, errGitHubTimeout)
	req = req.WithContext(ctx)

	start := time.Now()

	logger := lt.logger.With().
		Str("client_type", lt.clientType).
		Str("method", req.Method).
		Str("request_url", req.URL.String()).
		Str("user_agent", req.Header.Get("User-Agent")).
		Logger()

	resp, err := lt.transport.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		cancel()
		return resp, err
	}

	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}

	logger = logger.With().
		Int("status", resp.StatusCode).
		Str("duration", duration.String()).
		Logger()

	if resp.StatusCode != http.StatusOK {
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
