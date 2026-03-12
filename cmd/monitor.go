package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kalverra/octometrics/monitor"
)

var (
	skipCPU    bool
	skipMemory bool
	skipDisk   bool
	skipIO     bool

	duration   time.Duration
	interval   time.Duration
	outputFile string
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor system resources",
	Long: `Monitor system resources for later analysis.

This command will monitor system resources like CPU, memory, disk, and I/O during a GHA job.

It will write the data to a file for later analysis. Primarily used in the octometrics-action to monitor system resources during a GHA job.`,
	Example: `
octometrics monitor # Monitor system resources until interrupted
octometrics monitor --duration=1h # Monitor system resources for 1 hour
octometrics monitor --interval=5s # Monitor system resources every 5 seconds
`,
	RunE: func(_ *cobra.Command, _ []string) error {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)
		if duration > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), duration)
			defer cancel()
		} else {
			ctx = context.Background()
		}

		err := monitor.Start(ctx,
			monitor.WithObserveInterval(interval),
			monitor.WithOutputFile(outputFile),
		)
		if err != nil {
			return fmt.Errorf("error monitoring system resources, stopping monitoring: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)

	monitorCmd.Flags().BoolVar(&skipCPU, "skip-cpu", false, "Skip CPU monitoring")
	monitorCmd.Flags().BoolVar(&skipMemory, "skip-memory", false, "Skip memory monitoring")
	monitorCmd.Flags().BoolVar(&skipDisk, "skip-disk", false, "Skip disk monitoring")
	monitorCmd.Flags().BoolVar(&skipIO, "skip-io", false, "Skip IO monitoring")
	monitorCmd.Flags().DurationVarP(&duration, "duration", "d", 0, "Duration to monitor, defaults to indefinite")
	monitorCmd.Flags().DurationVarP(&interval, "interval", "i", 1*time.Second, "At what interval to observe metrics")
	monitorCmd.Flags().
		StringVarP(&outputFile, "output-file", "o", monitor.DataFile, "Output file for the monitor data")
}
