package monitor

import (
	"fmt"
	"time"

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
	OutputFile       string
	ObserveInterval  time.Duration
	MonitorCPU       bool
	MonitorMemory    bool
	MonitorDisk      bool
	MonitorIO        bool
	MonitorProcesses bool
}

func defaultOptions() *options {
	return &options{
		OutputFile:       DataFile,
		ObserveInterval:  time.Second,
		MonitorCPU:       true,
		MonitorMemory:    true,
		MonitorDisk:      true,
		MonitorIO:        true,
		MonitorProcesses: false,
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
	GitHubActions *githubActions `json:"github_actions,omitempty"`
}

// logTime is a custom time type for parsing log timestamps.
type logTime struct {
	time.Time
}

type githubActions struct {
	RunID             *string `json:"run_id,omitempty"`
	RunNumber         *string `json:"run_number,omitempty"`
	WorkflowRunID     *string `json:"workflow_run_id,omitempty"`
	WorkflowRunNumber *string `json:"workflow_run_number,omitempty"`
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
