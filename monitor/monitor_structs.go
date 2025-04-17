package monitor

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/kalverra/octometrics/logging"
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

// DisableProcesses disables process monitoring
func DisableProcesses() Option {
	return func(opts *options) {
		opts.MonitorProcesses = false
	}
}

type options struct {
	OutputFile                  string
	ObserveInterval             time.Duration
	MonitorCPU                  bool
	MonitorMemory               bool
	MonitorDisk                 bool
	MonitorIO                   bool
	MonitorProcesses            bool
	MonitorGitHubActionsEnvVars bool
}

func defaultOptions() *options {
	return &options{
		OutputFile:                  DataFile,
		ObserveInterval:             time.Second,
		MonitorCPU:                  true,
		MonitorMemory:               true,
		MonitorDisk:                 true,
		MonitorIO:                   true,
		MonitorProcesses:            false,
		MonitorGitHubActionsEnvVars: true,
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
	Action           string `json:"GITHUB_ACTION,omitempty"              env:"GITHUB_ACTION"`
	ActionPath       string `json:"GITHUB_ACTION_PATH,omitempty"         env:"GITHUB_ACTION_PATH"`
	ActionRepository string `json:"GITHUB_ACTION_REPOSITORY,omitempty"   env:"GITHUB_ACTION_REPOSITORY"`
	Actor            string `json:"GITHUB_ACTOR,omitempty"               env:"GITHUB_ACTOR"`
	ActorID          string `json:"GITHUB_ACTOR_ID,omitempty"            env:"GITHUB_ACTOR_ID"`
	APIURL           string `json:"GITHUB_API_URL,omitempty"             env:"GITHUB_API_URL"`
	BaseRef          string `json:"GITHUB_BASE_REF,omitempty"            env:"GITHUB_BASE_REF"`
	Env              string `json:"GITHUB_ENV,omitempty"                 env:"GITHUB_ENV"`
	EventName        string `json:"GITHUB_EVENT_NAME,omitempty"          env:"GITHUB_EVENT_NAME"`
	EventPath        string `json:"GITHUB_EVENT_PATH,omitempty"          env:"GITHUB_EVENT_PATH"`
	GraphQLURL       string `json:"GITHUB_GRAPHQL_URL,omitempty"         env:"GITHUB_GRAPHQL_URL"`
	HeadRef          string `json:"GITHUB_HEAD_REF,omitempty"            env:"GITHUB_HEAD_REF"`
	// Job is the github-context job_id. This in no way matches to the numerical Job ID returned by the API, nor the name of the job.
	Job string `json:"GITHUB_JOB,omitempty"                 env:"GITHUB_JOB"`
	// JobName is a custom env var set by octometrics-action and describes the name of the job on the runner so we can match it with the API.
	// There is currently no native way to do this in GitHub Actions.
	// https://github.com/actions/toolkit/issues/550
	JobName           string `json:"GITHUB_JOB_NAME,omitempty"            env:"GITHUB_JOB_NAME"`
	Output            string `json:"GITHUB_OUTPUT,omitempty"              env:"GITHUB_OUTPUT"`
	Path              string `json:"GITHUB_PATH,omitempty"                env:"GITHUB_PATH"`
	Ref               string `json:"GITHUB_REF,omitempty"                 env:"GITHUB_REF"`
	RefName           string `json:"GITHUB_REF_NAME,omitempty"            env:"GITHUB_REF_NAME"`
	Repository        string `json:"GITHUB_REPOSITORY,omitempty"          env:"GITHUB_REPOSITORY"`
	RepositoryID      string `json:"GITHUB_REPOSITORY_ID,omitempty"       env:"GITHUB_REPOSITORY_ID"`
	RepositoryOwner   string `json:"GITHUB_REPOSITORY_OWNER,omitempty"    env:"GITHUB_REPOSITORY_OWNER"`
	RepositoryOwnerID string `json:"GITHUB_REPOSITORY_OWNER_ID,omitempty" env:"GITHUB_REPOSITORY_OWNER_ID"`
	RetentionDays     string `json:"GITHUB_RETENTION_DAYS,omitempty"      env:"GITHUB_RETENTION_DAYS"`
	RunAttempt        string `json:"GITHUB_RUN_ATTEMPT,omitempty"         env:"GITHUB_RUN_ATTEMPT"`
	// RunID refers to the workflow run ID
	RunID       int64  `json:"GITHUB_RUN_ID,omitempty"              env:"GITHUB_RUN_ID"`
	RunNumber   int    `json:"GITHUB_RUN_NUMBER,omitempty"          env:"GITHUB_RUN_NUMBER"`
	ServerURL   string `json:"GITHUB_SERVER_URL,omitempty"          env:"GITHUB_SERVER_URL"`
	SHA         string `json:"GITHUB_SHA,omitempty"                 env:"GITHUB_SHA"`
	StepSummary string `json:"GITHUB_STEP_SUMMARY,omitempty"        env:"GITHUB_STEP_SUMMARY"`
	// Token isn't guaranteed to be set as an env var, but it's a standard process, especially for the octometrics-action.
	Token             string `json:"GITHUB_TOKEN,omitempty"               env:"GITHUB_TOKEN"`
	TriggeringActor   string `json:"GITHUB_TRIGGERING_ACTOR,omitempty"    env:"GITHUB_TRIGGERING_ACTOR"`
	Workflow          string `json:"GITHUB_WORKFLOW,omitempty"            env:"GITHUB_WORKFLOW"`
	WorkflowRef       string `json:"GITHUB_WORKFLOW_REF,omitempty"        env:"GITHUB_WORKFLOW_REF"`
	WorkflowSHA       string `json:"GITHUB_WORKFLOW_SHA,omitempty"        env:"GITHUB_WORKFLOW_SHA"`
	Workspace         string `json:"GITHUB_WORKSPACE,omitempty"           env:"GITHUB_WORKSPACE"`
	RunnerArch        string `json:"RUNNER_ARCH,omitempty"                env:"RUNNER_ARCH"`
	RunnerDebug       string `json:"RUNNER_DEBUG,omitempty"               env:"RUNNER_DEBUG"`
	RunnerEnvironment string `json:"RUNNER_ENVIRONMENT,omitempty"         env:"RUNNER_ENVIRONMENT"`
	RunnerName        string `json:"RUNNER_NAME,omitempty"                env:"RUNNER_NAME"`
	RunnerOS          string `json:"RUNNER_OS,omitempty"                  env:"RUNNER_OS"`
	RunnerTemp        string `json:"RUNNER_TEMP,omitempty"                env:"RUNNER_TEMP"`
	RunnerToolCache   string `json:"RUNNER_TOOL_CACHE,omitempty"          env:"RUNNER_TOOL_CACHE"`
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
