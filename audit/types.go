package audit

import (
	"time"

	"github.com/kalverra/octometrics/gather"
)

// Options holds configuration for running a workflow audit.
type Options struct {
	Owner               string
	Repo                string
	Workflow            string  // filename or ID
	RunsCount           int     // default 10
	IncludeRuns         []int64 // specific run IDs to include
	ExcludeRuns         []int64 // specific run IDs to exclude
	Branch              string
	Status              string // completed, success, etc.
	DataDir             string
	Format              string  // html, md, table, json
	AIOutput            bool    // terminal-first output for AI / scripts
	CPUThresholdPercent float64 // default 30.0%
	RAMThresholdPercent float64 // default 50.0%
}

// WorkflowAudit contains the complete results of a multi-run workflow audit.
type WorkflowAudit struct {
	Workflow        WorkflowInfo     `json:"workflow"`
	SampleSize      int              `json:"sample_size"`
	DateRange       DateRange        `json:"date_range"`
	Summary         RunSummaryStats  `json:"summary"`
	Jobs            []JobAudit       `json:"jobs"`
	Recommendations []Recommendation `json:"recommendations"`
	SampledRuns     []SampledRun     `json:"sampled_runs"`
}

// WorkflowInfo describes the target workflow.
type WorkflowInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	HTMLURL string `json:"html_url,omitempty"`
}

// DateRange holds the earliest and latest sampled run timestamps.
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// RunSummaryStats aggregates overall statistics across all sampled runs.
type RunSummaryStats struct {
	TotalRuns        int           `json:"total_runs"`
	SuccessRuns      int           `json:"success_runs"`
	FailedRuns       int           `json:"failed_runs"`
	SuccessRate      float64       `json:"success_rate"`
	AvgDuration      time.Duration `json:"avg_duration"`
	MinDuration      time.Duration `json:"min_duration"`
	MaxDuration      time.Duration `json:"max_duration"`
	P95Duration      time.Duration `json:"p95_duration"`
	TotalCostUSD     float64       `json:"total_cost_usd"`
	AvgCostPerRunUSD float64       `json:"avg_cost_per_run_usd"`
}

// JobAudit contains aggregated metrics and recommendations for a single workflow job.
type JobAudit struct {
	Name             string              `json:"name"`
	RunCount         int                 `json:"run_count"`
	AvgDuration      time.Duration       `json:"avg_duration"`
	MinDuration      time.Duration       `json:"min_duration"`
	MaxDuration      time.Duration       `json:"max_duration"`
	P95Duration      time.Duration       `json:"p95_duration"`
	TotalCostUSD     float64             `json:"total_cost_usd"`
	AvgCostPerRunUSD float64             `json:"avg_cost_per_run_usd"`
	Runner           string              `json:"runner"`
	RunnerSpecs      gather.RunnerSpecs  `json:"runner_specs"`
	Utilization      ResourceUtilization `json:"utilization"`
	Recommendations  []Recommendation    `json:"recommendations,omitempty"`
}

// ResourceUtilization represents aggregated resource metrics for a job across runs.
type ResourceUtilization struct {
	CPUCoresProvisioned int     `json:"cpu_cores_provisioned,omitempty"`
	CPUAvgPercent       float64 `json:"cpu_avg_percent,omitempty"`
	CPUPeakPercent      float64 `json:"cpu_peak_percent,omitempty"`
	CPULoad1mAvg        float64 `json:"cpu_load_1m_avg,omitempty"`
	CPULoad1mPeak       float64 `json:"cpu_load_1m_peak,omitempty"`
	CPULoad5mAvg        float64 `json:"cpu_load_5m_avg,omitempty"`
	CPULoad5mPeak       float64 `json:"cpu_load_5m_peak,omitempty"`

	RAMProvisionedGB  float64 `json:"ram_provisioned_gb,omitempty"`
	MemoryAvgPercent  float64 `json:"memory_avg_percent,omitempty"`
	MemoryPeakPercent float64 `json:"memory_peak_percent,omitempty"`
	MemoryAvgGB       float64 `json:"memory_avg_gb,omitempty"`
	MemoryPeakGB      float64 `json:"memory_peak_gb,omitempty"`

	NetworkIOAvgMB  float64 `json:"network_io_avg_mb,omitempty"`
	NetworkIOPeakMB float64 `json:"network_io_peak_mb,omitempty"`

	Source string `json:"source"` // "runs-on", "octometrics-action", "none"
}

// RecommendationType classifies the optimization advisory.
type RecommendationType string

// Supported recommendation categories.
const (
	// RecommendationTypeRunnerRightsizing advises changing vCPU/RAM profile.
	RecommendationTypeRunnerRightsizing RecommendationType = "runner_rightsizing"
	// RecommendationTypeCostOptimization advises switching to spot or cheaper architecture.
	RecommendationTypeCostOptimization RecommendationType = "cost_optimization"
	// RecommendationTypePerformanceBottleneck advises optimizing jobs dominating workflow runtime.
	RecommendationTypePerformanceBottleneck RecommendationType = "performance_bottleneck"
)

// Severity classifies the urgency or impact of the recommendation.
type Severity string

// Supported recommendation severities.
const (
	// SeverityHigh marks significant savings (>40%) or severe OOM risk.
	SeverityHigh Severity = "HIGH"
	// SeverityMedium marks moderate savings or potential bottlenecks.
	SeverityMedium Severity = "MEDIUM"
	// SeverityLow marks minor adjustments.
	SeverityLow Severity = "LOW"
	// SeverityInfo marks informational suggestions like architecture evaluation.
	SeverityInfo Severity = "INFO"
)

// Recommendation represents a specific actionable suggestion.
type Recommendation struct {
	Type                      RecommendationType `json:"type"`
	Severity                  Severity           `json:"severity"`
	JobName                   string             `json:"job_name,omitempty"`
	Title                     string             `json:"title"`
	Reason                    string             `json:"reason"`
	CurrentConfig             string             `json:"current_config,omitempty"`
	RecommendedConfig         string             `json:"recommended_config,omitempty"`
	ActionableYAML            string             `json:"actionable_yaml,omitempty"`
	EstimatedSavingsPercent   float64            `json:"estimated_savings_percent,omitempty"`
	EstimatedSavingsPerRunUSD float64            `json:"estimated_savings_per_run_usd,omitempty"`
}

// SampledRun contains high-level information for each sampled run.
type SampledRun struct {
	ID         int64         `json:"id"`
	Status     string        `json:"status"`
	Conclusion string        `json:"conclusion"`
	Branch     string        `json:"branch"`
	Duration   time.Duration `json:"duration"`
	CostUSD    float64       `json:"cost_usd"`
	CreatedAt  time.Time     `json:"created_at"`
	HTMLURL    string        `json:"html_url"`
}
