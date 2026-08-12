package observe

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// CheckWorkflowDrift checks git log for changes to .github/workflows between baseSHA and HEAD.
func CheckWorkflowDrift(repoDir, baseSHA string) []string {
	if baseSHA == "" {
		return nil
	}
	//nolint:gosec // execution of git command for SHA verification
	verifyCmd := exec.Command("git", "cat-file", "-e", fmt.Sprintf("%s^{commit}", baseSHA))
	if repoDir != "" {
		verifyCmd.Dir = repoDir
	}
	if err := verifyCmd.Run(); err != nil {
		return []string{
			fmt.Sprintf(
				"⚠ Could not resolve run SHA %s in local git repository; skipping workflow drift check.",
				baseSHA,
			),
		}
	}

	//nolint:gosec // execution of git command for workflow drift detection
	cmd := exec.Command(
		"git",
		"log",
		"--name-only",
		"--oneline",
		fmt.Sprintf("%s..HEAD", baseSHA),
		"--",
		".github/workflows/",
	)
	if repoDir != "" {
		cmd.Dir = repoDir
	}
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	counts := make(map[string]int)
	lines := strings.SplitSeq(string(out), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ".github/workflows/") {
			counts[line]++
		}
	}

	if len(counts) == 0 {
		return nil
	}

	var files []string
	for f := range counts {
		files = append(files, f)
	}
	sort.Strings(files)

	var warnings []string
	for _, f := range files {
		c := counts[f]
		warnings = append(warnings, fmt.Sprintf("⚠ %s changed in %d commit(s) since this run.", f, c))
	}
	return warnings
}
