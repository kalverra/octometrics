package monitor

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/kalverra/octometrics/internal/logging"
)

// Option mutates how monitoring is done
type Option func(*options)

// WithOutputFile sets a custom output file for monitoring data
func WithOutputFile(outputFile string) Option {
	return func(opts *options) {
		opts.OutputFile = outputFile
	}
}

// WithObserveInterval sets the interval at which to observe system resources
func WithObserveInterval(interval time.Duration) Option {
	return func(opts *options) {
		opts.ObserveInterval = interval
	}
}

// DisableCPU disables CPU monitoring
func DisableCPU() Option {
	return func(opts *options) {
		opts.MonitorCPU = false
	}
}

// DisableMemory disables memory monitoring
func DisableMemory() Option {
	return func(opts *options) {
		opts.MonitorMemory = false
	}
}

// DisableDisk disables disk monitoring
func DisableDisk() Option {
	return func(opts *options) {
		opts.MonitorDisk = false
	}
}

// DisableIO disables IO monitoring
func DisableIO() Option {
	return func(opts *options) {
		opts.MonitorIO = false
	}
}

// WithDiskPath sets the filesystem path monitored for disk usage.
func WithDiskPath(path string) Option {
	return func(opts *options) {
		opts.DiskPath = path
	}
}

type options struct {
	OutputFile      string
	ObserveInterval time.Duration
	MonitorCPU      bool
	MonitorMemory   bool
	MonitorDisk     bool
	MonitorIO       bool
	DiskPath        string
	prevCPUTimes    []cpu.TimesStat
	prevIOStats     []net.IOCountersStat
}

func defaultOptions() *options {
	return &options{
		OutputFile:      DataFile,
		ObserveInterval: time.Second,
		MonitorCPU:      true,
		MonitorMemory:   true,
		MonitorDisk:     true,
		MonitorIO:       true,
		DiskPath:        defaultDiskPath(),
	}
}

// monitorEntry represents a single entry in the monitor data file,
// a zerolog log entry with additional fields for system monitoring data.
type monitorEntry struct {
	// General values
	Time        logTime  `json:"time"`
	Message     string   `json:"message"`
	Level       string   `json:"level"`
	Total       *uint64  `json:"total,omitempty"`
	Used        *uint64  `json:"used,omitempty"`
	Available   *uint64  `json:"available,omitempty"`
	UsedPercent *float64 `json:"used_percent,omitempty"`

	// CPU specific values
	Num       *int     `json:"num,omitempty"`
	Model     *string  `json:"model,omitempty"`
	Vendor    *string  `json:"vendor,omitempty"`
	Family    *string  `json:"family,omitempty"`
	CacheSize *int32   `json:"cache_size,omitempty"`
	Cores     *int32   `json:"cores,omitempty"`
	Mhz       *float64 `json:"mhz,omitempty"`

	// IO specific values
	BytesSent   *uint64 `json:"bytes_sent,omitempty"`
	BytesRecv   *uint64 `json:"bytes_recv,omitempty"`
	PacketsSent *uint64 `json:"packets_sent,omitempty"`
	PacketsRecv *uint64 `json:"packets_recv,omitempty"`

	// GitHub Actions Environment Variables
	// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/store-information-in-variables#default-environment-variables
	GitHubActionsEnvVars *githubActionsEnvVars `json:"github_actions_env_vars,omitempty"`
}

// logTime is a custom time type for parsing log timestamps.
type logTime struct {
	time.Time
}

// Tracks GitHub Actions environment variables
// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/store-information-in-variables#default-environment-variables
type githubActionsEnvVars struct {
	Action           string `env:"GITHUB_ACTION"            json:"GITHUB_ACTION,omitempty"`
	ActionPath       string `env:"GITHUB_ACTION_PATH"       json:"GITHUB_ACTION_PATH,omitempty"`
	ActionRepository string `env:"GITHUB_ACTION_REPOSITORY" json:"GITHUB_ACTION_REPOSITORY,omitempty"`
	Actor            string `env:"GITHUB_ACTOR"             json:"GITHUB_ACTOR,omitempty"`
	ActorID          string `env:"GITHUB_ACTOR_ID"          json:"GITHUB_ACTOR_ID,omitempty"`
	APIURL           string `env:"GITHUB_API_URL"           json:"GITHUB_API_URL,omitempty"`
	BaseRef          string `env:"GITHUB_BASE_REF"          json:"GITHUB_BASE_REF,omitempty"`
	Env              string `env:"GITHUB_ENV"               json:"GITHUB_ENV,omitempty"`
	EventName        string `env:"GITHUB_EVENT_NAME"        json:"GITHUB_EVENT_NAME,omitempty"`
	EventPath        string `env:"GITHUB_EVENT_PATH"        json:"GITHUB_EVENT_PATH,omitempty"`
	GraphQLURL       string `env:"GITHUB_GRAPHQL_URL"       json:"GITHUB_GRAPHQL_URL,omitempty"`
	HeadRef          string `env:"GITHUB_HEAD_REF"          json:"GITHUB_HEAD_REF,omitempty"`
	// Job is the github-context job_id. This in no way matches to the numerical Job ID returned by the API, nor the name of the job.
	Job string `env:"GITHUB_JOB" json:"GITHUB_JOB,omitempty"`
	// JobName is a custom env var set by octometrics-action and describes the name of the job on the runner so we can match it with the API.
	// There is currently no native way to do this in GitHub Actions.
	// https://github.com/actions/toolkit/issues/550
	JobName           string `env:"GITHUB_JOB_NAME"            json:"GITHUB_JOB_NAME,omitempty"`
	Output            string `env:"GITHUB_OUTPUT"              json:"GITHUB_OUTPUT,omitempty"`
	Path              string `env:"GITHUB_PATH"                json:"GITHUB_PATH,omitempty"`
	Ref               string `env:"GITHUB_REF"                 json:"GITHUB_REF,omitempty"`
	RefName           string `env:"GITHUB_REF_NAME"            json:"GITHUB_REF_NAME,omitempty"`
	Repository        string `env:"GITHUB_REPOSITORY"          json:"GITHUB_REPOSITORY,omitempty"`
	RepositoryID      string `env:"GITHUB_REPOSITORY_ID"       json:"GITHUB_REPOSITORY_ID,omitempty"`
	RepositoryOwner   string `env:"GITHUB_REPOSITORY_OWNER"    json:"GITHUB_REPOSITORY_OWNER,omitempty"`
	RepositoryOwnerID string `env:"GITHUB_REPOSITORY_OWNER_ID" json:"GITHUB_REPOSITORY_OWNER_ID,omitempty"`
	RetentionDays     string `env:"GITHUB_RETENTION_DAYS"      json:"GITHUB_RETENTION_DAYS,omitempty"`
	RunAttempt        string `env:"GITHUB_RUN_ATTEMPT"         json:"GITHUB_RUN_ATTEMPT,omitempty"`
	// RunID refers to the workflow run ID
	RunID       int64  `env:"GITHUB_RUN_ID"       json:"GITHUB_RUN_ID,omitempty"`
	RunNumber   int    `env:"GITHUB_RUN_NUMBER"   json:"GITHUB_RUN_NUMBER,omitempty"`
	ServerURL   string `env:"GITHUB_SERVER_URL"   json:"GITHUB_SERVER_URL,omitempty"`
	SHA         string `env:"GITHUB_SHA"          json:"GITHUB_SHA,omitempty"`
	StepSummary string `env:"GITHUB_STEP_SUMMARY" json:"GITHUB_STEP_SUMMARY,omitempty"`
	// Token isn't guaranteed to be set as an env var, but it's a standard process, especially for the octometrics-action.
	Token             string `env:"GITHUB_TOKEN"            json:"GITHUB_TOKEN,omitempty"`
	TriggeringActor   string `env:"GITHUB_TRIGGERING_ACTOR" json:"GITHUB_TRIGGERING_ACTOR,omitempty"`
	Workflow          string `env:"GITHUB_WORKFLOW"         json:"GITHUB_WORKFLOW,omitempty"`
	WorkflowRef       string `env:"GITHUB_WORKFLOW_REF"     json:"GITHUB_WORKFLOW_REF,omitempty"`
	WorkflowSHA       string `env:"GITHUB_WORKFLOW_SHA"     json:"GITHUB_WORKFLOW_SHA,omitempty"`
	Workspace         string `env:"GITHUB_WORKSPACE"        json:"GITHUB_WORKSPACE,omitempty"`
	RunnerArch        string `env:"RUNNER_ARCH"             json:"RUNNER_ARCH,omitempty"`
	RunnerDebug       string `env:"RUNNER_DEBUG"            json:"RUNNER_DEBUG,omitempty"`
	RunnerEnvironment string `env:"RUNNER_ENVIRONMENT"      json:"RUNNER_ENVIRONMENT,omitempty"`
	RunnerName        string `env:"RUNNER_NAME"             json:"RUNNER_NAME,omitempty"`
	RunnerOS          string `env:"RUNNER_OS"               json:"RUNNER_OS,omitempty"`
	RunnerTemp        string `env:"RUNNER_TEMP"             json:"RUNNER_TEMP,omitempty"`
	RunnerToolCache   string `env:"RUNNER_TOOL_CACHE"       json:"RUNNER_TOOL_CACHE,omitempty"`
}

func collectGitHubActionsEnvVars() (*githubActionsEnvVars, error) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return nil, nil
	}

	var envVars githubActionsEnvVars
	if err := env.Parse(&envVars); err != nil {
		return nil, fmt.Errorf("unable to parse GitHub Actions environment variables: %w", err)
	}

	return &envVars, nil
}

// UnmarshalJSON parses the custom time format from the log entry.
func (l *logTime) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = s[1 : len(s)-1]

	// Parse the string using the custom layout
	t, err := time.Parse(logging.TimeLayout, s)
	if err != nil {
		return fmt.Errorf("unable to parse time: %w", err)
	}
	l.Time = t
	return nil
}

func (m *monitorEntry) GetLevel() string {
	if m == nil {
		return "UNKNOWN"
	}
	return m.Level
}

func (m *monitorEntry) GetMessage() string {
	if m == nil {
		return ""
	}
	return m.Message
}

func (m *monitorEntry) GetTime() time.Time {
	if m == nil {
		return time.Time{}
	}
	return m.Time.Time
}

func (m *monitorEntry) GetNum() int {
	if m == nil || m.Num == nil {
		return 0
	}
	return *m.Num
}

func (m *monitorEntry) GetModel() string {
	if m == nil || m.Model == nil {
		return ""
	}
	return *m.Model
}

func (m *monitorEntry) GetVendor() string {
	if m == nil || m.Vendor == nil {
		return ""
	}
	return *m.Vendor
}

func (m *monitorEntry) GetFamily() string {
	if m == nil || m.Family == nil {
		return ""
	}
	return *m.Family
}

func (m *monitorEntry) GetCacheSize() int32 {
	if m == nil || m.CacheSize == nil {
		return 0
	}
	return *m.CacheSize
}

func (m *monitorEntry) GetCores() int32 {
	if m == nil || m.Cores == nil {
		return 0
	}
	return *m.Cores
}

func (m *monitorEntry) GetMhz() float64 {
	if m == nil || m.Mhz == nil {
		return 0
	}
	return *m.Mhz
}

func (m *monitorEntry) GetTotal() uint64 {
	if m == nil || m.Total == nil {
		return 0
	}
	return *m.Total
}

func (m *monitorEntry) GetUsed() uint64 {
	if m == nil || m.Used == nil {
		return 0
	}
	return *m.Used
}

func (m *monitorEntry) GetAvailable() uint64 {
	if m == nil || m.Available == nil {
		return 0
	}
	return *m.Available
}

func (m *monitorEntry) GetUsedPercent() float64 {
	if m == nil || m.UsedPercent == nil {
		return 0
	}
	return *m.UsedPercent
}

func (m *monitorEntry) GetBytesSent() uint64 {
	if m == nil || m.BytesSent == nil {
		return 0
	}
	return *m.BytesSent
}

func (m *monitorEntry) GetBytesRecv() uint64 {
	if m == nil || m.BytesRecv == nil {
		return 0
	}
	return *m.BytesRecv
}

func (m *monitorEntry) GetPacketsSent() uint64 {
	if m == nil || m.PacketsSent == nil {
		return 0
	}
	return *m.PacketsSent
}

func (m *monitorEntry) GetPacketsRecv() uint64 {
	if m == nil || m.PacketsRecv == nil {
		return 0
	}
	return *m.PacketsRecv
}
