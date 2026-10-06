// Package cost provides GitHub Actions billing analysis, runner breakdowns, and Runs-On migration savings modeling.
package cost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v89/github"
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// WorkflowSpec holds resolved runner information and rates for a workflow.
type WorkflowSpec struct {
	Repository   string  `json:"repository"`
	WorkflowPath string  `json:"workflow_path"`
	RunnerSpec   string  `json:"runner_spec"`
	RatePerMin   float64 `json:"rate_per_min"`
}

var (
	cpuPattern    = regexp.MustCompile(`(?:cpu[=:\s]+)(\d+)`)
	cldPattern    = regexp.MustCompile(`cld-runner-(\d+)`)
	docsPattern   = regexp.MustCompile(`runner=(\d+)cpu-(linux|windows)-(x64|arm64)`)
	imagePattern  = regexp.MustCompile(`(?:image[=:\s]+)[a-zA-Z0-9_-]*(arm64|x64)`)
	familyPattern = regexp.MustCompile(`(?:family[=:\s]+)([a-zA-Z0-9_+.*-]+)`)
)

// ExtractRunnerSpecFromContent inspects workflow YAML content and extracts the best matching runner key.
func ExtractRunnerSpecFromContent(content []byte) string {
	str := string(content)

	// Check for CLD runner: runner=cld-runner-16
	if matches := cldPattern.FindStringSubmatch(str); len(matches) > 1 {
		return matches[1] + "cpu-linux-x64"
	}

	// Check for docs runner: runner=2cpu-linux-x64
	if matches := docsPattern.FindStringSubmatch(str); len(matches) > 3 {
		return matches[1] + "cpu-" + matches[2] + "-" + matches[3]
	}

	// Check for cpu=N
	arch := "x64"
	if imgMatches := imagePattern.FindStringSubmatch(str); len(imgMatches) > 1 {
		arch = imgMatches[1]
	}

	// Check if ARM family is mentioned (e.g. c8g, m8g, graviton)
	if famMatches := familyPattern.FindStringSubmatch(str); len(famMatches) > 1 {
		if strings.Contains(famMatches[1], "g") {
			arch = "arm64"
		}
	}

	if cpuMatches := cpuPattern.FindStringSubmatch(str); len(cpuMatches) > 1 {
		return cpuMatches[1] + "cpu-linux-" + arch
	}

	// Default fallback for self-hosted
	return "2cpu-linux-x64"
}

// RateForRunnerSpec returns estimated rate in dollars per minute for a runner spec,
// calibrated against empirical Cloudzero AWS spot billing.
func RateForRunnerSpec(spec string) float64 {
	switch {
	case strings.HasPrefix(spec, "64cpu"):
		return 0.0260
	case strings.HasPrefix(spec, "48cpu"):
		return 0.0200
	case strings.HasPrefix(spec, "32cpu"):
		return 0.0130
	case strings.HasPrefix(spec, "16cpu"):
		if strings.Contains(spec, "arm64") {
			return 0.0075
		}
		return 0.0090
	case strings.HasPrefix(spec, "8cpu"):
		return 0.0065
	case strings.HasPrefix(spec, "4cpu"):
		return 0.0050
	default:
		return 0.0031
	}
}

func specCachePath(dataDir, owner, repo, workflowPath string) string {
	hash := sha256.Sum256([]byte(workflowPath))
	hashStr := hex.EncodeToString(hash[:8])
	base := filepath.Base(workflowPath)
	filename := fmt.Sprintf("%s-%s.json", base, hashStr)
	return filepath.Join(dataDir, "workflow_specs", owner, repo, filename)
}

// SaveWorkflowSpecCache persists a resolved workflow spec to disk.
func SaveWorkflowSpecCache(dataDir, owner string, spec WorkflowSpec) error {
	path := specCachePath(dataDir, owner, spec.Repository, spec.WorkflowPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o600)
}

// LoadWorkflowSpecCache attempts to load a cached workflow spec from disk.
func LoadWorkflowSpecCache(dataDir, owner, repo, workflowPath string) (*WorkflowSpec, bool) {
	path := specCachePath(dataDir, owner, repo, workflowPath)
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, false
	}
	var spec WorkflowSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, false
	}
	return &spec, true
}

// ResolveWorkflowSpecs enriches self-hosted workflows by resolving their runner specifications.
// Queries local cache first; only queries GitHub API for un-cached workflows.
func ResolveWorkflowSpecs(
	ctx context.Context,
	log zerolog.Logger,
	client *gather.GitHubClient,
	dataDir, owner string,
	uniqueWorkflows map[string]string, // key: "repo:workflowPath", value: repo
) map[string]*WorkflowSpec {
	results := make(map[string]*WorkflowSpec, len(uniqueWorkflows))

	for key, repo := range uniqueWorkflows {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		wfPath := parts[1]

		// 1. Try local cache
		if cached, ok := LoadWorkflowSpecCache(dataDir, owner, repo, wfPath); ok {
			results[key] = cached
			continue
		}

		// 2. Fetch from GitHub API if client provided
		var spec WorkflowSpec
		var fetched bool
		if client != nil && client.Rest != nil {
			content, _, resp, err := client.Rest.Repositories.GetContents(
				ctx,
				owner,
				repo,
				wfPath,
				&github.RepositoryContentGetOptions{},
			)
			if err == nil && resp.StatusCode == http.StatusOK && content != nil {
				raw, decodeErr := content.GetContent()
				if decodeErr == nil {
					specRunner := ExtractRunnerSpecFromContent([]byte(raw))
					spec = WorkflowSpec{
						Repository:   repo,
						WorkflowPath: wfPath,
						RunnerSpec:   specRunner,
						RatePerMin:   RateForRunnerSpec(specRunner),
					}
					fetched = true
					_ = SaveWorkflowSpecCache(dataDir, owner, spec)
					log.Debug().
						Str("repo", repo).
						Str("workflow", wfPath).
						Str("spec", specRunner).
						Msg("Enriched workflow spec from GitHub API")
				}
			}
		}

		if !fetched {
			// 3. Fallback based on known path heuristics
			specRunner := fallbackSpecForWorkflow(repo, wfPath)
			spec = WorkflowSpec{
				Repository:   repo,
				WorkflowPath: wfPath,
				RunnerSpec:   specRunner,
				RatePerMin:   RateForRunnerSpec(specRunner),
			}
		}

		results[key] = &spec
	}

	return results
}

func fallbackSpecForWorkflow(_, wfPath string) string {
	lowerPath := strings.ToLower(wfPath)
	switch {
	case strings.Contains(lowerPath, "integration") || strings.Contains(lowerPath, "smoke") || strings.Contains(lowerPath, "e2e"):
		return "32cpu-linux-x64"
	case strings.Contains(lowerPath, "proposal") || strings.Contains(lowerPath, "deploy") || strings.Contains(lowerPath, "codeql"):
		return "16cpu-linux-x64"
	case strings.Contains(lowerPath, "core") || strings.Contains(lowerPath, "ci"):
		return "8cpu-linux-x64"
	default:
		return "8cpu-linux-x64"
	}
}
