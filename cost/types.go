package cost

import "time"

// RunnerType represents whether a runner is GitHub-hosted or Runs-On (self-hosted).
type RunnerType string

const (
	// RunnerTypeGHANative indicates GitHub-hosted runners.
	RunnerTypeGHANative RunnerType = "GHA native"
	// RunnerTypeRunsOn indicates Runs-On (self-hosted EC2) runners.
	RunnerTypeRunsOn RunnerType = "runs-on"
)

// UsageRecord represents a single row from a GitHub Actions billing usage report CSV.
type UsageRecord struct {
	Date               time.Time
	Product            string
	SKU                string
	Quantity           float64
	UnitType           string
	AppliedCostPerUnit float64
	GrossAmount        float64
	DiscountAmount     float64
	NetAmount          float64
	Username           string
	Organization       string
	Repository         string
	WorkflowPath       string
	RunnerType         RunnerType
}

// RunnerBreakdown contains aggregated metrics for a runner SKU or instance type.
type RunnerBreakdown struct {
	RunnerName    string     `json:"runner_name"`
	RunnerType    RunnerType `json:"runner_type"`
	Minutes       float64    `json:"minutes"`
	GrossAmount   float64    `json:"gross_amount"`
	NetAmount     float64    `json:"net_amount"`
	EstimatedCost float64    `json:"estimated_cost"`
	WorkflowCount int        `json:"workflow_count"`
	RepoCount     int        `json:"repo_count"`
}

// RepoBreakdown contains aggregated metrics for a single repository.
type RepoBreakdown struct {
	Repository    string   `json:"repository"`
	GHAMinutes    float64  `json:"gha_minutes"`
	GHANetCost    float64  `json:"gha_net_cost"`
	GHAGrossCost  float64  `json:"gha_gross_cost"`
	RunsOnMinutes float64  `json:"runs_on_minutes"`
	RunsOnCost    float64  `json:"runs_on_cost"`
	TotalMinutes  float64  `json:"total_minutes"`
	TotalCost     float64  `json:"total_cost"`
	Workflows     []string `json:"workflows,omitempty"`
}

// WorkflowBreakdown contains aggregated metrics for a single workflow.
type WorkflowBreakdown struct {
	Repository    string     `json:"repository"`
	WorkflowPath  string     `json:"workflow_path"`
	Runner        string     `json:"runner"`
	RunnerType    RunnerType `json:"runner_type"`
	Minutes       float64    `json:"minutes"`
	NetAmount     float64    `json:"net_amount"`
	GrossAmount   float64    `json:"gross_amount"`
	EstimatedCost float64    `json:"estimated_cost"`
}

// SavingsEstimate models the cost difference of migrating GHA native workflows to Runs-On.
type SavingsEstimate struct {
	SKU                 string  `json:"sku"`
	EquivalentRunner    string  `json:"equivalent_runner"`
	Minutes             float64 `json:"minutes"`
	CurrentNetCost      float64 `json:"current_net_cost"`
	CurrentGrossCost    float64 `json:"current_gross_cost"`
	EstimatedRunsOnCost float64 `json:"estimated_runs_on_cost"`
	EstimatedSavings    float64 `json:"estimated_savings"`
	SavingsPercent      float64 `json:"savings_percent"`
}

// Report holds the complete cost and savings analysis results.
type Report struct {
	DateStart             time.Time                       `json:"date_start"`
	DateEnd               time.Time                       `json:"date_end"`
	Organization          string                          `json:"organization"`
	TotalMinutes          float64                         `json:"total_minutes"`
	TotalGHANetCost       float64                         `json:"total_gha_net_cost"`
	TotalGHAGrossCost     float64                         `json:"total_gha_gross_cost"`
	TotalRunsOnCost       float64                         `json:"total_runs_on_cost"`
	TotalComputeCost      float64                         `json:"total_compute_cost"`
	ByRunnerType          map[RunnerType]*RunnerBreakdown `json:"by_runner_type"`
	ByRunner              []*RunnerBreakdown              `json:"by_runner"`
	ByRepo                []*RepoBreakdown                `json:"by_repo"`
	ByWorkflow            []*WorkflowBreakdown            `json:"by_workflow"`
	Savings               []*SavingsEstimate              `json:"savings"`
	TotalProjectedSavings float64                         `json:"total_projected_savings"`
	CloudzeroExact        bool                            `json:"cloudzero_exact"`
	UntaggedRunsOnCost    float64                         `json:"untagged_runs_on_cost,omitempty"`
}
