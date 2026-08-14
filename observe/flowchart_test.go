package observe

import (
	"testing"

	"github.com/google/go-github/v89/github"
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

func TestBuildFlowChart_ChildAndMatrixJobs(t *testing.T) {
	t.Parallel()

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"build-chainlink": {Name: "build-chainlink"},
			"run-core-cre-e2e-tests": {
				Name:  "run-core-cre-e2e-tests",
				Needs: gather.Needs{"build-chainlink"},
				Uses:  "./.github/workflows/e2e.yml",
			},
		},
	}

	jobs := []*gather.JobData{
		{
			WorkflowJob: &github.WorkflowJob{
				ID:         new(int64(1)),
				Name:       new("build-chainlink"),
				Status:     new("completed"),
				Conclusion: new("success"),
			},
		},
		{
			WorkflowJob: &github.WorkflowJob{
				ID:         new(int64(2)),
				Name:       new("run-core-cre-e2e-tests / define-test-matrix"),
				Status:     new("completed"),
				Conclusion: new("success"),
			},
		},
		{
			WorkflowJob: &github.WorkflowJob{
				ID:         new(int64(3)),
				Name:       new("run-core-cre-e2e-tests / Bucket_B (1, 2)"),
				Status:     new("completed"),
				Conclusion: new("success"),
			},
		},
	}

	chart := buildFlowChart(def, jobs)
	assert.Contains(t, chart, "define_test_matrix")
	assert.Contains(t, chart, "build_chainlink -->")
}

func TestBuildFlowChart_SanitizationAndParentMatching(t *testing.T) {
	t.Parallel()

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"run_core_cre_e2e_regression_tests": {
				Name: "Run Core CRE E2E Regression Tests",
			},
			"test_job": {
				Name:  "Test Job",
				Needs: gather.Needs{"Run_Core_CRE_E2E_Regression_Tests"},
			},
		},
	}

	jobs := []*gather.JobData{
		{
			WorkflowJob: &github.WorkflowJob{
				ID:   new(int64(1)),
				Name: new("Run Core CRE E2E Regression Tests / Test_CRE_V2_EVM_Read_HeavyCalls (mixed-env)"),
			},
		},
	}

	chart := buildFlowChart(def, jobs)

	// Node IDs in Mermaid statement (left of label bracket) must not contain parentheses
	assert.Contains(
		t,
		chart,
		"Test_CRE_V2_EVM_Read_HeavyCalls__mixed_env_[\"Test_CRE_V2_EVM_Read_HeavyCalls (mixed-env)\"]",
	)

	// Edge source for child job must match parent node ID run_core_cre_e2e_regression_tests
	assert.Contains(t, chart, "run_core_cre_e2e_regression_tests --> Test_CRE_V2_EVM_Read_HeavyCalls__mixed_env_")

	// Edge for needs matching must map Run_Core_CRE_E2E_Regression_Tests to run_core_cre_e2e_regression_tests
	assert.Contains(t, chart, "run_core_cre_e2e_regression_tests --> test_job")
}
