package observe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kalverra/octometrics/gather"
)

// buildFlowChart generates a Mermaid flowchart TD string showing job dependencies.
// Nodes are styled based on runtime job conclusions where available.
// Returns empty string if def is nil or has no jobs.
func buildFlowChart(def *gather.WorkflowDef, jobs []*gather.JobData) string {
	if def == nil || len(def.Jobs) == 0 {
		return ""
	}

	// Map runtime job names to conclusions for styling
	jobConclusions := make(map[string]string)
	for _, job := range jobs {
		if job == nil || job.WorkflowJob == nil {
			continue
		}
		name := job.GetName()
		if name == "" {
			name = fmt.Sprint(job.GetID())
		}
		jobConclusions[name] = job.GetConclusion()
	}

	// Sort job IDs for deterministic output
	jobIDs := make([]string, 0, len(def.Jobs))
	for id := range def.Jobs {
		jobIDs = append(jobIDs, id)
	}
	sort.Strings(jobIDs)

	var b strings.Builder
	b.WriteString("flowchart TD\n")

	// Emit nodes
	for _, id := range jobIDs {
		job := def.Jobs[id]
		displayName := job.Name
		if displayName == "" {
			displayName = id
		}
		sanitized := sanitizeMermaidID(id)
		sanizedName := sanitizeMermaidName(displayName)
		fmt.Fprintf(&b, "    %s[%q]\n", sanitized, sanizedName)
	}

	// Emit edges (sorted by source then target for determinism)
	type edge struct {
		from, to string
	}
	var edges []edge
	for _, id := range jobIDs {
		job := def.Jobs[id]
		for _, need := range job.Needs {
			edges = append(edges, edge{from: need, to: id})
		}
	}

	// Process extra runtime jobs (e.g. reusable workflow child jobs like "parent / child")
	extraNodes := make(map[string]string)
	for _, job := range jobs {
		if job == nil || job.WorkflowJob == nil {
			continue
		}
		name := job.GetName()
		if strings.Contains(name, " / ") {
			parts := strings.Split(name, " / ")
			parent := strings.TrimSpace(parts[0])
			child := strings.TrimSpace(parts[len(parts)-1])
			parentID := sanitizeMermaidID(parent)
			childID := sanitizeMermaidID(child)

			if _, exists := def.Jobs[childID]; !exists {
				extraNodes[childID] = child
				edges = append(edges, edge{from: parentID, to: childID})
			}
		}
	}

	extraIDs := make([]string, 0, len(extraNodes))
	for id := range extraNodes {
		extraIDs = append(extraIDs, id)
	}
	sort.Strings(extraIDs)

	for _, id := range extraIDs {
		sanitized := sanitizeMermaidID(id)
		sanitizedName := sanitizeMermaidName(extraNodes[id])
		fmt.Fprintf(&b, "    %s[%q]\n", sanitized, sanitizedName)
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		return edges[i].to < edges[j].to
	})

	for _, e := range edges {
		fmt.Fprintf(&b, "    %s --> %s\n", sanitizeMermaidID(e.from), sanitizeMermaidID(e.to))
	}

	// Trim trailing newline for clean comparison
	result := strings.TrimRight(b.String(), "\n")
	return result
}

// sanitizeMermaidID sanitizes a string for use as a Mermaid node ID.
func sanitizeMermaidID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}
