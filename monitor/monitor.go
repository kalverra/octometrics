package monitor

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type Observations struct {
}

func Monitor(interval time.Duration) error {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

	cpus, err := cpu.Info()
	if err != nil {
		return err
	}
	for _, c := range cpus {
		log.Info().Str("name", c.ModelName).Int32("cores", c.Cores).Msg("CPU Info")
	}

	var monitorErrs error
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-interruptChan:
			log.Info().Msg("Stopping Monitoring")
			return monitorErrs
		case <-ticker.C:
			if err := monitor(); err != nil {
				log.Error().Err(err).Msg("Error monitoring")
				if monitorErrs == nil {
					monitorErrs = err
				} else {
					monitorErrs = fmt.Errorf("%w; %w", monitorErrs, err)
				}
			}
		}
	}
}

func monitor() error {
	v, err := mem.VirtualMemory()
	if err != nil {
		return err
	}
	log.Info().Uint64("Available", v.Available).Uint64("Used", v.Used).Msg("Virtual Memory")

	processes, err := process.Processes()
	if err != nil {
		return err
	}
	for _, p := range processes {
		name, err := p.Exe() // Name has a bug on Mac: https://github.com/shirou/gopsutil/issues/1803
		if err != nil {
			return fmt.Errorf("error getting process name: %w", err)
		}
		name = filepath.Base(name)
		cpuPercent, err := p.CPUPercent()
		if err != nil {
			return fmt.Errorf("error getting process CPU percent: %w", err)
		}
		memPercent, err := p.MemoryPercent()
		if err != nil {
			return fmt.Errorf("error getting process memory percent: %w", err)
		}
		log.Info().
			Str("name", name).
			Float64("cpu_pct", cpuPercent).
			Float32("mem_pct", memPercent).
			Int32("pid", p.Pid).
			Msg("Process")
	}
	return nil
}
