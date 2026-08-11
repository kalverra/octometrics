package observe

import (
	"os"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestParseCommitMsg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		maxLen       int
		wantType     string
		wantScope    string
		wantTag      string
		wantSummary  string
		wantFullSum  string
		wantBody     string
		wantBreaking bool
	}{
		{
			name:         "conventional commit with scope and breaking change",
			input:        "feat(auth)!: add OAuth2 login flow\n\nDetailed body explanation here.",
			maxLen:       50,
			wantType:     "feat",
			wantScope:    "auth",
			wantSummary:  "add OAuth2 login flow",
			wantFullSum:  "add OAuth2 login flow",
			wantBody:     "Detailed body explanation here.",
			wantBreaking: true,
		},
		{
			name:        "bracketed tag commit message",
			input:       "[CRE] Add mixed-env topology + automatic non-determinism detection\n\nSecond line body text",
			maxLen:      40,
			wantTag:     "[CRE]",
			wantSummary: "Add mixed-env topology + automatic no...",
			wantFullSum: "Add mixed-env topology + automatic non-determinism detection",
			wantBody:    "Second line body text",
		},
		{
			name:        "plain commit truncated",
			input:       "This is a very long commit message summary line that should definitely be truncated because it exceeds the maximum length limit",
			maxLen:      30,
			wantSummary: "This is a very long commit ...",
			wantFullSum: "This is a very long commit message summary line that should definitely be truncated because it exceeds the maximum length limit",
		},
		{
			name:        "conventional commit simple fix",
			input:       "fix: resolve nil pointer exception",
			maxLen:      50,
			wantType:    "fix",
			wantSummary: "resolve nil pointer exception",
			wantFullSum: "resolve nil pointer exception",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseCommitMsg(tt.input, tt.maxLen)
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.wantScope, got.Scope)
			assert.Equal(t, tt.wantTag, got.Tag)
			assert.Equal(t, tt.wantSummary, got.Summary)
			assert.Equal(t, tt.wantFullSum, got.FullSummary)
			assert.Equal(t, tt.wantBody, got.Body)
			assert.Equal(t, tt.wantBreaking, got.IsBreaking)
		})
	}
}

func TestPullRequest_CostAndDurationAggregation(t *testing.T) {
	t.Parallel()

	_, _ = testhelpers.Setup(t)

	startTime1 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	endTime1 := startTime1.Add(12 * time.Minute)
	startTime2 := time.Date(2026, 8, 2, 14, 0, 0, 0, time.UTC)
	endTime2 := startTime2.Add(45 * time.Minute)

	commit1 := &gather.CommitData{
		RepositoryCommit: &github.RepositoryCommit{
			SHA: new("1111111111111111111111111111111111111111"),
			Commit: &github.Commit{
				Message: new("feat(core): first commit"),
				Author: &github.CommitAuthor{
					Date: &github.Timestamp{Time: startTime1},
				},
			},
		},
		Owner:            "owner",
		Repo:             "repo",
		StartActionsTime: startTime1,
		EndActionsTime:   endTime1,
		Cost:             150, // $0.15
		CostGathered:     true,
		CostEstimate:     true,
	}

	commit2 := &gather.CommitData{
		RepositoryCommit: &github.RepositoryCommit{
			SHA: new("2222222222222222222222222222222222222222"),
			Commit: &github.Commit{
				Message: new("[BUG] fix crash on startup\n\nExtra body details"),
				Author: &github.CommitAuthor{
					Date: &github.Timestamp{Time: startTime2},
				},
			},
		},
		Owner:            "owner",
		Repo:             "repo",
		StartActionsTime: startTime2,
		EndActionsTime:   endTime2,
		Cost:             250, // $0.25
		CostGathered:     true,
	}

	prData := &gather.PullRequestData{
		PullRequest: &github.PullRequest{
			Number:  new(100),
			State:   new("open"),
			HTMLURL: new("https://github.com/owner/repo/pull/100"),
			User:    &github.User{Login: new("octocat")},
		},
		CommitData: []*gather.CommitData{commit1, commit2},
	}

	// Verify CommitData.GetDuration()
	assert.Equal(t, 12*time.Minute, commit1.GetDuration())
	assert.Equal(t, 45*time.Minute, commit2.GetDuration())

	// Test observe.PullRequestFromData or PullRequest calculation
	observation := buildPRObservation("owner", "repo", 100, prData)

	assert.Equal(t, int64(400), observation.Cost) // 150 + 250
	assert.True(t, observation.CostGathered)
	assert.True(t, observation.CostEstimate)
}

func TestPullRequest_TemplateRendering(t *testing.T) {
	t.Parallel()

	log, _ := testhelpers.Setup(t)
	outputDir := t.TempDir()

	startTime := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	endTime := startTime.Add(5 * time.Minute)

	commit := &gather.CommitData{
		RepositoryCommit: &github.RepositoryCommit{
			SHA: new("abcdef1234567890abcdef1234567890abcdef12"),
			Commit: &github.Commit{
				Message: new("fix(ui): resolve alignment issue in table header\n\nDetailed fix explanation."),
				Author: &github.CommitAuthor{
					Date: &github.Timestamp{Time: startTime},
				},
			},
		},
		Owner:            "owner",
		Repo:             "repo",
		StartActionsTime: startTime,
		EndActionsTime:   endTime,
		Cost:             80, // $0.08
		CostGathered:     true,
	}

	prData := &gather.PullRequestData{
		PullRequest: &github.PullRequest{
			Number:  new(42),
			State:   new("closed"),
			HTMLURL: new("https://github.com/owner/repo/pull/42"),
			User:    &github.User{Login: new("alice")},
		},
		CommitData: []*gather.CommitData{commit},
	}

	obs := buildPRObservation("owner", "repo", 42, prData)
	require.NotNil(t, obs)

	// Render HTML
	htmlFile, err := obs.Render(log, "html", WithCustomOutputDir(outputDir))
	require.NoError(t, err)

	//nolint:gosec // test file read
	htmlRaw, err := os.ReadFile(htmlFile)
	require.NoError(t, err)
	htmlContent := string(htmlRaw)

	// Header cost check
	assert.Contains(t, htmlContent, "$0.08") // Cost in PR observation header
	// Check conventional commit tag rendering
	assert.Contains(t, htmlContent, "fix")
	assert.Contains(t, htmlContent, "ui")
	assert.Contains(t, htmlContent, "resolve alignment issue in table header")
	// Check duration and cost per commit
	assert.Contains(t, htmlContent, "5m 0s")
	assert.Contains(t, htmlContent, "$0.08")

	// Render MD
	mdFile, err := obs.Render(log, "md", WithCustomOutputDir(outputDir))
	require.NoError(t, err)

	//nolint:gosec // test file read
	mdRaw, err := os.ReadFile(mdFile)
	require.NoError(t, err)
	mdContent := string(mdRaw)
	assert.Contains(t, mdContent, "5m 0s")
	assert.Contains(t, mdContent, "$0.08")
}
