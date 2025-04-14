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
	ObservedCPUMsg                  = "Observed CPU Usage"
	ObservedMemMsg                  = "Observed Memory Usage"
	ObservedDiskMsg                 = "Observed Disk Usage"
	ObservedProcMsg                 = "Observed Process Usage"
	ObservedIOMsg                   = "Observed IO Usage"
	ObservedGitHubActionsEnvVarsMsg = "Observed GitHub Actions Environment Variables"
)

var (
	ErrMonitorCPU       = errors.New("error monitoring CPU")
	ErrMonitorMemory    = errors.New("error monitoring Memory")
	ErrMonitorDisk      = errors.New("error monitoring Disk")
	ErrMonitorIO        = errors.New("error monitoring IO")
	ErrMonitorProcesses = errors.New("error monitoring Processes")
)

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
		Str("output_file", opts.OutputFile).
		Str("observe_interval", opts.ObserveInterval.String()).
		Bool("monitor_cpu", opts.MonitorCPU).
		Bool("monitor_memory", opts.MonitorMemory).
		Bool("monitor_disk", opts.MonitorDisk).
		Bool("monitor_processes", opts.MonitorProcesses).
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
			Int32("num", cpu.CPU).
			Str("model", cpu.ModelName).
			Str("vendor", cpu.VendorID).
			Str("family", cpu.Family).
			Int32("cache_size", cpu.CacheSize).
			Int32("cores", cpu.Cores).
			Float64("mhz", cpu.Mhz).
			Msg(CPUSystemInfoMsg)
	}

	memStat, err := mem.VirtualMemory()
	if err != nil {
		return err
	}
	log.Info().
		Uint64("total", memStat.Total).
		Msg(MemSystemInfoMsg)

	diskStat, err := disk.Usage("/")
	if err != nil {
		return err
	}
	log.Info().
		Uint64("total", diskStat.Total).
		Msg(DiskSystemInfoMsg)

	return nil
}

func observe(log zerolog.Logger, opts *options) error {
	var (
		eg        errgroup.Group
		startTime = time.Now()
	)

	if opts.MonitorGitHubActionsEnvVars {
		eg.Go(func() error {
			return observeGitHubActionsEnvVars(log)
		})
	}

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
			Int("cpu", i).
			Float64("percent", percent).
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
		Uint64("available", v.Available).
		Uint64("used", v.Used).
		Msg(ObservedMemMsg)
	return nil
}

func observeDisk(log zerolog.Logger) error {
	usageStat, err := disk.Usage("/")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMonitorDisk, err)
	}
	log.Trace().
		Uint64("used", usageStat.Used).
		Uint64("available", usageStat.Free).
		Float64("used_percent", usageStat.UsedPercent).
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
			Uint64("bytes_sent", stat.BytesSent).
			Uint64("bytes_recv", stat.BytesRecv).
			Uint64("packets_sent", stat.PacketsSent).
			Uint64("packets_recv", stat.PacketsRecv).
			Msg(ObservedIOMsg)
	}
	return nil
}

func observeGitHubActionsEnvVars(log zerolog.Logger) error {
	envVars, err := collectGitHubActionsEnvVars()
	if err != nil {
		return fmt.Errorf("error collecting GitHub Actions environment variables: %w", err)
	}
	if envVars == nil {
		return nil
	}
	log.Trace().
		Interface("env_vars", envVars).
		Msg(ObservedGitHubActionsEnvVarsMsg)
	return nil
}
