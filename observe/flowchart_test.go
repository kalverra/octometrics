package observe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
)

func TestBuildFlowChart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		def       *gather.WorkflowDef
		jobs      []*gather.JobData
		want      string
		wantEmpty bool
	}{
		{
			name: "simple chain build -> test -> deploy",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"build":  {Name: "build"},
					"test":   {Name: "test", Needs: gather.Needs{"build"}},
					"deploy": {Name: "deploy", Needs: gather.Needs{"test"}},
				},
			},
			jobs: []*gather.JobData{},
			want: `flowchart TD
    build["build"]
    deploy["deploy"]
    test["test"]
    build --> test
    test --> deploy`,
		},
		{
			name: "fan-out: build -> [lint, test]",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"build": {Name: "build"},
					"test":  {Name: "test", Needs: gather.Needs{"build"}},
					"lint":  {Name: "lint", Needs: gather.Needs{"build"}},
				},
			},
			jobs: []*gather.JobData{},
			want: `flowchart TD
    build["build"]
    lint["lint"]
    test["test"]
    build --> lint
    build --> test`,
		},
		{
			name: "fan-in: [build, lint] -> deploy",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"build":  {Name: "build"},
					"lint":   {Name: "lint"},
					"deploy": {Name: "deploy", Needs: gather.Needs{"build", "lint"}},
				},
			},
			jobs: []*gather.JobData{},
			want: `flowchart TD
    build["build"]
    deploy["deploy"]
    lint["lint"]
    build --> deploy
    lint --> deploy`,
		},
		{
			name: "custom name used in node",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"build": {Name: "Build Application"},
				},
			},
			jobs: []*gather.JobData{},
			want: `flowchart TD
    build["Build Application"]`,
		},
		{
			name:      "nil def",
			def:       nil,
			wantEmpty: true,
		},
		{
			name:      "empty jobs",
			def:       &gather.WorkflowDef{Jobs: map[string]gather.JobDef{}},
			wantEmpty: true,
		},
		{
			name: "job with no needs",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"standalone": {Name: "standalone"},
				},
			},
			jobs: []*gather.JobData{},
			want: `flowchart TD
    standalone["standalone"]`,
		},
		{
			name: "reusable workflow job",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"call-workflow": {Name: "call-workflow", Uses: "./.github/workflows/reusable.yml"},
				},
			},
			jobs: []*gather.JobData{},
			want: `flowchart TD
    call_workflow["call-workflow"]`,
		},
		{
			name: "status styling from runtime jobs",
			def: &gather.WorkflowDef{
				Jobs: map[string]gather.JobDef{
					"build": {Name: "build"},
					"test":  {Name: "test", Needs: gather.Needs{"build"}},
				},
			},
			jobs: nil,
			want: `flowchart TD
    build["build"]
    test["test"]
    build --> test`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildFlowChart(tt.def, tt.jobs)
			if tt.wantEmpty {
				assert.Empty(t, got)
				return
			}
			require.NotEmpty(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}
