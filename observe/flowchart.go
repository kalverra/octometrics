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
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", sanitized, sanizedName)
	}

	// Helper for recording edges, resolving job names to canonical IDs
	type edge struct {
		from, to string
	}
	var edges []edge
	seenEdges := make(map[edge]bool)

	addEdge := func(from, to string) {
		if targetID, found := def.GetJobIDByName(from); found {
			from = targetID
		}
		if targetID, found := def.GetJobIDByName(to); found {
			to = targetID
		}
		e := edge{from: sanitizeMermaidID(from), to: sanitizeMermaidID(to)}
		if e.from != "" && e.to != "" && !seenEdges[e] {
			seenEdges[e] = true
			edges = append(edges, e)
		}
	}

	for _, id := range jobIDs {
		job := def.Jobs[id]
		for _, need := range job.Needs {
			addEdge(need, id)
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

			parentID := parent
			if targetID, found := def.GetJobIDByName(parent); found {
				parentID = targetID
			}

			childSanitized := sanitizeMermaidID(child)
			if _, exists := def.Jobs[child]; !exists {
				if _, existsID := def.Jobs[childSanitized]; !existsID {
					extraNodes[childSanitized] = child
					addEdge(parentID, child)
				}
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
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", sanitized, sanitizedName)
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		return edges[i].to < edges[j].to
	})

	for _, e := range edges {
		fmt.Fprintf(&b, "    %s --> %s\n", e.from, e.to)
	}

	// Trim trailing newline for clean comparison
	result := strings.TrimRight(b.String(), "\n")
	return result
}

// sanitizeMermaidID sanitizes a string for use as a Mermaid node ID.
func sanitizeMermaidID(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := b.String()
	if res == "" {
		return "node"
	}
	return res
}
