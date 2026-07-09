package monitor

import (
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCPUPercentsFromDelta(t *testing.T) {
	t.Parallel()

	prev := []cpu.TimesStat{
		{
			CPU:       "cpu0",
			User:      100,
			System:    50,
			Idle:      850,
			Iowait:    0,
			Irq:       0,
			Softirq:   0,
			Steal:     0,
			Guest:     0,
			GuestNice: 0,
		},
	}
	curr := []cpu.TimesStat{
		{
			CPU:       "cpu0",
			User:      200,
			System:    100,
			Idle:      900,
			Iowait:    0,
			Irq:       0,
			Softirq:   0,
			Steal:     0,
			Guest:     0,
			GuestNice: 0,
		},
	}

	percents, err := cpuPercentsFromDelta(prev, curr)
	require.NoError(t, err)
	require.Len(t, percents, 1)
	assert.InDelta(t, 75.0, percents[0], 0.01)
}

func TestCPUPercentsFromDelta_emptyPrev(t *testing.T) {
	t.Parallel()

	percents, err := cpuPercentsFromDelta(nil, []cpu.TimesStat{{CPU: "cpu0", Idle: 100}})
	require.NoError(t, err)
	assert.Nil(t, percents)
}

func TestIODeltasFromCounters(t *testing.T) {
	t.Parallel()

	prev := []net.IOCountersStat{
		{Name: "eth0", BytesSent: 1000, BytesRecv: 2000, PacketsSent: 10, PacketsRecv: 20},
	}
	curr := []net.IOCountersStat{
		{Name: "eth0", BytesSent: 1500, BytesRecv: 2600, PacketsSent: 15, PacketsRecv: 26},
	}

	deltas, err := ioDeltasFromCounters(prev, curr)
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, uint64(500), deltas[0].BytesSent)
	assert.Equal(t, uint64(600), deltas[0].BytesRecv)
	assert.Equal(t, uint64(5), deltas[0].PacketsSent)
	assert.Equal(t, uint64(6), deltas[0].PacketsRecv)
}

func TestIODeltasFromCounters_emptyPrev(t *testing.T) {
	t.Parallel()

	deltas, err := ioDeltasFromCounters(nil, []net.IOCountersStat{{Name: "eth0"}})
	require.NoError(t, err)
	assert.Nil(t, deltas)
}
