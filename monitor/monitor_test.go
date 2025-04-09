package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestMonitorIntegration(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name           string
		opts           []Option
		monitorTime    time.Duration
		expectedOutput string
	}{
		{
			name:        "monitor all metrics",
			opts:        []Option{WithObserveInterval(250 * time.Millisecond)},
			monitorTime: time.Second,
		},
		{
			name: "monitor only CPU",
			opts: []Option{
				DisableMemory(),
				DisableDisk(),
				DisableIO(),
				WithObserveInterval(250 * time.Millisecond),
			},
			monitorTime: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				_, testDir  = testhelpers.Setup(t)
				outputFile  = filepath.Join(testDir, "monitor.json")
				ctx, cancel = context.WithTimeout(context.Background(), tt.monitorTime)
				err         error
			)

			t.Cleanup(cancel)

			// Start monitoring in a goroutine
			go func() {
				err = Start(ctx, append(tt.opts, WithOutputFile(outputFile))...)
			}()

			// Wait for the context to timeout
			<-ctx.Done()

			require.NoError(t, err, "error while monitoring")

			// Verify the output file exists and has content
			require.FileExists(t, outputFile, "monitor output file should exist")
			data, err := os.ReadFile(outputFile)
			require.NoError(t, err, "error reading monitor output file")
			require.NotEmpty(t, data, "monitor output file should not be empty")

			// Verify the content has expected log messages
			content := string(data)
			require.Contains(t, content, "Starting Monitoring", "should contain start message")
			require.Contains(t, content, "CPU System Info", "should contain CPU info")
			require.Contains(t, content, "System Memory Info", "should contain memory info")
			require.Contains(t, content, "System Disk Info", "should contain disk info")

			// Verify we have some observations
			require.Contains(t, content, "Observed CPU Usage", "should contain CPU observations")
			require.Contains(t, content, "Observed Memory Usage", "should contain memory observations")
			require.Contains(t, content, "Observed Disk Usage", "should contain disk observations")
			require.Contains(t, content, "Observed IO Usage", "should contain IO observations")
		})
	}
}
