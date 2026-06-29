package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the authenticated user for each host you've run gitcollect auth on",
	Args:  cobra.NoArgs,
	RunE:  runWhoami,
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}

func runWhoami(cmd *cobra.Command, args []string) error {
	hosts, err := config.Hosts()
	if err != nil {
		return fmt.Errorf("whoami: could not read config: %w", err)
	}
	if len(hosts) == 0 {
		return fmt.Errorf("not authenticated. Run: gitcollect auth")
	}

	rows := make([][]string, 0, len(hosts))
	for _, host := range hosts {
		token, err := config.LoadToken(host)
		if err != nil {
			return fmt.Errorf("whoami: could not read token for %s: %w", host, err)
		}

		client := api.NewClient(host, token)
		username, err := client.GetAuthenticatedUser()
		if err != nil {
			rows = append(rows, []string{host, fmt.Sprintf("error: %v", err)})
			continue
		}
		rows = append(rows, []string{host, username})
	}

	output.Table([]string{"HOST", "USER"}, rows)
	return nil
}
