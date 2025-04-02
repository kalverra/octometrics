package observe

import (
	"path/filepath"
	"time"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/gather"
	"github.com/rs/zerolog/log"
)

func Commit(client *github.Client, owner, repo string, commitSHA string, outputTypes []string) error {
	commit, err := gather.Commit(client, owner, repo, commitSHA, false)
	if err != nil {
		return err
	}

	startTime := time.Now()

	workflowRuns := make([]*gather.WorkflowRunData, 0, len(commit.WorkflowRunIDs))
	for _, workflowRunID := range commit.WorkflowRunIDs {
		workflowRun, err := gather.WorkflowRun(client, owner, repo, workflowRunID, false)
		if err != nil {
			return err
		}
		workflowRuns = append(workflowRuns, workflowRun)
	}

	commitTemplateData := buildCommitGanttData(commit, workflowRuns)
	err = renderGantt(commitTemplateData, outputTypes)
	if err != nil {
		return err
	}

	log.Debug().
		Str("commit_sha", commitSHA).
		Str("duration", startTime.String()).
		Msg("Observed commit")
	return nil

}

func buildCommitGanttData(commitData *gather.CommitData, workflowRuns []*gather.WorkflowRunData) *ganttData {
	tasks := make([]ganttItem, 0, len(workflowRuns))
	commitSHA := commitData.GetSHA()
	owner := commitData.GetOwner()
	repo := commitData.GetRepo()
	for _, workflowRun := range workflowRuns {

		startedAt := workflowRun.GetRunStartedAt().Time
		duration := workflowRun.GetRunCompletedAt().Sub(startedAt)

		workflowName := workflowRun.GetName()
		// Colons in names break mermaid rendering https://github.com/mermaid-js/mermaid/issues/742
		tasks = append(tasks, ganttItem{
			Name:       workflowName,
			StartTime:  startedAt,
			Conclusion: conclusionToGanntStatus(workflowRun.GetConclusion()),
			Duration:   duration,
			Link:       workflowRunLink(owner, repo, workflowRun.GetID()) + ".html",
		})
	}

	return &ganttData{
		ID:       commitSHA,
		Owner:    owner,
		Repo:     repo,
		Name:     "Commit " + commitSHA,
		Link:     commitData.GetHTMLURL(),
		DataType: "commit",
		Cost:     commitData.GetCost(),
		Items:    tasks,
	}
}

func commitRunLink(owner, repo, sha string) string {
	return filepath.Join("/", owner, repo, gather.CommitsDataDir, sha)
}
