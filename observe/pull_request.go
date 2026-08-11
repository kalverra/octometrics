package observe

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// CommitMessageFormatted contains parsed conventional commit information for display.
type CommitMessageFormatted struct {
	Raw         string
	Type        string
	Scope       string
	Tag         string
	Summary     string
	FullSummary string
	Body        string
	IsBreaking  bool
}

var (
	convCommitRegex = regexp.MustCompile(`^([a-zA-Z0-9_\-]+)(?:\(([^)]+)\))?(!+)?: (.*)$`)
	bracketTagRegex = regexp.MustCompile(`^(\[[^\]]+\])\s*(.*)$`)
)

func parseCommitMsg(msg string, maxSummaryLen int) CommitMessageFormatted {
	if maxSummaryLen <= 0 {
		maxSummaryLen = 100
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return CommitMessageFormatted{}
	}

	lines := strings.Split(msg, "\n")
	firstLine := strings.TrimSpace(lines[0])

	var body string
	if len(lines) > 1 {
		body = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}

	res := CommitMessageFormatted{
		Raw:         msg,
		FullSummary: firstLine,
		Summary:     firstLine,
		Body:        body,
	}

	if match := convCommitRegex.FindStringSubmatch(firstLine); len(match) > 0 {
		res.Type = match[1]
		res.Scope = match[2]
		res.IsBreaking = match[3] != ""
		res.FullSummary = match[4]
		res.Summary = match[4]
	} else if match := bracketTagRegex.FindStringSubmatch(firstLine); len(match) > 0 {
		res.Tag = match[1]
		res.FullSummary = match[2]
		res.Summary = match[2]
	}

	if len(res.Summary) > maxSummaryLen {
		if maxSummaryLen > 3 {
			res.Summary = res.Summary[:maxSummaryLen-3] + "..."
		} else {
			res.Summary = res.Summary[:maxSummaryLen]
		}
	}

	return res
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func buildPRObservation(owner, repo string, pullRequestNumber int, prData *gather.PullRequestData) *Observation {
	var (
		totalCost    int64
		costEstimate bool
		costGathered bool
	)
	if prData != nil {
		for _, commitData := range prData.GetCommitData() {
			totalCost += commitData.GetCost()
			if commitData.GetCostEstimate() {
				costEstimate = true
			}
			if commitData.GetCostGathered() {
				costGathered = true
			}
		}
	}

	observation := &Observation{
		ID:           fmt.Sprint(pullRequestNumber),
		Name:         fmt.Sprintf("Pull Request #%d", pullRequestNumber),
		GitHubLink:   prData.GetHTMLURL(),
		Owner:        owner,
		Repo:         repo,
		State:        prData.GetState(),
		Actor:        prData.GetUser().GetLogin(),
		Cost:         totalCost,
		CostEstimate: costEstimate,
		CostGathered: costGathered,
		CommitData:   prData.GetCommitData(),
		DataType:     "pull_request",
	}
	return observation
}

// PullRequest creates an Observation for a given pull request number.
func PullRequest(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	owner, repo string,
	pullRequestNumber int,
	opts ...Option,
) (*Observation, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	prData, err := gather.PullRequest(ctx, log, client, owner, repo, pullRequestNumber, options.gatherOptions...)
	if err != nil {
		return nil, err
	}

	return buildPRObservation(owner, repo, pullRequestNumber, prData), nil
}

func commitRunLink(owner, repo, sha string) string {
	return path.Join("/", owner, repo, gather.CommitsDataDir, sha)
}
