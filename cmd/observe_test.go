package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kalverra/octometrics/internal/config"
)

func TestBuildObserveOptions(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DataDir:          "/tmp/custom-data-dir",
		ExcludeCosts:     true,
		ExcludeWorkflows: []string{"test-wf"},
	}

	opts := buildObserveOptions(cfg)
	assert.NotEmpty(t, opts)
}

func TestBuildObserveOptions_IncludeWorkflows(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DataDir:          "/tmp/custom-data-dir",
		IncludeWorkflows: []string{"included-wf"},
	}

	opts := buildObserveOptions(cfg)
	assert.NotEmpty(t, opts)
}
