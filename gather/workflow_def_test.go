package gather

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflowDef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantJobs map[string]JobDef
		wantErr  bool
	}{
		{
			name: "simple workflow with needs",
			yaml: `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo test
  deploy:
    needs: [build, test]
    runs-on: ubuntu-latest
    steps:
      - run: echo deploy
`,
			wantJobs: map[string]JobDef{
				"build": {
					Name:   "build",
					Needs:  nil,
					RunsOn: "ubuntu-latest",
				},
				"test": {
					Name:   "test",
					Needs:  []string{"build"},
					RunsOn: "ubuntu-latest",
				},
				"deploy": {
					Name:   "deploy",
					Needs:  []string{"build", "test"},
					RunsOn: "ubuntu-latest",
				},
			},
		},
		{
			name: "job with custom name",
			yaml: `
name: CI
on: push
jobs:
  build:
    name: Build Application
    runs-on: ubuntu-latest
    steps:
      - run: echo build
`,
			wantJobs: map[string]JobDef{
				"build": {
					Name:   "Build Application",
					Needs:  nil,
					RunsOn: "ubuntu-latest",
				},
			},
		},
		{
			name: "reusable workflow",
			yaml: `
name: CI
on: push
jobs:
  call-workflow:
    uses: ./.github/workflows/reusable.yml
`,
			wantJobs: map[string]JobDef{
				"call-workflow": {
					Name: "call-workflow",
					Uses: "./.github/workflows/reusable.yml",
				},
			},
		},
		{
			name: "matrix job",
			yaml: `
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        node: [14, 16, 18]
    steps:
      - run: echo test
`,
			wantJobs: map[string]JobDef{
				"test": {
					Name:   "test",
					Needs:  nil,
					RunsOn: "ubuntu-latest",
				},
			},
		},
		{
			name: "runs-on as list",
			yaml: `
name: CI
on: push
jobs:
  build:
    runs-on: [self-hosted, linux]
    steps:
      - run: echo build
`,
			wantJobs: map[string]JobDef{
				"build": {
					Name:   "build",
					Needs:  nil,
					RunsOn: "self-hosted, linux",
				},
			},
		},
		{
			name: "no jobs",
			yaml: `
name: Empty
on: push
`,
			wantJobs: nil,
		},
		{
			name:    "invalid yaml",
			yaml:    "invalid: [yaml: broken",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def, err := ParseWorkflowDef([]byte(tt.yaml))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, def)
			assert.Equal(t, tt.wantJobs, def.Jobs)
		})
	}
}

func TestWorkflowDefGetJobID(t *testing.T) {
	t.Parallel()

	def := &WorkflowDef{
		Jobs: map[string]JobDef{
			"build":                             {Name: "Build Application"},
			"test":                              {Name: "test"},
			"run-core-cre-e2e-regression-tests": {Name: "Run Core CRE E2E Regression Tests"},
		},
	}

	// Find by name
	id, ok := def.GetJobIDByName("Build Application")
	assert.True(t, ok)
	assert.Equal(t, "build", id)

	// Find by job ID (fallback)
	id, ok = def.GetJobIDByName("test")
	assert.True(t, ok)
	assert.Equal(t, "test", id)

	// Find by case-insensitive / sanitized name
	id, ok = def.GetJobIDByName("Run_Core_CRE_E2E_Regression_Tests")
	assert.True(t, ok)
	assert.Equal(t, "run-core-cre-e2e-regression-tests", id)

	// Not found
	_, ok = def.GetJobIDByName("unknown")
	assert.False(t, ok)
}
