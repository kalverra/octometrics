package observe

import (
	"fmt"
	"path/filepath"

	"github.com/google/go-github/v70/github"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

func PullRequest(
	log zerolog.Logger,
	client *github.Client,
	owner, repo string,
	pullRequestNumber int,
	opts ...Option,
) (*Observation, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	prData, err := gather.PullRequest(log, client, owner, repo, pullRequestNumber, options.gatherOptions...)
	if err != nil {
		return nil, err
	}

	observation := &Observation{
		ID:         fmt.Sprint(pullRequestNumber),
		Name:       fmt.Sprintf("Pull Request #%d", pullRequestNumber),
		GitHubLink: prData.GetHTMLURL(),
		Owner:      owner,
		Repo:       repo,
		State:      prData.GetState(),
		Actor:      prData.GetUser().GetLogin(),
		CommitData: prData.GetCommitData(),
		DataType:   "pull_request",
	}
	return observation, nil
}

func commitRunLink(owner, repo, sha string) string {
	return filepath.Join("/", owner, repo, gather.CommitsDataDir, sha)
}
