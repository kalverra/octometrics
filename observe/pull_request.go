package observe

import (
	"bytes"
	"fmt"
	htmlTemplate "html/template"
	"os"
	"path/filepath"
	"text/template"

	"github.com/google/go-github/v70/github"
	"github.com/kalverra/octometrics/gather"
)

func PullRequest(client *github.Client, owner, repo string, pullRequestNumber int, outputTypes []string) error {
	prData, err := gather.PullRequest(client, owner, repo, pullRequestNumber, false)
	if err != nil {
		return err
	}

	// TODO: Make markdown too
	tmpl, err := htmlTemplate.New(fmt.Sprintf("pull_request_%s", "html")).Funcs(template.FuncMap{
		"commitRunLink": commitRunLink,
	}).ParseFiles(
		filepath.Join(templatesDir, fmt.Sprintf("pull_request.%s", "html")),
	)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var rendered bytes.Buffer
	err = tmpl.Execute(&rendered, prData)
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	outputFile := filepath.Join(
		htmlOutputDir,
		owner, repo,
		gather.PullRequestsDataDir,
		fmt.Sprintf("%d.html", pullRequestNumber),
	)
	err = os.MkdirAll(filepath.Dir(outputFile), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	err = os.WriteFile(outputFile, rendered.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}
