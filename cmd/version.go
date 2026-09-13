package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var versionJSON bool

type versionInfo struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and platform information",
	Long: `Print the gitcollect version, Go runtime version, and OS/arch.

Examples:
  gitcollect version
  gitcollect version --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if versionJSON {
			return output.JSON(versionInfo{
				Version:   appVersion,
				GoVersion: runtime.Version(),
				Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			})
		}
		fmt.Printf("gitcollect %s %s/%s\n", appVersion, runtime.GOOS, runtime.GOARCH)
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "print version information as JSON")
	rootCmd.AddCommand(versionCmd)
}
