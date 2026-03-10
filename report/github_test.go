package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ref    string
		wantN  int
		wantOK bool
	}{
		{"PR ref", "refs/pull/42/merge", 42, true},
		{"PR ref high number", "refs/pull/12345/merge", 12345, true},
		{"branch ref", "refs/heads/main", 0, false},
		{"tag ref", "refs/tags/v1.0.0", 0, false},
		{"empty ref", "", 0, false},
		{"malformed PR ref", "refs/pull/", 0, false},
		{"non-numeric PR", "refs/pull/abc/merge", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gha := &ghaContext{Ref: tt.ref}
			n, ok := gha.pullRequestNumber()
			assert.Equal(t, tt.wantN, n)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestDetectGitHubActionsNotInGHA(t *testing.T) {
	t.Parallel()

	// When GITHUB_ACTIONS is not set, detection should fail.
	// In test environments, GITHUB_ACTIONS is not "true" by default.
	gha, err := detectGitHubActions()
	if gha != nil {
		t.Skip("running in GitHub Actions, skipping non-GHA test")
	}
	require.Error(t, err)
	assert.Nil(t, gha)
}
