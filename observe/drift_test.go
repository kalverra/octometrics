package observe

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckWorkflowDrift(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	runCmd := func(args ...string) {
		//nolint:gosec // git commands in unit test
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runCmd("init")
	runCmd("config", "user.name", "Test User")
	runCmd("config", "user.email", "test@example.com")
	runCmd("config", "commit.gpgsign", "false")

	wfDir := filepath.Join(tmpDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o700))

	wfFile := filepath.Join(wfDir, "test.yaml")
	require.NoError(t, os.WriteFile(wfFile, []byte("name: test\n"), 0o600))
	runCmd("add", ".")
	runCmd("commit", "--no-gpg-sign", "-m", "Initial commit")

	//nolint:gosec // git rev-parse in unit test
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	require.NoError(t, err)
	baseSHA := string(out)[:40]

	// Make another commit modifying the workflow
	require.NoError(t, os.WriteFile(wfFile, []byte("name: test\n# update\n"), 0o600))
	runCmd("add", ".")
	runCmd("commit", "--no-gpg-sign", "-m", "Update workflow")

	warnings := CheckWorkflowDrift(tmpDir, baseSHA)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], ".github/workflows/test.yaml changed in 1 commit(s) since this run")
}

func TestCheckWorkflowDrift_UnresolvableSHA(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	runCmd := func(args ...string) {
		//nolint:gosec // git commands in unit test
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runCmd("init")
	runCmd("config", "user.name", "Test User")
	runCmd("config", "user.email", "test@example.com")
	runCmd("config", "commit.gpgsign", "false")

	fakeSHA := "1234567890abcdef1234567890abcdef12345678"
	warnings := CheckWorkflowDrift(tmpDir, fakeSHA)
	require.Len(t, warnings, 1)
	assert.Contains(
		t,
		warnings[0],
		"Could not resolve run SHA 1234567890abcdef1234567890abcdef12345678 in local git repository",
	)
}
