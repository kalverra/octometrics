package gather

import (
	"io"
	"net/http"
	"testing"

	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

const (
	testDataDir = "testdata"
)

var (
	testGatherOwner = "kalverra"
	testGatherRepo  = "octometrics"
)

func TestNewGitHubClient(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)

	var (
		mockEndpoint = mock.EndpointPattern{
			Method:  "GET",
			Pattern: "/mock-endpoint",
		}
		githubToken  = "mock-token"
		writeBodyErr error
	)

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mockEndpoint,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+githubToken {
					w.WriteHeader(http.StatusUnauthorized)
					_, writeBodyErr = w.Write([]byte(`{"message": "authorization token not present"}`))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, writeBodyErr = w.Write([]byte(`{"message": "mock response"}`))
			}),
		),
	)

	client, err := NewGitHubClient(log, githubToken, mockedHTTPClient.Transport)
	require.NoError(t, err, "error creating GitHub client")
	require.NotNil(t, client, "client should not be nil")

	rawRestClient := client.Rest.Client()
	require.NotNil(t, rawRestClient, "rawRestClient should not be nil")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, mockEndpoint.Pattern, nil)
	require.NoError(t, err, "error creating mock request")
	resp, err := rawRestClient.Do(req)
	require.NoError(t, err, "error getting mock endpoint")
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "error reading mock endpoint response")
	require.Equal(t, http.StatusOK, resp.StatusCode, "status code should be 200, got body: %s", string(bodyBytes))

	require.NoError(t, writeBodyErr, "error writing mock response")
}
