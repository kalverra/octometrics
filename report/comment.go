package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/rs/zerolog"
)

const commentMarkerPrefix = "<!-- octometrics-report:"

// commentMarker returns a unique HTML comment used to identify an existing octometrics report comment.
func commentMarker(jobName string) string {
	return fmt.Sprintf("%s %s -->", commentMarkerPrefix, jobName)
}

// postComment posts or updates the report as a PR comment. If the workflow is not
// associated with a PR, it falls back to a commit comment on the HEAD SHA.
func postComment(log zerolog.Logger, gha *ghaContext, markdown string) error {
	if gha.Token == "" {
		return fmt.Errorf("GITHUB_TOKEN not set, cannot post comment")
	}

	marker := commentMarker(gha.JobName)
	body := marker + "\n\n" + markdown

	if prNumber, ok := gha.pullRequestNumber(); ok {
		return upsertPRComment(log, gha, prNumber, marker, body)
	}

	if gha.SHA != "" {
		return createCommitComment(log, gha, body)
	}

	return fmt.Errorf("no PR number or SHA available for posting a comment")
}

// upsertPRComment creates or updates an octometrics report comment on a PR.
func upsertPRComment(log zerolog.Logger, gha *ghaContext, prNumber int, marker, body string) error {
	client := gha.newGitHubClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingID, err := findExistingComment(ctx, client, gha.Owner, gha.Repo, prNumber, marker)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to search for existing comment, will create a new one")
	}

	if existingID != 0 {
		_, _, err = client.Issues.EditComment(ctx, gha.Owner, gha.Repo, existingID, &github.IssueComment{
			Body: new(body),
		})
		if err != nil {
			return fmt.Errorf("failed to update PR comment: %w", err)
		}
		log.Info().Int("pr", prNumber).Int64("comment_id", existingID).Msg("Updated existing PR comment")
		return nil
	}

	_, _, err = client.Issues.CreateComment(ctx, gha.Owner, gha.Repo, prNumber, &github.IssueComment{
		Body: new(body),
	})
	if err != nil {
		return fmt.Errorf("failed to create PR comment: %w", err)
	}
	log.Info().Int("pr", prNumber).Msg("Created PR comment")
	return nil
}

// findExistingComment searches for a comment containing the marker string on a PR.
func findExistingComment(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	prNumber int,
	marker string,
) (int64, error) {
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for comment, err := range client.Issues.ListCommentsIter(ctx, owner, repo, prNumber, opts) {
		if err != nil {
			return 0, fmt.Errorf("failed to list PR comments: %w", err)
		}

		if strings.Contains(comment.GetBody(), marker) {
			return comment.GetID(), nil
		}
	}

	return 0, nil
}

// createCommitComment posts the report as a commit comment on the HEAD SHA.
func createCommitComment(log zerolog.Logger, gha *ghaContext, body string) error {
	client := gha.newGitHubClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := client.Repositories.CreateComment(ctx, gha.Owner, gha.Repo, gha.SHA, &github.RepositoryComment{
		Body: new(body),
	})
	if err != nil {
		return fmt.Errorf("failed to create commit comment: %w", err)
	}
	log.Info().Str("sha", gha.SHA).Msg("Created commit comment")
	return nil
}
