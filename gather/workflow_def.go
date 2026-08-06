package gather

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// WorkflowDef represents the parsed workflow definition.
type WorkflowDef struct {
	Jobs map[string]JobDef `yaml:"jobs"`
}

// JobDef represents a single job definition in the workflow.
type JobDef struct {
	Name   string `yaml:"name"`
	Uses   string `yaml:"uses"`
	Needs  Needs  `yaml:"needs"`
	RunsOn RunsOn `yaml:"runs-on"`
}

// Needs handles the YAML union type: needs can be a single string or a list.
type Needs []string

// UnmarshalYAML handles both string and []string for needs.
func (n *Needs) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*n = []string{value.Value}
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*n = list
		return nil
	}
	return fmt.Errorf("unexpected node kind for needs: %v", value.Kind)
}

// RunsOn handles the YAML union type: runs-on can be a single string or a list.
type RunsOn string

// UnmarshalYAML handles both string and []string for runs-on.
func (r *RunsOn) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*r = RunsOn(value.Value)
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		// Join list items for display
		var result strings.Builder
		for i, item := range list {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(item)
		}
		*r = RunsOn(result.String())
		return nil
	}
	return fmt.Errorf("unexpected node kind for runs-on: %v", value.Kind)
}

// ParseWorkflowDef parses a workflow YAML file into a WorkflowDef.
func ParseWorkflowDef(content []byte) (*WorkflowDef, error) {
	var def WorkflowDef
	if err := yaml.Unmarshal(content, &def); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	// Set default names for jobs without explicit names
	for id, job := range def.Jobs {
		if job.Name == "" {
			job.Name = id
			def.Jobs[id] = job
		}
	}

	return &def, nil
}

// GetJobIDByName finds a job ID by its display name or job ID.
// Returns the job ID and true if found.
func (d *WorkflowDef) GetJobIDByName(name string) (string, bool) {
	if d == nil || d.Jobs == nil {
		return "", false
	}

	// First try exact name match
	for id, job := range d.Jobs {
		if job.Name == name {
			return id, true
		}
	}

	// Fallback to job ID match
	if _, ok := d.Jobs[name]; ok {
		return name, true
	}

	return "", false
}

// workflowDefData fetches and parses the workflow YAML file at the run's HeadSHA.
// Returns nil (no error) if the file is not found (404) — the flow chart is omitted gracefully.
func workflowDefData(
	parentCtx context.Context,
	log zerolog.Logger,
	client *GitHubClient,
	owner, repo string,
	workflowRun *github.WorkflowRun,
) (*WorkflowDef, error) {
	if client == nil || workflowRun == nil {
		return nil, nil
	}

	workflowPath := workflowRun.GetPath()
	if workflowPath == "" {
		return nil, nil
	}

	headSHA := workflowRun.GetHeadSHA()
	if headSHA == "" {
		return nil, nil
	}

	ctx, cancel := ghCtx(parentCtx)
	defer cancel()

	fileContent, _, resp, err := client.Rest.Repositories.GetContents(ctx, owner, repo, workflowPath,
		&github.RepositoryContentGetOptions{Ref: headSHA},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			log.Warn().
				Str("workflow_path", workflowPath).
				Str("head_sha", headSHA).
				Msg("Workflow file not found at run SHA; skipping flow chart")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch workflow file '%s' at SHA '%s': %w", workflowPath, headSHA, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d fetching workflow file", resp.StatusCode)
	}
	if fileContent == nil || fileContent.Content == nil {
		return nil, nil
	}

	rawContent, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow file content: %w", err)
	}

	def, err := ParseWorkflowDef(rawContent)
	if err != nil {
		log.Warn().
			Str("workflow_path", workflowPath).
			Err(err).
			Msg("Failed to parse workflow YAML; skipping flow chart")
		return nil, nil
	}

	return def, nil
}
