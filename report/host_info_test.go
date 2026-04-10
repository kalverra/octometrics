package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kalverra/octometrics/monitor"
)

func TestMachineInfoMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, machineInfoMarkdown(nil))
	})

	t.Run("cpu memory disk", func(t *testing.T) {
		t.Parallel()
		md := machineInfoMarkdown(&monitor.Analysis{
			SystemInfo: &monitor.SystemInfo{
				CPU: []*monitor.SystemCPUInfo{
					{Num: 0, Model: "Test CPU", Mhz: 2400},
					{Num: 1, Model: "Test CPU", Mhz: 2400},
				},
				Memory: &monitor.SystemMemoryInfo{Total: 16 * 1024 * 1024 * 1024},
				Disk:   &monitor.SystemDiskInfo{Total: 500 * 1024 * 1024 * 1024},
			},
		})
		require.NotEmpty(t, md)
		assert.Contains(t, md, "### Host")
		assert.Contains(t, md, "2 logical processor")
		assert.Contains(t, md, "Test CPU")
		assert.Contains(t, md, "2400 MHz")
		assert.Contains(t, md, "RAM")
		assert.Contains(t, md, "Disk")
	})
}
