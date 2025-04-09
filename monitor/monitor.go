// package monitor provides a way to monitor system resources like CPU, memory, and disk usage while your workflow runs.
package monitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"golang.org/x/sync/errgroup"

	"github.com/kalverra/octometrics/logging"
)

const (
	DataFile = "octometrics.monitor.json"

	// Log messages used to indicate the system info
	CPUSystemInfoMsg  = "CPU System Info"
	MemSystemInfoMsg  = "System Memory Info"
	DiskSystemInfoMsg = "System Disk Info"

	// Log messages used to indicate the status of monitoring
	ObservedCPUMsg  = "Observed CPU Usage"
	ObservedMemMsg  = "Observed Memory Usage"
	ObservedDiskMsg = "Observed Disk Usage"
	ObservedProcMsg = "Observed Process Usage"
	ObservedIOMsg   = "Observed IO Usage"
)

var (
	ErrMonitorCPU       = errors.New("error monitoring CPU")
	ErrMonitorMemory    = errors.New("error monitoring Memory")
	ErrMonitorDisk      = errors.New("error monitoring Disk")
	ErrMonitorIO        = errors.New("error monitoring IO")
	ErrMonitorProcesses = errors.New("error monitoring Processes")
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

// Start begins monitoring system resources and writes the data to a file.
// It will stop when the context is cancelled or an interrupt is received.
func Start(ctx context.Context, options ...Option) error {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	log, err := logging.New(
		logging.WithFileName(opts.OutputFile),
		logging.WithLevel("trace"),
		logging.DisableConsoleLog(),
	)
	if err != nil {
		return fmt.Errorf("error creating logger: %w", err)
	}

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

	log.Info().
		Str("Output File", opts.OutputFile).
		Str("Observe Interval", opts.ObserveInterval.String()).
		Bool("Monitor CPU", opts.MonitorCPU).
		Bool("Monitor Memory", opts.MonitorMemory).
		Bool("Monitor Disk", opts.MonitorDisk).
		Bool("Monitor Processes", opts.MonitorProcesses).
		Msg("Starting Monitoring")

	if err := systemInfo(log); err != nil {
		return fmt.Errorf("error gathering system info: %w", err)
	}

	// Observe immediately before starting the ticker
	if err := observe(log, opts); err != nil {
		return fmt.Errorf("error observing system info: %w", err)
	}

	ticker := time.NewTicker(opts.ObserveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			switch ctx.Err() {
			case context.Canceled:
				log.Info().Str("Reason", "Monitoring cancelled").Msg("Stopping Monitoring")
			case context.DeadlineExceeded:
				log.Info().Str("Reason", "Monitoring timed out").Msg("Stopping Monitoring")
			default:
				log.Error().Str("Reason", ctx.Err().Error()).Msg("Stopping Monitoring")
			}
			return nil
		case <-interruptChan:
			log.Info().Str("Reason", "Process interrupted or terminated").Msg("Stopping Monitoring")
			return nil
		case <-ticker.C:
			if err := observe(log, opts); err != nil {
				return fmt.Errorf("error observing system info: %w", err)
			}
		}
	}
}

func systemInfo(log zerolog.Logger) error {
	cpus, err := cpu.Info()
	if err != nil {
		return err
	}
	for _, cpu := range cpus {
		log.Info().
			Int32("Num", cpu.CPU).
			Str("Model", cpu.ModelName).
			Str("Vendor", cpu.VendorID).
			Str("Family", cpu.Family).
			Int32("Cache Size", cpu.CacheSize).
			Int32("Cores", cpu.Cores).
			Float64("Mhz", cpu.Mhz).
			Msg(CPUSystemInfoMsg)
	}

	memStat, err := mem.VirtualMemory()
	if err != nil {
		return err
	}
	log.Info().
		Uint64("Total", memStat.Total).
		Msg(MemSystemInfoMsg)

	diskStat, err := disk.Usage("/")
	if err != nil {
		return err
	}
	log.Info().
		Uint64("Total", diskStat.Total).
		Msg(DiskSystemInfoMsg)

	return nil
}

func observe(log zerolog.Logger, opts *options) error {
	var (
		eg        errgroup.Group
		startTime = time.Now()
	)

	if opts.MonitorCPU {
		eg.Go(func() error {
			return observeCPU(log)
		})
	}

	if opts.MonitorMemory {
		eg.Go(func() error {
			return observeMemory(log)
		})
	}

	if opts.MonitorDisk {
		eg.Go(func() error {
			return observeDisk(log)
		})
	}

	if opts.MonitorIO {
		eg.Go(func() error {
			return observeIO(log)
		})
	}

	if opts.MonitorProcesses {
		log.Warn().Msg("Process monitoring not implemented yet")
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error while monitoring: %w", err)
	}

	log.Trace().
		Str("Duration", time.Since(startTime).String()).
		Msg("Finished observation")
	return nil
}

func observeCPU(log zerolog.Logger) error {
	cpuPercents, err := cpu.Percent(0, true)
	if err != nil {
		return fmt.Errorf("error monitoring CPU: %w", err)
	}

	if len(cpuPercents) == 0 {
		return ErrMonitorCPU
	}

	for i, percent := range cpuPercents {
		log.Trace().
			Int("CPU", i).
			Float64("Percent", percent).
			Msg(ObservedCPUMsg)
	}
	return nil
}

func observeMemory(log zerolog.Logger) error {
	v, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMonitorMemory, err)
	}
	log.Trace().
		Uint64("Available", v.Available).
		Uint64("Used", v.Used).
		Msg(ObservedMemMsg)
	return nil
}

func observeDisk(log zerolog.Logger) error {
	usageStat, err := disk.Usage("/")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMonitorDisk, err)
	}
	log.Trace().
		Uint64("Used", usageStat.Used).
		Uint64("Available", usageStat.Free).
		Float64("Used Percent", usageStat.UsedPercent).
		Msg(ObservedDiskMsg)
	return nil
}

func observeIO(log zerolog.Logger) error {
	ioStats, err := net.IOCounters(false)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMonitorIO, err)
	}
	for _, stat := range ioStats {
		log.Trace().
			Uint64("Bytes Sent", stat.BytesSent).
			Uint64("Bytes Recv", stat.BytesRecv).
			Uint64("Packets Sent", stat.PacketsSent).
			Uint64("Packets Recv", stat.PacketsRecv).
			Msg(ObservedIOMsg)
	}
	return nil
}
