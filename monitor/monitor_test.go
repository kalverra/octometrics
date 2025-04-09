package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func TestMonitorIntegration(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	defaultObserverInterval := 250 * time.Millisecond

	tests := []struct {
		name           string
		opts           []Option
		monitorTime    time.Duration
		expectedOutput string
	}{
		{
			name:        "monitor all metrics",
			opts:        []Option{WithObserveInterval(defaultObserverInterval)},
			monitorTime: time.Second,
		},
		{
			name: "monitor only CPU",
			opts: []Option{
				DisableMemory(),
				DisableDisk(),
				DisableIO(),
				WithObserveInterval(defaultObserverInterval),
			},
			monitorTime: time.Second,
		},
		{
			name: "monitor only memory",
			opts: []Option{
				DisableCPU(),
				DisableDisk(),
				DisableIO(),
				WithObserveInterval(defaultObserverInterval),
			},
			monitorTime: time.Second,
		},
		{
			name: "monitor only disk",
			opts: []Option{
				DisableCPU(),
				DisableMemory(),
				DisableIO(),
				WithObserveInterval(defaultObserverInterval),
			},
			monitorTime: time.Second,
		},
		{
			name: "monitor only IO",
			opts: []Option{
				DisableCPU(),
				DisableMemory(),
				DisableDisk(),
				WithObserveInterval(defaultObserverInterval),
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

			startTime := time.Now()
			// Start monitoring in a goroutine
			go func() {
				err = Start(ctx, append(tt.opts, WithOutputFile(outputFile))...)
			}()

			// Wait for the context to timeout
			<-ctx.Done()

			elapsedSeconds := time.Since(startTime).Truncate(time.Second).Seconds()
			require.LessOrEqual(
				t,
				elapsedSeconds,
				tt.monitorTime.Seconds(),
				"monitor should have finished before or at the same time as the context timeout",
			)
			require.NoError(t, err, "error while monitoring")

			// Verify the output file exists and has content
			require.FileExists(t, outputFile, "monitor output file should exist")
			data, err := os.ReadFile(outputFile)
			require.NoError(t, err, "error reading monitor output file")
			require.NotEmpty(t, data, "monitor output file should not be empty")

			// Verify the content has expected log messages
			content := string(data)
			require.Contains(t, content, "Starting Monitoring", "should contain start message")

			// Create default options to check which metrics are enabled
			opts := defaultOptions()
			for _, opt := range tt.opts {
				opt(opts)
			}

			// Check that basic system info is logged
			assert.Contains(t, content, CPUSystemInfoMsg, "should contain CPU system info")
			assert.Contains(t, content, MemSystemInfoMsg, "should contain memory system info")
			assert.Contains(t, content, DiskSystemInfoMsg, "should contain disk system info")

			// Only assert metrics that are enabled
			if opts.MonitorCPU {

				assert.Contains(t, content, ObservedCPUMsg, "should contain CPU observations")
			} else {
				assert.NotContains(t, content, ObservedCPUMsg, "should not contain CPU observations")
			}

			if opts.MonitorMemory {
				assert.Contains(t, content, ObservedMemMsg, "should contain memory observations")
			} else {
				assert.NotContains(t, content, ObservedMemMsg, "should not contain memory observations")
			}

			if opts.MonitorDisk {
				assert.Contains(t, content, ObservedDiskMsg, "should contain disk observations")
			} else {
				assert.NotContains(t, content, ObservedDiskMsg, "should not contain disk observations")
			}

			if opts.MonitorIO {
				assert.Contains(t, content, ObservedIOMsg, "should contain IO observations")
			} else {
				assert.NotContains(t, content, ObservedIOMsg, "should not contain IO observations")
			}
		})
	}
}
