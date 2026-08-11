// Package githuburl provides parsing for GitHub web URLs into resource identifiers.
package githuburl

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Result contains parsed GitHub repository and target identifier details.
type Result struct {
	Owner             string
	Repo              string
	WorkflowRunID     int64
	CommitSHA         string
	PullRequestNumber int
}

// Parse parses a GitHub web URL for workflow run, commit, or PR into a Result struct.
func Parse(rawURL string) (*Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid GitHub URL %q: %w", rawURL, err)
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname != "github.com" && hostname != "www.github.com" {
		return nil, fmt.Errorf("unsupported GitHub URL host %q", u.Hostname())
	}

	parts := strings.FieldsFunc(u.Path, func(r rune) bool {
		return r == '/'
	})

	if len(parts) < 4 {
		return nil, fmt.Errorf("unsupported GitHub URL path %q", u.Path)
	}

	owner := parts[0]
	repo := parts[1]

	switch parts[2] {
	case "actions":
		if len(parts) < 5 || parts[3] != "runs" {
			return nil, fmt.Errorf("unsupported GitHub URL path %q", u.Path)
		}
		runID, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid workflow run ID %q: %w", parts[4], err)
		}
		return &Result{
			Owner:         owner,
			Repo:          repo,
			WorkflowRunID: runID,
		}, nil

	case "commit":
		return &Result{
			Owner:     owner,
			Repo:      repo,
			CommitSHA: parts[3],
		}, nil

	case "pull", "pulls":
		prNum, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid pull request number %q: %w", parts[3], err)
		}
		return &Result{
			Owner:             owner,
			Repo:              repo,
			PullRequestNumber: prNum,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported GitHub URL path %q", u.Path)
	}
}
