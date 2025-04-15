package cmd

import (
	"fmt"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// These variables are set at build time and describe the version and build of the application
var (
	version   = "dev"
	commit    = "dev"
	buildTime = time.Now().Format("2006-01-02T15:04:05.000")
	builtBy   = "local"
	builtWith = runtime.Version()
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Octometrics",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(
			"octometrics version %s built with %s from commit %s at %s by %s\n",
			version,
			builtWith,
			commit,
			buildTime,
			builtBy,
		)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
