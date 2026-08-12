package observe

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/gather"
)

func TestCalculateCriticalPath(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	job1 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(101)),
			Name:        new("Build"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   &github.Timestamp{Time: now.Add(-10 * time.Second)},
			StartedAt:   &github.Timestamp{Time: now},
			CompletedAt: &github.Timestamp{Time: now.Add(120 * time.Second)},
		},
	}

	job2 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(102)),
			Name:        new("Test A"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   &github.Timestamp{Time: now.Add(120 * time.Second)},
			StartedAt:   &github.Timestamp{Time: now.Add(125 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(300 * time.Second)},
		},
	}

	job3 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(103)),
			Name:        new("Test B"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			CreatedAt:   &github.Timestamp{Time: now.Add(120 * time.Second)},
			StartedAt:   &github.Timestamp{Time: now.Add(130 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(180 * time.Second)},
		},
	}

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"build":  {Name: "Build"},
			"test_a": {Name: "Test A", Needs: []string{"build"}},
			"test_b": {Name: "Test B", Needs: []string{"build"}},
		},
	}

	cp := CalculateCriticalPath([]*gather.JobData{job1, job2, job3}, def)

	if cp == nil {
		t.Fatal("expected non-nil CriticalPathInfo")
	}

	if len(cp.CriticalNodes) != 2 {
		t.Fatalf("expected 2 nodes on critical path, got %d", len(cp.CriticalNodes))
	}

	if cp.CriticalNodes[0].JobName != "Build" || cp.CriticalNodes[1].JobName != "Test A" {
		t.Errorf("unexpected critical path order: %v -> %v", cp.CriticalNodes[0].JobName, cp.CriticalNodes[1].JobName)
	}

	var job3Node *CriticalPathNode
	for i := range cp.Nodes {
		if cp.Nodes[i].JobID == 103 {
			job3Node = &cp.Nodes[i]
			break
		}
	}
	if job3Node == nil {
		t.Fatal("job3 node not found in CriticalPathInfo.Nodes")
	}
	if job3Node.Slack != 120*time.Second {
		t.Errorf("expected slack of 120s for Job 3, got %v", job3Node.Slack)
	}
}

func TestCalculateCriticalPath_ExcludesSkippedJobs(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	job1 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(1)),
			Name:        new("Build"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now},
			CompletedAt: &github.Timestamp{Time: now.Add(100 * time.Second)},
		},
	}

	jobSkipped := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(2)),
			Name:        new("Skipped Job"),
			Status:      new("completed"),
			Conclusion:  new("skipped"),
			StartedAt:   &github.Timestamp{Time: now.Add(100 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(100 * time.Second)},
		},
	}

	job2 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(3)),
			Name:        new("Run Tests"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(105 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(500 * time.Second)},
		},
	}

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"build":     {Name: "Build"},
			"skipped":   {Name: "Skipped Job", Needs: []string{"build"}},
			"run_tests": {Name: "Run Tests", Needs: []string{"build"}},
		},
	}

	cp := CalculateCriticalPath([]*gather.JobData{job1, jobSkipped, job2}, def)
	if cp == nil {
		t.Fatal("expected non-nil critical path")
	}

	for _, node := range cp.CriticalNodes {
		if node.JobName == "Skipped Job" {
			t.Errorf("skipped job should never be on critical path")
		}
	}
}

func TestCategorizeStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
	}{
		{"Set up job", "runner-overhead"},
		{"Checkout", "runner-overhead"},
		{"Set up Go", "runner-overhead"},
		{"ECR login", "runner-overhead"},
		{"Post Set up Go", "runner-overhead"},
		{"Complete job", "runner-overhead"},

		{"Start local CRE", "test-setup"},
		{"Setup Aptos CLI", "test-setup"},
		{"observability stack", "test-setup"},
		{"Install dependencies", "test-setup"},

		{"Run CRE Smoke system tests (mixed-env)", "test-execution"},
		{"go test ./...", "test-execution"},
		{"gotestsum", "test-execution"},
		{"Test_CRE_V2_Suite_Bucket_B", "test-execution"},
	}

	for _, tt := range tests {
		cat := CategorizeStep(tt.name)
		if cat != tt.expected {
			t.Errorf("CategorizeStep(%q) = %q; expected %q", tt.name, cat, tt.expected)
		}
	}
}

func TestAggregateSteps(t *testing.T) {
	t.Parallel()
	now := time.Now()

	job1 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:         new(int64(1)),
			Name:       new("Job 1"),
			Status:     new("completed"),
			Conclusion: new("success"),
			Steps: []*github.TaskStep{
				{
					Name:        new("Set up Go"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now},
					CompletedAt: &github.Timestamp{Time: now.Add(15 * time.Second)},
				},
				{
					Name:        new("Run tests"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now.Add(15 * time.Second)},
					CompletedAt: &github.Timestamp{Time: now.Add(100 * time.Second)},
				},
			},
		},
	}

	job2 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:         new(int64(2)),
			Name:       new("Job 2"),
			Status:     new("completed"),
			Conclusion: new("success"),
			Steps: []*github.TaskStep{
				{
					Name:        new("Set up Go"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now},
					CompletedAt: &github.Timestamp{Time: now.Add(25 * time.Second)},
				},
				{
					Name:        new("Run tests"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now.Add(25 * time.Second)},
					CompletedAt: &github.Timestamp{Time: now.Add(150 * time.Second)},
				},
			},
		},
	}

	summaries, pctMap := AggregateSteps([]*gather.JobData{job1, job2})

	if len(summaries) != 2 {
		t.Fatalf("expected 2 step summaries, got %d", len(summaries))
	}

	if summaries[0].Name != "Run tests" {
		t.Errorf("expected 'Run tests' to be first, got %q", summaries[0].Name)
	}
	if summaries[0].Count != 2 {
		t.Errorf("expected count 2, got %d", summaries[0].Count)
	}
	if summaries[0].TotalDuration != 210*time.Second {
		t.Errorf("expected total duration 210s, got %v", summaries[0].TotalDuration)
	}
	if pct, ok := pctMap["Run tests"]; !ok || pct <= 0 {
		t.Errorf("expected positive percentage for 'Run tests', got %v", pct)
	}
}

func TestGetSlowestJobSteps(t *testing.T) {
	t.Parallel()
	now := time.Now()

	jobSlow := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(20)),
			Name:        new("Slow Job"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now},
			CompletedAt: &github.Timestamp{Time: now.Add(300 * time.Second)},
			Steps: []*github.TaskStep{
				{
					Name:        new("Set up Go"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now},
					CompletedAt: &github.Timestamp{Time: now.Add(20 * time.Second)},
				},
				{
					Name:        new("Start local CRE"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now.Add(20 * time.Second)},
					CompletedAt: &github.Timestamp{Time: now.Add(100 * time.Second)},
				},
				{
					Name:        new("go test ./..."),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now.Add(100 * time.Second)},
					CompletedAt: &github.Timestamp{Time: now.Add(290 * time.Second)},
				},
				{
					Name:        new("Post Set up Go"),
					Status:      new("completed"),
					Conclusion:  new("success"),
					StartedAt:   &github.Timestamp{Time: now.Add(290 * time.Second)},
					CompletedAt: &github.Timestamp{Time: now.Add(291 * time.Second)}, // <=1s minor step
				},
			},
		},
	}

	breakdowns := GetSlowestJobSteps([]*gather.JobData{jobSlow}, 1)
	if len(breakdowns) != 1 {
		t.Fatalf("expected 1 breakdown for top 1, got %d", len(breakdowns))
	}

	bd := breakdowns[0]
	if bd.RunnerOverheadTotal != 21*time.Second {
		t.Errorf("expected runner overhead 21s, got %v", bd.RunnerOverheadTotal)
	}
	if bd.TestSetupTotal != 80*time.Second {
		t.Errorf("expected test setup 80s, got %v", bd.TestSetupTotal)
	}
	if bd.TestExecutionTotal != 190*time.Second {
		t.Errorf("expected test execution 190s, got %v", bd.TestExecutionTotal)
	}
	if bd.MinorStepsCount != 1 {
		t.Errorf("expected 1 minor step <=1s collapsed, got %d", bd.MinorStepsCount)
	}
}

func TestSanitizeMermaidNameMiddleTruncate(t *testing.T) {
	t.Parallel()
	longName := "Workflow / Extra Long Job Name With Category E2E Regression Tests / Test_CRE_V2_EVM_BalanceAt_Invalid_Address_Regression"
	sanitized := sanitizeMermaidName(longName)
	if len(sanitized) > 80 {
		t.Errorf("expected length <= 80, got %d", len(sanitized))
	}
	if !containsPrefixAndSuffix(sanitized, "Workflow", "Regression") {
		t.Errorf("expected middle truncation preserving prefix and suffix, got %q", sanitized)
	}
}

func containsPrefixAndSuffix(s, prefix, suffix string) bool {
	return len(s) >= len(prefix)+len(suffix) && s[:len(prefix)] == prefix && s[len(s)-len(suffix):] == suffix
}

func TestCalculateCriticalPath_MatrixAndNeedsMapping(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC)

	// Chain: changes -> build-chainlink -> compile-tests -> define-test-matrix -> ETH Smoke Tests -> gate
	j1 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(1)),
			Name:        new("changes"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now},
			CompletedAt: &github.Timestamp{Time: now.Add(10 * time.Second)},
		},
	}
	j2 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(2)),
			Name:        new("build-chainlink"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(10 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(60 * time.Second)},
		},
	}
	j3 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(3)),
			Name:        new("compile-tests"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(60 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(120 * time.Second)},
		},
	}
	j4 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(4)),
			Name:        new("define-test-matrix"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(120 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(130 * time.Second)},
		},
	}
	j5 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(5)),
			Name:        new("ETH Smoke Tests"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(130 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(600 * time.Second)},
		},
	}
	j6 := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(6)),
			Name:        new("gate"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(600 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(602 * time.Second)},
		},
	}

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"changes":            {Name: "changes"},
			"build-chainlink":    {Name: "build-chainlink", Needs: []string{"changes"}},
			"compile-tests":      {Name: "compile-tests", Needs: []string{"build-chainlink"}},
			"define-test-matrix": {Name: "define-test-matrix", Needs: []string{"compile-tests"}},
			"run-smoke-tests":    {Name: "ETH Smoke Tests", Needs: []string{"define-test-matrix"}},
			"gate":               {Name: "gate", Needs: []string{"run-smoke-tests"}},
		},
	}

	cp := CalculateCriticalPath([]*gather.JobData{j1, j2, j3, j4, j5, j6}, def)
	require.NotNil(t, cp)
	assert.GreaterOrEqual(t, len(cp.CriticalNodes), 5, "critical path should trace full chain, not just 1-2 nodes")
}

func TestCalculateCriticalPath_DeterministicTieBreak(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC)

	// Parallel jobs A and B finish at exact same time with equal duration
	jBuild := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(1)),
			Name:        new("Build"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now},
			CompletedAt: &github.Timestamp{Time: now.Add(100 * time.Second)},
		},
	}
	jA := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(2)),
			Name:        new("Test_A"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(100 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(300 * time.Second)},
		},
	}
	jB := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(3)),
			Name:        new("Test_B"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(100 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(300 * time.Second)},
		},
	}

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"build":  {Name: "Build"},
			"test_a": {Name: "Test_A", Needs: []string{"build"}},
			"test_b": {Name: "Test_B", Needs: []string{"build"}},
		},
	}

	var firstPath string
	for range 20 {
		cp := CalculateCriticalPath([]*gather.JobData{jBuild, jA, jB}, def)
		require.NotNil(t, cp)
		var names []string
		for _, n := range cp.CriticalNodes {
			names = append(names, n.JobName)
		}
		pathStr := strings.Join(names, " -> ")
		if firstPath == "" {
			firstPath = pathStr
		} else {
			assert.Equal(t, firstPath, pathStr, "Critical path must be 100% deterministic across repeated runs")
		}
	}
}

func TestCalculateCriticalPath_TrailingSpaceMatching(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC)

	// YAML definition template has trailing space "Build Chainlink Image "
	jBuild := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(1)),
			Name:        new("Build Chainlink Image"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now},
			CompletedAt: &github.Timestamp{Time: now.Add(200 * time.Second)},
		},
	}
	jTest := &gather.JobData{
		WorkflowJob: &github.WorkflowJob{
			ID:          new(int64(2)),
			Name:        new("Run E2E Tests"),
			Status:      new("completed"),
			Conclusion:  new("success"),
			StartedAt:   &github.Timestamp{Time: now.Add(200 * time.Second)},
			CompletedAt: &github.Timestamp{Time: now.Add(500 * time.Second)},
		},
	}

	def := &gather.WorkflowDef{
		Jobs: map[string]gather.JobDef{
			"build-image": {Name: "Build Chainlink Image "},
			"run-e2e":     {Name: "Run E2E Tests", Needs: []string{"build-image"}},
		},
	}

	cp := CalculateCriticalPath([]*gather.JobData{jBuild, jTest}, def)
	require.NotNil(t, cp)
	assert.Len(t, cp.CriticalNodes, 2, "Build Chainlink Image must be linked to Run E2E Tests despite trailing space")
	assert.Equal(t, "Build Chainlink Image", cp.CriticalNodes[0].JobName)
}
