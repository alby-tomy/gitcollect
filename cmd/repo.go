package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage per-repo access within a collection",
}

var (
	repoAccessGroups []string
	repoAccessUsers  []string
	repoAccessOpen   bool
)

var repoAccessCmd = &cobra.Command{
	Use:   "access <collection> <repo>",
	Short: "Restrict or open up who can access a repo",
	Long: `Restrict or open up who can access a repo within a collection.

To grant or revoke access for individual users, use --users to set the complete list:
  gitcollect repo access cybersecurity vuln-scanner --users alice bob
  gitcollect repo access cybersecurity vuln-scanner --open`,
	Args: cobra.ExactArgs(2),
	RunE: runRepoAccess,
}

var repoShowCmd = &cobra.Command{
	Use:   "show <collection> <repo>",
	Short: "Show who can access a repo and why",
	Args:  cobra.ExactArgs(2),
	RunE:  runRepoShow,
}

func init() {
	repoAccessCmd.Flags().StringSliceVar(&repoAccessGroups, "groups", nil, "restrict access to these groups (comma-separated)")
	repoAccessCmd.Flags().StringSliceVar(&repoAccessUsers, "users", nil, "restrict access to these individual users (comma-separated)")
	repoAccessCmd.Flags().BoolVar(&repoAccessOpen, "open", false, "open the repo to all members")

	repoCmd.AddCommand(repoAccessCmd)
	repoCmd.AddCommand(repoShowCmd)
	rootCmd.AddCommand(repoCmd)
}

func runRepoAccess(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoName := args[1]

	groupsSet := cmd.Flags().Changed("groups")
	usersSet := cmd.Flags().Changed("users")
	if repoAccessOpen && (groupsSet || usersSet) {
		return NewUsageError(fmt.Errorf("repo access: --open cannot be combined with --groups or --users"))
	}
	if !repoAccessOpen && !groupsSet && !usersSet {
		return NewUsageError(fmt.Errorf("repo access: specify --groups, --users, or --open"))
	}

	groups := repoAccessGroups
	users := repoAccessUsers
	if repoAccessOpen {
		groups = []string{}
		users = []string{}
	}

	col, caller, callerID, client, err := loadForOwner("repo access", name)
	if err != nil {
		return err
	}

	before, found := repoAccessOf(col, repoName)
	if !found {
		return fmt.Errorf("repo access: %q is not in collection %q", repoName, name)
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("repo access: only %s (the owner) can change access for %s", col.Logins[col.Owner], repoName)
	}

	beforeDesc := describeAccess(col, before)

	if err := col.SetRepoAccess(repoName, groups, users, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "repo.access.set",
			Target:     repoName,
			Detail:     "Failed to update access",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("repo access: %w", err)
	}

	after, _ := repoAccessOf(col, repoName)
	afterDesc := describeAccess(col, after)

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "repo.access.set",
		Target:     repoName,
		Detail:     fmt.Sprintf("%s → %s", beforeDesc, afterDesc),
		Result:     "ok",
	})

	output.Success("Updated access for %s", repoName)
	fmt.Printf("  Before: %s\n", beforeDesc)
	fmt.Printf("  After:  %s\n", afterDesc)
	output.Suggestion(fmt.Sprintf("gitcollect inspect %s --repo %s", name, repoName))
	return nil
}

func repoAccessOf(col *collection.Collection, repoName string) (collection.RepoAccess, bool) {
	for _, r := range col.Repos {
		if r.Name == repoName {
			return r, true
		}
	}
	return collection.RepoAccess{}, false
}

// describeAccess renders r's access rule for display. r.Users holds
// platform IDs (see collection.RepoAccess's doc comment), so col is
// needed to resolve them to logins via loginsFor before joining.
func describeAccess(col *collection.Collection, r collection.RepoAccess) string {
	switch {
	case len(r.Groups) == 0 && len(r.Users) == 0:
		return "open to all members"
	case len(r.Groups) > 0 && len(r.Users) > 0:
		return fmt.Sprintf("groups: %s, users: %s", strings.Join(r.Groups, ", "), strings.Join(loginsFor(col, r.Users), ", "))
	case len(r.Groups) > 0:
		return fmt.Sprintf("groups: %s", strings.Join(r.Groups, ", "))
	default:
		return fmt.Sprintf("users: %s", strings.Join(loginsFor(col, r.Users), ", "))
	}
}

func runRepoShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoName := args[1]

	col, _, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("repo show: %w", err)
	}

	repo, found := repoAccessOf(col, repoName)
	if !found {
		return fmt.Errorf("repo show: %q is not in collection %q", repoName, name)
	}

	fmt.Printf("Repo:   %s\n", repo.Name)
	fmt.Printf("Access: %s\n", describeAccess(col, repo))
	fmt.Println()

	details := access.RepoAccessMap(col, repoName)
	rows := make([][]string, 0, len(details))
	for _, d := range details {
		mark := "✗ no"
		if d.CanAccess {
			mark = "✓ yes"
		}
		rows = append(rows, []string{d.Username, mark, d.Reason})
	}
	output.Table([]string{"MEMBER", "ACCESS", "REASON"}, rows)
	return nil
}
