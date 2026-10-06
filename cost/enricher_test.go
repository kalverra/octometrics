package cost

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRunnerSpecFromYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantSpec string
	}{
		{
			name: "chainlink cld-runner-16",
			yaml: `
jobs:
  build:
    runs-on: runs-on=${{ github.run_id }}/runner=cld-runner-16/image=ccip
`,
			wantSpec: "16cpu-linux-x64",
		},
		{
			name: "chainlink explicit cpu=32 and family",
			yaml: `
jobs:
  test:
    runs-on:
      - runs-on=${{ github.run_id }}
      - cpu=32
      - ram=64
      - family=c7i+c8i
      - image=ubuntu24-full-x64
`,
			wantSpec: "32cpu-linux-x64",
		},
		{
			name: "chainlink arm64 docker build",
			yaml: `
jobs:
  docker:
    runs-on:
      - runs-on=${{ github.run_id }}
      - cpu=16
      - image=ubuntu24-full-arm64
`,
			wantSpec: "16cpu-linux-arm64",
		},
		{
			name: "infra-griddle-app docs runner=2cpu",
			yaml: `
jobs:
  test:
    runs-on:
      - runs-on=${{ github.run_id }}
      - runner=2cpu-linux-x64
`,
			wantSpec: "2cpu-linux-x64",
		},
		{
			name: "generic self-hosted fallback",
			yaml: `
jobs:
  test:
    runs-on: [self-hosted, linux, x64]
`,
			wantSpec: "2cpu-linux-x64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractRunnerSpecFromContent([]byte(tt.yaml))
			assert.Equal(t, tt.wantSpec, got)
		})
	}
}

func TestWorkflowSpecCache(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	spec := WorkflowSpec{
		Repository:   "chainlink",
		WorkflowPath: ".github/workflows/ci.yml",
		RunnerSpec:   "32cpu-linux-x64",
		RatePerMin:   0.098,
	}

	err := SaveWorkflowSpecCache(tempDir, "smartcontractkit", spec)
	require.NoError(t, err)

	loaded, ok := LoadWorkflowSpecCache(tempDir, "smartcontractkit", "chainlink", ".github/workflows/ci.yml")
	require.True(t, ok)
	assert.Equal(t, "32cpu-linux-x64", loaded.RunnerSpec)
	assert.InDelta(t, 0.098, loaded.RatePerMin, 0.0001)

	expectedPath := specCachePath(tempDir, "smartcontractkit", "chainlink", ".github/workflows/ci.yml")
	_, statErr := os.Stat(expectedPath)
	require.NoError(t, statErr)
}
