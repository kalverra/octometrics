package gather

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"
)

// RepoSummary represents a lightweight repository listing item.
type RepoSummary struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

// PRSummary represents a lightweight pull request listing item.
type PRSummary struct {
	ID         int64     `json:"id"`
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	Actor      string    `json:"actor"`
	CreatedAt  time.Time `json:"created_at"`
	HTMLURL    string    `json:"html_url"`
	Owner      string    `json:"owner"`
	Repo       string    `json:"repo"`
	Downloaded bool      `json:"downloaded"`
}

// WorkflowSummary represents a lightweight workflow listing item.
type WorkflowSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

// RunSummary represents a lightweight workflow run listing item.
type RunSummary struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	WorkflowID int64     `json:"workflow_id"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Event      string    `json:"event"`
	Actor      string    `json:"actor"`
	HeadBranch string    `json:"head_branch"`
	HeadSHA    string    `json:"head_sha"`
	CreatedAt  time.Time `json:"created_at"`
	HTMLURL    string    `json:"html_url"`
	Downloaded bool      `json:"downloaded"`
}

// CommitSummary represents a lightweight commit listing item.
type CommitSummary struct {
	SHA          string    `json:"sha"`
	Message      string    `json:"message"`
	Author       string    `json:"author"`
	CommittedAt  time.Time `json:"committed_at"`
	HTMLURL      string    `json:"html_url"`
	Branch       string    `json:"branch,omitempty"`
	Parents      []string  `json:"parents,omitempty"`
	IsMergeQueue bool      `json:"is_merge_queue,omitempty"`
	Downloaded   bool      `json:"downloaded"`
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

var (
	browseTTL   = 60 * time.Second
	browseCache sync.Map
)

func getCached(key string) (any, bool) {
	if val, ok := browseCache.Load(key); ok {
		entry := val.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.value, true
		}
		browseCache.Delete(key)
	}
	return nil, false
}

func setCached(key string, val any) {
	browseCache.Store(key, cacheEntry{
		value:     val,
		expiresAt: time.Now().Add(browseTTL),
	})
}

// SearchRepos searches GitHub repositories.
func SearchRepos(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	query string,
	limit int,
) ([]RepoSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("SearchRepos:%s:%d", query, limit)
	if val, ok := getCached(key); ok {
		log.Trace().Str("query", query).Msg("browse cache hit for SearchRepos")
		return val.([]RepoSummary), nil
	}

	log.Trace().Str("query", query).Msg("searching repositories via GitHub API")
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: limit},
	}
	res, _, err := client.Rest.Search.Repositories(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("search repos: %w", err)
	}

	var results []RepoSummary
	for _, r := range res.Repositories {
		owner := ""
		if r.GetOwner() != nil {
			owner = r.GetOwner().GetLogin()
		}
		results = append(results, RepoSummary{
			Owner:         owner,
			Name:          r.GetName(),
			FullName:      r.GetFullName(),
			Description:   r.GetDescription(),
			DefaultBranch: r.GetDefaultBranch(),
			HTMLURL:       r.GetHTMLURL(),
		})
	}

	setCached(key, results)
	return results, nil
}

// SearchPullRequests searches GitHub pull requests using the search API.
func SearchPullRequests(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	query string,
	limit int,
) ([]PRSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("SearchPRs:%s:%d", query, limit)
	if val, ok := getCached(key); ok {
		log.Trace().Str("query", query).Msg("browse cache hit for SearchPullRequests")
		return val.([]PRSummary), nil
	}

	q := query
	if !strings.Contains(q, "is:pr") {
		q += " is:pr"
	}

	log.Trace().Str("query", q).Msg("searching pull requests via GitHub API")
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: limit},
	}
	res, _, err := client.Rest.Search.Issues(ctx, q, opts)
	if err != nil {
		return nil, fmt.Errorf("search prs: %w", err)
	}

	var results []PRSummary
	for _, issue := range res.Issues {
		actor := ""
		if issue.GetUser() != nil {
			actor = issue.GetUser().GetLogin()
		}
		created := time.Time{}
		if issue.CreatedAt != nil {
			created = issue.CreatedAt.Time
		}

		owner, repo := parseRepoURL(issue.GetRepositoryURL(), issue.GetHTMLURL())

		results = append(results, PRSummary{
			ID:        issue.GetID(),
			Number:    issue.GetNumber(),
			Title:     issue.GetTitle(),
			State:     issue.GetState(),
			Actor:     actor,
			CreatedAt: created,
			HTMLURL:   issue.GetHTMLURL(),
			Owner:     owner,
			Repo:      repo,
		})
	}

	setCached(key, results)
	return results, nil
}

func parseRepoURL(repoURL, htmlURL string) (string, string) {
	if repoURL != "" {
		parts := strings.Split(strings.TrimSuffix(repoURL, "/"), "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2], parts[len(parts)-1]
		}
	}
	if htmlURL != "" {
		parts := strings.Split(htmlURL, "/")
		if len(parts) >= 5 {
			return parts[3], parts[4]
		}
	}
	return "", ""
}

// RepoInfo fetches repository information.
func RepoInfo(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
) (*RepoSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("RepoInfo:%s:%s", owner, repo)
	if val, ok := getCached(key); ok {
		log.Trace().Str("owner", owner).Str("repo", repo).Msg("browse cache hit for RepoInfo")
		res := val.(RepoSummary)
		return &res, nil
	}

	log.Trace().Str("owner", owner).Str("repo", repo).Msg("fetching repo info via GitHub API")
	r, _, err := client.Rest.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo info: %w", err)
	}

	oName := owner
	if r.GetOwner() != nil {
		oName = r.GetOwner().GetLogin()
	}
	res := RepoSummary{
		Owner:         oName,
		Name:          r.GetName(),
		FullName:      r.GetFullName(),
		Description:   r.GetDescription(),
		DefaultBranch: r.GetDefaultBranch(),
		HTMLURL:       r.GetHTMLURL(),
	}

	setCached(key, res)
	return &res, nil
}

// ListWorkflows lists repository workflows.
func ListWorkflows(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
) ([]WorkflowSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("ListWorkflows:%s:%s", owner, repo)
	if val, ok := getCached(key); ok {
		log.Trace().Str("owner", owner).Str("repo", repo).Msg("browse cache hit for ListWorkflows")
		return val.([]WorkflowSummary), nil
	}

	log.Trace().Str("owner", owner).Str("repo", repo).Msg("listing workflows via GitHub API")
	res, _, err := client.Rest.Actions.ListWorkflows(ctx, owner, repo, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	var results []WorkflowSummary
	for _, wf := range res.Workflows {
		results = append(results, WorkflowSummary{
			ID:      wf.GetID(),
			Name:    wf.GetName(),
			Path:    wf.GetPath(),
			State:   wf.GetState(),
			HTMLURL: wf.GetHTMLURL(),
		})
	}

	setCached(key, results)
	return results, nil
}

// ListRuns lists workflow runs for a repository or a specific workflow.
func ListRuns(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	workflowID int64,
	limit int,
) ([]RunSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("ListRuns:%s:%s:%d:%d", owner, repo, workflowID, limit)
	if val, ok := getCached(key); ok {
		log.Trace().
			Str("owner", owner).
			Str("repo", repo).
			Int64("workflowID", workflowID).
			Msg("browse cache hit for ListRuns")
		return val.([]RunSummary), nil
	}

	log.Trace().
		Str("owner", owner).
		Str("repo", repo).
		Int64("workflowID", workflowID).
		Msg("listing workflow runs via GitHub API")
	opts := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{PerPage: limit},
	}

	var runs []*github.WorkflowRun
	if workflowID == 0 {
		res, _, err := client.Rest.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list repo workflow runs: %w", err)
		}
		runs = res.WorkflowRuns
	} else {
		res, _, err := client.Rest.Actions.ListWorkflowRunsByID(ctx, owner, repo, workflowID, opts)
		if err != nil {
			return nil, fmt.Errorf("list workflow runs by id: %w", err)
		}
		runs = res.WorkflowRuns
	}

	var results []RunSummary
	for _, r := range runs {
		actor := ""
		if r.GetActor() != nil {
			actor = r.GetActor().GetLogin()
		} else if r.GetTriggeringActor() != nil {
			actor = r.GetTriggeringActor().GetLogin()
		}

		created := time.Time{}
		if r.CreatedAt != nil {
			created = r.CreatedAt.Time
		} else if r.RunStartedAt != nil {
			created = r.RunStartedAt.Time
		}

		results = append(results, RunSummary{
			ID:         r.GetID(),
			Name:       r.GetName(),
			WorkflowID: r.GetWorkflowID(),
			Status:     r.GetStatus(),
			Conclusion: r.GetConclusion(),
			Event:      r.GetEvent(),
			Actor:      actor,
			HeadBranch: r.GetHeadBranch(),
			HeadSHA:    r.GetHeadSHA(),
			CreatedAt:  created,
			HTMLURL:    r.GetHTMLURL(),
		})
	}

	setCached(key, results)
	return results, nil
}

// ListCommits lists repository commits.
func ListCommits(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	limit int,
) ([]CommitSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("ListCommits:%s:%s:%d", owner, repo, limit)
	if val, ok := getCached(key); ok {
		log.Trace().Str("owner", owner).Str("repo", repo).Msg("browse cache hit for ListCommits")
		return val.([]CommitSummary), nil
	}

	log.Trace().Str("owner", owner).Str("repo", repo).Msg("listing commits via GitHub API")
	opts := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: limit},
	}
	res, _, err := client.Rest.Repositories.ListCommits(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}

	var results []CommitSummary
	for _, c := range res {
		author := ""
		if c.GetAuthor() != nil {
			author = c.GetAuthor().GetLogin()
		} else if c.GetCommit() != nil && c.GetCommit().GetCommitter() != nil {
			author = c.GetCommit().GetCommitter().GetName()
		}

		committedAt := time.Time{}
		if c.GetCommit() != nil {
			if c.GetCommit().GetCommitter() != nil && c.GetCommit().GetCommitter().Date != nil {
				committedAt = c.GetCommit().GetCommitter().GetDate().Time
			} else if c.GetCommit().GetAuthor() != nil && c.GetCommit().GetAuthor().Date != nil {
				committedAt = c.GetCommit().GetAuthor().GetDate().Time
			}
		}

		msg := ""
		if c.GetCommit() != nil {
			msg = c.GetCommit().GetMessage()
		}

		var parentSHAs []string
		for _, p := range c.Parents {
			if p.GetSHA() != "" {
				parentSHAs = append(parentSHAs, p.GetSHA())
			}
		}

		isMQ := strings.Contains(msg, "gh-readonly-queue") ||
			strings.Contains(strings.ToLower(msg), "merge queue") ||
			(len(parentSHAs) > 1 && strings.Contains(strings.ToLower(author), "merge queue"))

		branch := ""
		if isMQ {
			branch = "merge-queue"
		}

		results = append(results, CommitSummary{
			SHA:          c.GetSHA(),
			Message:      msg,
			Author:       author,
			CommittedAt:  committedAt,
			HTMLURL:      c.GetHTMLURL(),
			Branch:       branch,
			Parents:      parentSHAs,
			IsMergeQueue: isMQ,
		})
	}

	setCached(key, results)
	return results, nil
}

// ListPullRequests lists repository pull requests.
func ListPullRequests(
	ctx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	limit int,
) ([]PRSummary, error) {
	if client == nil || client.Rest == nil {
		return nil, nil
	}
	key := fmt.Sprintf("ListPRs:%s:%s:%d", owner, repo, limit)
	if val, ok := getCached(key); ok {
		log.Trace().Str("owner", owner).Str("repo", repo).Msg("browse cache hit for ListPullRequests")
		return val.([]PRSummary), nil
	}

	log.Trace().Str("owner", owner).Str("repo", repo).Msg("listing PRs via GitHub API")
	opts := &github.PullRequestListOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: limit},
	}
	res, _, err := client.Rest.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("list prs: %w", err)
	}

	var results []PRSummary
	for _, pr := range res {
		actor := ""
		if pr.GetUser() != nil {
			actor = pr.GetUser().GetLogin()
		}
		created := time.Time{}
		if pr.CreatedAt != nil {
			created = pr.CreatedAt.Time
		}

		results = append(results, PRSummary{
			ID:        pr.GetID(),
			Number:    pr.GetNumber(),
			Title:     pr.GetTitle(),
			State:     pr.GetState(),
			Actor:     actor,
			CreatedAt: created,
			HTMLURL:   pr.GetHTMLURL(),
			Owner:     owner,
			Repo:      repo,
		})
	}

	setCached(key, results)
	return results, nil
}
