package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatGatherTitle(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Gathering data (0s)", formatGatherTitle(0*time.Second))
	assert.Equal(t, "Gathering data (2s)", formatGatherTitle(2*time.Second))
	assert.Equal(t, "Gathering data (1m5s)", formatGatherTitle(65*time.Second))
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "3.67s", formatDuration(3669844167*time.Nanosecond))
	assert.Equal(t, "16.9s", formatDuration(16899200959*time.Nanosecond))
	assert.Equal(t, "2s", formatDuration(2*time.Second))
}
