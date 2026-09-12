package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	whoamiJSON      bool
	whoamiCheck     bool
	whoamiNewClient = func(host, token string) api.Client { return api.NewClient(host, token) }
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the authenticated user for each host you've run gitcollect auth on",
	Long: `Show which user is authenticated for each host.

Displays the GitHub/GitLab username, platform user ID, and platform
host for every token stored in ~/.gitcollect/config. Useful for
verifying you are using the right account before team operations.

Examples:
  gitcollect whoami
  gitcollect whoami --json

See also:
  gitcollect auth   — store or refresh a token`,
	Args: cobra.NoArgs,
	RunE: runWhoami,
}

func init() {
	whoamiCmd.Flags().BoolVar(&whoamiJSON, "json", false, "machine-readable output")
	whoamiCmd.Flags().BoolVar(&whoamiCheck, "check", false, "exit with code 1 if any stored token is rejected by the platform")
	rootCmd.AddCommand(whoamiCmd)
}

type whoamiEntry struct {
	Host  string `json:"host"`
	User  string `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

func runWhoami(cmd *cobra.Command, args []string) error {
	hosts, err := config.Hosts()
	if err != nil {
		return fmt.Errorf("whoami: could not read config: %w", err)
	}
	if len(hosts) == 0 {
		return fmt.Errorf("not authenticated. Run: gitcollect auth")
	}

	entries := make([]whoamiEntry, 0, len(hosts))
	anyRejected := false
	for _, host := range hosts {
		token, err := config.LoadToken(host)
		if err != nil {
			return fmt.Errorf("whoami: could not read token for %s: %w", host, err)
		}

		client := whoamiNewClient(host, token)
		user, err := client.GetAuthenticatedUser()
		if err != nil {
			if errors.Is(err, api.ErrUnauthorized) {
				anyRejected = true
			}
			entries = append(entries, whoamiEntry{Host: host, Error: err.Error()})
			continue
		}
		entries = append(entries, whoamiEntry{Host: host, User: user.Login})
	}

	if whoamiJSON {
		return output.JSON(entries)
	}

	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		user := e.User
		if e.Error != "" {
			user = "error: " + e.Error
		}
		rows = append(rows, []string{e.Host, user})
	}
	output.Table([]string{"HOST", "USER"}, rows)
	if anyRejected {
		output.Suggestion("gitcollect auth  # for any host shown above with a rejected token")
		if whoamiCheck {
			return fmt.Errorf("whoami: one or more stored tokens were rejected by the platform")
		}
	}
	return nil
}
