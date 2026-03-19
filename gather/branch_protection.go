package gather

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"
)

// BranchProtectionResult contains the required status checks for a repository's default branch.
type BranchProtectionResult struct {
	RequiredChecks   []string
	PermissionDenied bool
}

// BranchProtection fetches the required status checks for the default branch of the given repository.
// Returns PermissionDenied=true (not an error) when the token lacks admin read access.
// Returns an empty RequiredChecks slice (not an error) when the branch has no protection rules.
func BranchProtection(log zerolog.Logger, client *GitHubClient, owner, repo string) (*BranchProtectionResult, error) {
	if client == nil {
		return &BranchProtectionResult{PermissionDenied: true}, nil
	}

	ctx, cancel := ghCtx()
	defer cancel()

	repoData, _, err := client.Rest.Repositories.Get(ctx, owner, repo)
	if err != nil {
		log.Warn().Err(err).
			Str("owner", owner).Str("repo", repo).
			Msg("Failed to get repository info for branch protection; skipping")
		return &BranchProtectionResult{}, nil
	}

	defaultBranch := repoData.GetDefaultBranch()
	if defaultBranch == "" {
		return &BranchProtectionResult{}, nil
	}

	ctx2, cancel2 := ghCtx()
	defer cancel2()

	checks, _, err := client.Rest.Repositories.GetRequiredStatusChecks(ctx2, owner, repo, defaultBranch)
	if err != nil {
		if errors.Is(err, github.ErrBranchNotProtected) {
			return &BranchProtectionResult{}, nil
		}

		if errResp, ok := errors.AsType[*github.ErrorResponse](err); ok {
			switch errResp.Response.StatusCode {
			case http.StatusForbidden:
				log.Warn().
					Str("owner", owner).Str("repo", repo).Str("branch", defaultBranch).
					Msg("Insufficient permissions to read branch protection required status checks")
				return &BranchProtectionResult{PermissionDenied: true}, nil
			case http.StatusNotFound:
				return &BranchProtectionResult{}, nil
			}
		}

		return nil, fmt.Errorf(
			"failed to get required status checks for %s/%s branch %s: %w",
			owner, repo, defaultBranch, err,
		)
	}

	result := &BranchProtectionResult{}
	seen := make(map[string]bool)

	if checks.Checks != nil {
		for _, check := range *checks.Checks {
			if !seen[check.Context] {
				result.RequiredChecks = append(result.RequiredChecks, check.Context)
				seen[check.Context] = true
			}
		}
	}
	if checks.Contexts != nil {
		for _, c := range *checks.Contexts {
			if !seen[c] {
				result.RequiredChecks = append(result.RequiredChecks, c)
				seen[c] = true
			}
		}
	}

	log.Debug().
		Str("owner", owner).Str("repo", repo).Str("branch", defaultBranch).
		Strs("required_checks", result.RequiredChecks).
		Msg("Fetched branch protection required status checks")

	return result, nil
}
