package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillCmdRegistration(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, skillCmd)
	assert.Equal(t, "skill", skillCmd.Use)
	assert.Contains(t, skillCmd.Aliases, "instructions")
	assert.Contains(t, skillCmd.Aliases, "agent")

	sub, _, err := rootCmd.Find([]string{"skill"})
	require.NoError(t, err)
	assert.Equal(t, "skill", sub.Use)

	subInst, _, err := rootCmd.Find([]string{"instructions"})
	require.NoError(t, err)
	assert.Equal(t, "skill", subInst.Use)

	subAgent, _, err := rootCmd.Find([]string{"agent"})
	require.NoError(t, err)
	assert.Equal(t, "skill", subAgent.Use)
}

//nolint:paralleltest // modifies skillCmd output buffer
func TestSkillCmdOutput(t *testing.T) {
	var buf bytes.Buffer
	skillCmd.SetOut(&buf)
	defer skillCmd.SetOut(nil)

	err := skillCmd.RunE(skillCmd, []string{})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "name: octometrics")
	assert.Contains(t, out, "Triage CI Failures")
	assert.Contains(t, out, "Profile Workflow Bottlenecks")
	assert.Contains(t, out, "Compare Workflow Performance")
	assert.Contains(t, out, "octometrics log")
}

func init() {
	rootSkillPath := filepath.Clean(filepath.Join("..", "SKILL.md"))
	data, err := os.ReadFile(rootSkillPath)
	if err == nil {
		SetSkillContent(string(data))
	}
}

func TestSkillTokenBypass(t *testing.T) {
	t.Parallel()

	assert.False(t, commandNeedsGitHubToken(skillCmd))
}
