package githuburl_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/githuburl"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    *githuburl.Result
		wantErr string
	}{
		{
			name:  "workflow run URL",
			input: "https://github.com/smartcontractkit/chainlink/actions/runs/31503697636",
			want: &githuburl.Result{
				Owner:         "smartcontractkit",
				Repo:          "chainlink",
				WorkflowRunID: 31503697636,
			},
		},
		{
			name:  "workflow run URL with subpath and query",
			input: "https://github.com/smartcontractkit/chainlink/actions/runs/31503697636/job/123?pr=1#step:1:1",
			want: &githuburl.Result{
				Owner:         "smartcontractkit",
				Repo:          "chainlink",
				WorkflowRunID: 31503697636,
				JobID:         123,
			},
		},
		{
			name:  "workflow job URL",
			input: "https://github.com/kalverra/octometrics/actions/runs/31503697636/job/93924509957",
			want: &githuburl.Result{
				Owner:         "kalverra",
				Repo:          "octometrics",
				WorkflowRunID: 31503697636,
				JobID:         93924509957,
			},
		},
		{
			name:  "commit URL",
			input: "https://github.com/smartcontractkit/chainlink/commit/9ea808d3cb1acaa7b417d850782455c45d69e178",
			want: &githuburl.Result{
				Owner:     "smartcontractkit",
				Repo:      "chainlink",
				CommitSHA: "9ea808d3cb1acaa7b417d850782455c45d69e178",
			},
		},
		{
			name:  "pull request URL",
			input: "https://github.com/smartcontractkit/chainlink/pull/23388",
			want: &githuburl.Result{
				Owner:             "smartcontractkit",
				Repo:              "chainlink",
				PullRequestNumber: 23388,
			},
		},
		{
			name:  "pull request URL with subpath and trailing slash",
			input: "https://github.com/smartcontractkit/chainlink/pull/23388/files/",
			want: &githuburl.Result{
				Owner:             "smartcontractkit",
				Repo:              "chainlink",
				PullRequestNumber: 23388,
			},
		},
		{
			name:    "non-github host",
			input:   "https://gitlab.com/owner/repo/pull/1",
			wantErr: "unsupported GitHub URL host",
		},
		{
			name:    "invalid URL format",
			input:   "not-a-url",
			wantErr: "invalid GitHub URL",
		},
		{
			name:    "unsupported path structure",
			input:   "https://github.com/owner/repo/issues/10",
			wantErr: "unsupported GitHub URL path",
		},
		{
			name:    "invalid workflow run ID",
			input:   "https://github.com/owner/repo/actions/runs/invalid",
			wantErr: "invalid workflow run ID",
		},
		{
			name:    "invalid PR number",
			input:   "https://github.com/owner/repo/pull/invalid",
			wantErr: "invalid pull request number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := githuburl.Parse(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
