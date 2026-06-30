package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var authHost string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with GitHub or GitLab and store the access token",
	Long: `Prompts for a personal access token (input is hidden) and verifies it
against the platform API before storing it under ~/.gitcollect/config.

Run "gitcollect auth --host gitlab.com" to authenticate a GitLab instance
instead of the default, github.com.`,
	Args: cobra.NoArgs,
	RunE: runAuth,
}

func init() {
	authCmd.Flags().StringVar(&authHost, "host", "github.com", "platform host to authenticate (e.g. github.com, gitlab.com)")
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	host := strings.TrimSpace(authHost)
	if host == "" {
		return NewUsageError(fmt.Errorf("auth: invalid host: host must not be empty"))
	}

	token, err := promptForToken(host)
	if err != nil {
		return fmt.Errorf("auth: failed to read token: %w", err)
	}
	if token == "" {
		return NewUsageError(fmt.Errorf("auth: no token entered: a token is required to authenticate"))
	}

	client := api.NewClient(host, token)

	user, err := client.GetAuthenticatedUser()
	if err != nil {
		return fmt.Errorf("auth: token rejected by %s: %w", host, err)
	}

	if err := config.SaveToken(host, token); err != nil {
		return fmt.Errorf("auth: failed to save token: %w", err)
	}
	if err := config.SaveUser(host, user.Login); err != nil {
		return fmt.Errorf("auth: failed to cache username: %w", err)
	}
	if err := config.SaveUserID(host, user.ID); err != nil {
		return fmt.Errorf("auth: failed to cache user ID: %w", err)
	}

	output.Success("Authenticated as %s on %s", user.Login, host)
	return nil
}

// promptForToken writes the prompt directly to stderr (rather than via
// internal/output) so it shares a line with the hidden input that follows;
// output's status helpers are line-oriented and not meant for that.
func promptForToken(host string) (string, error) {
	fmt.Fprintf(os.Stderr, "Token for %s (input hidden): ", host)
	tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(tokenBytes)), nil
}
