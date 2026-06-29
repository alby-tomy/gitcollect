package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and platform information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("gitcollect %s %s/%s\n", appVersion, runtime.GOOS, runtime.GOARCH)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
