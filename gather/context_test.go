package gather

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestWorkflowRun_ContextCancellation(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	// RoundTripper that blocks until context is cancelled
	blockTransport := &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}

	client, err := NewGitHubClient(log, "mock-token", blockTransport)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	_, _, err = WorkflowRun(
		ctx,
		log,
		client,
		"owner",
		"repo",
		12345,
		CustomDataFolder(tempDir),
		ForceUpdate(),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCommit_ContextCancellation(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	blockTransport := &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}

	client, err := NewGitHubClient(log, "mock-token", blockTransport)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	_, err = Commit(
		ctx,
		log,
		client,
		"owner",
		"repo",
		"abcd123",
		CustomDataFolder(tempDir),
		ForceUpdate(),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPullRequest_ContextCancellation(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	blockTransport := &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}

	client, err := NewGitHubClient(log, "mock-token", blockTransport)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	_, err = PullRequest(
		ctx,
		log,
		client,
		"owner",
		"repo",
		42,
		CustomDataFolder(tempDir),
		ForceUpdate(),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRange_ContextCancellation(t *testing.T) {
	t.Parallel()

	log, tempDir := testhelpers.Setup(t)

	blockTransport := &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}

	client, err := NewGitHubClient(log, "mock-token", blockTransport)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	_, err = Range(
		ctx,
		log,
		client,
		"owner",
		"repo",
		time.Now().Add(-24*time.Hour),
		time.Now(),
		"",
		CustomDataFolder(tempDir),
		ForceUpdate(),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
