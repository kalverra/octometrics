package observe

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncludeWorkflowsOption(t *testing.T) {
	t.Parallel()

	opts := defaultOptions()
	IncludeWorkflows([]string{"wf-a", "wf-b"})(opts)
	assert.Equal(t, []string{"wf-a", "wf-b"}, opts.includeWorkflows)
}

func TestExcludeWorkflowsOption(t *testing.T) {
	t.Parallel()

	opts := defaultOptions()
	ExcludeWorkflows([]string{"wf-x"})(opts)
	assert.Equal(t, []string{"wf-x"}, opts.excludeWorkflows)
}

func TestShouldIncludeWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		wfName string
		opts   *options
		want   bool
	}{
		{
			name:   "no filters includes all",
			wfName: "build",
			opts:   defaultOptions(),
			want:   true,
		},
		{
			name:   "excluded by name",
			wfName: "build",
			opts:   &options{excludeWorkflows: []string{"build"}},
			want:   false,
		},
		{
			name:   "not in exclude list passes",
			wfName: "test",
			opts:   &options{excludeWorkflows: []string{"build"}},
			want:   true,
		},
		{
			name:   "in include list passes",
			wfName: "build",
			opts:   &options{includeWorkflows: []string{"build", "test"}},
			want:   true,
		},
		{
			name:   "not in include list filtered out",
			wfName: "lint",
			opts:   &options{includeWorkflows: []string{"build", "test"}},
			want:   false,
		},
		{
			name:   "empty include list includes all",
			wfName: "build",
			opts:   &options{includeWorkflows: nil},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, shouldIncludeWorkflow(tt.wfName, tt.opts))
		})
	}
}

func TestDefaultOptionsNoFilters(t *testing.T) {
	t.Parallel()

	opts := defaultOptions()
	require.Nil(t, opts.excludeWorkflows)
	require.Nil(t, opts.includeWorkflows)
	assert.False(t, opts.noOpen)
	assert.Equal(t, 8080, opts.port)
}

func TestWithNoOpenOption(t *testing.T) {
	t.Parallel()

	opts := defaultOptions()
	WithNoOpen(true)(opts)
	assert.True(t, opts.noOpen)
}

func TestWithPortOption(t *testing.T) {
	t.Parallel()

	opts := defaultOptions()
	WithPort(8081)(opts)
	assert.Equal(t, 8081, opts.port)
}

func TestPrintObserveURLs(t *testing.T) {
	t.Parallel()

	t.Run("without initial path", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		printObserveURLs(&buf, "http://localhost:8080", "", "observe_output/md")
		out := buf.String()
		assert.Contains(t, out, "Observe data at http://localhost:8080\n")
		assert.NotContains(t, out, "Target page at")
		assert.Contains(t, out, "Markdown files written to observe_output/md/\n")
	})

	t.Run("with initial path", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		printObserveURLs(&buf, "http://localhost:8080", "/owner/repo/pull_requests/123.html", "observe_output/md")
		out := buf.String()
		assert.Contains(t, out, "Observe data at http://localhost:8080\n")
		assert.Contains(t, out, "Target page at http://localhost:8080/owner/repo/pull_requests/123.html\n")
		assert.Contains(t, out, "Markdown files written to observe_output/md/\n")
	})
}
