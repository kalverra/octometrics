package gather

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRunsOnLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		labels  []string
		wantKey string
		wantOk  bool
	}{
		{
			name: "real runs-on format with cpu and family",
			labels: []string{
				"runs-on=30918328545-inmem-compile/cpu=32/ram=64/family=c7i+c8i/spot=co/volume=100GB/extras=s3-cache",
			},
			wantKey: "32cpu-linux-x64",
			wantOk:  true,
		},
		{
			name: "real runs-on format spot=false",
			labels: []string{
				"runs-on=30918328545-0-1/cpu=8/ram=32/family=m6i+m5.*/spot=false/image=ubuntu24-full-x64/extras=s3-cache+tmpfs",
			},
			wantKey: "8cpu-linux-x64",
			wantOk:  true,
		},
		{
			name:    "simple Ncpu-linux-x64 format from docs",
			labels:  []string{"2cpu-linux-x64"},
			wantKey: "2cpu-linux-x64",
			wantOk:  true,
		},
		{
			name:    "runner= prefix format from docs",
			labels:  []string{"runner=2cpu-linux-x64"},
			wantKey: "2cpu-linux-x64",
			wantOk:  true,
		},
		{
			name:    "mixed labels with runs-on",
			labels:  []string{"self-hosted", "runs-on=123/cpu=4/ram=16/family=c7i/spot=true"},
			wantKey: "4cpu-linux-x64",
			wantOk:  true,
		},
		{
			name:   "ubuntu-latest (not runs-on)",
			labels: []string{"ubuntu-latest"},
			wantOk: false,
		},
		{
			name:   "self-hosted only",
			labels: []string{"self-hosted", "linux"},
			wantOk: false,
		},
		{
			name:   "empty labels",
			labels: []string{},
			wantOk: false,
		},
		{
			name:   "nil labels",
			labels: nil,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, ok := parseRunsOnLabel(tt.labels)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantKey, key)
			}
		})
	}
}

func TestRunsOnRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		wantRate int64
		wantOk   bool
	}{
		{
			name:     "2cpu linux x64",
			key:      "2cpu-linux-x64",
			wantRate: 10,
			wantOk:   true,
		},
		{
			name:     "4cpu linux x64",
			key:      "4cpu-linux-x64",
			wantRate: 17,
			wantOk:   true,
		},
		{
			name:   "unknown size",
			key:    "100cpu-linux-x64",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rate, ok := runsOnRate(tt.key)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantRate, rate)
			}
		})
	}
}

func TestCalculateRunsOnCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		labels       []string
		duration     time.Duration
		wantCost     int64
		wantEstimate bool
	}{
		{
			name:         "2cpu linux x64 for 5 minutes",
			labels:       []string{"2cpu-linux-x64"},
			duration:     5 * time.Minute,
			wantCost:     50,
			wantEstimate: true,
		},
		{
			name:         "4cpu linux x64 for 30 seconds",
			labels:       []string{"4cpu-linux-x64"},
			duration:     30 * time.Second,
			wantCost:     9,
			wantEstimate: true,
		},
		{
			name:         "real runs-on format falls back to estimate with cpu count",
			labels:       []string{"runs-on=123/cpu=4/ram=16/family=c7i/spot=true"},
			duration:     10 * time.Minute,
			wantCost:     170,
			wantEstimate: true,
		},
		{
			name:         "no runs-on label",
			labels:       []string{"ubuntu-latest"},
			duration:     5 * time.Minute,
			wantCost:     0,
			wantEstimate: false,
		},
		{
			name:         "unknown size",
			labels:       []string{"100cpu-linux-x64"},
			duration:     5 * time.Minute,
			wantCost:     0,
			wantEstimate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cost, estimate := calculateRunsOnCost(tt.labels, tt.duration)
			assert.Equal(t, tt.wantCost, cost)
			assert.Equal(t, tt.wantEstimate, estimate)
		})
	}
}

func TestRunsOnCostIntegration(t *testing.T) {
	t.Parallel()

	labels := []string{"runner=2cpu-linux-x64"}
	duration := 10 * time.Minute

	cost, estimate := calculateRunsOnCost(labels, duration)
	require.True(t, estimate)
	assert.Equal(t, int64(100), cost)
}
