package gather

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRunsOnCostSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		logs   string
		want   *RunsOnCostSummary
		wantOk bool
	}{
		{
			name: "full cost summary",
			logs: `Post job cleanup.
Running post-execution phase...
## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | m7i.4xlarge     |
| Instance Lifecycle     | spot            |
| Region                 | us-east-2       |
| Platform               | Linux/UNIX      |
| Arch                   | x64             |
| Az                     | us-east-2c      |
| Zone ID                | use2-az3        |
| Duration               | 6.25 minutes    |
| Cost                   | $0.0280         |
| GitHub equivalent cost | $0.2940         |
| Savings                | $0.2660 (90.5%) |
`,
			want: &RunsOnCostSummary{
				InstanceType:         "m7i.4xlarge",
				InstanceLifecycle:    "spot",
				Region:               "us-east-2",
				Platform:             "Linux/UNIX",
				Arch:                 "x64",
				Az:                   "us-east-2c",
				ZoneID:               "use2-az3",
				Duration:             "6.25 minutes",
				CostUSD:              0.0280,
				GitHubEquivalentCost: 0.2940,
				Savings:              "$0.2660 (90.5%)",
			},
			wantOk: true,
		},
		{
			name: "cost summary with extra log noise before and after",
			logs: `Some step output here
## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | c8g.large       |
| Instance Lifecycle     | on-demand       |
| Region                 | us-west-2       |
| Platform               | Linux/UNIX      |
| Arch                   | arm64           |
| Az                     | us-west-2a      |
| Zone ID                | usw2-az2        |
| Duration               | 2.5 minutes     |
| Cost                   | $0.0090         |
| GitHub equivalent cost | $0.0125         |
| Savings                | $0.0035 (28.0%) |

Some other output after`,
			want: &RunsOnCostSummary{
				InstanceType:         "c8g.large",
				InstanceLifecycle:    "on-demand",
				Region:               "us-west-2",
				Platform:             "Linux/UNIX",
				Arch:                 "arm64",
				Az:                   "us-west-2a",
				ZoneID:               "usw2-az2",
				Duration:             "2.5 minutes",
				CostUSD:              0.0090,
				GitHubEquivalentCost: 0.0125,
				Savings:              "$0.0035 (28.0%)",
			},
			wantOk: true,
		},
		{
			name: "no cost summary in logs",
			logs: `Just regular job output
No cost data here
## Some Other Summary
| thing | value |
`,
			wantOk: false,
		},
		{
			name:   "empty logs",
			logs:   "",
			wantOk: false,
		},
		{
			name: "cost summary missing some fields",
			logs: `## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | m7i.4xlarge     |
| Cost                   | $0.0280         |
`,
			want: &RunsOnCostSummary{
				InstanceType: "m7i.4xlarge",
				CostUSD:      0.0280,
			},
			wantOk: true,
		},
		{
			name: "cost summary with cost 0",
			logs: `## Execution Cost Summary

| metric                 | value           |
| ---------------------- | --------------- |
| Instance Type          | m7i.4xlarge     |
| Cost                   | $0.0000         |
`,
			want: &RunsOnCostSummary{
				InstanceType: "m7i.4xlarge",
				CostUSD:      0,
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseRunsOnCostSummary(tt.logs)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestRunsOnCostSummaryToTenthsOfCent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		usd  float64
		want int64
	}{
		{name: "$0.0280", usd: 0.0280, want: 28},
		{name: "$0.0090", usd: 0.0090, want: 9},
		{name: "$0.2940", usd: 0.2940, want: 294},
		{name: "$0.0010", usd: 0.0010, want: 1},
		{name: "$0.0000", usd: 0.0000, want: 0},
		{name: "$1.50", usd: 1.50, want: 1500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &RunsOnCostSummary{CostUSD: tt.usd}
			assert.Equal(t, tt.want, s.CostInTenthsOfCent())
		})
	}
}
