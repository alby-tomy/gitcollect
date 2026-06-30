package cmd

import (
	"errors"
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
	Args:  cobra.ExactArgs(2),
	RunE:  runRepoAccess,
}

var repoShowCmd = &cobra.Command{
	Use:   "show <collection> <repo>",
	Short: "Show who can access a repo and why",
	Args:  cobra.ExactArgs(2),
	RunE:  runRepoShow,
}

var repoGrantCmd = &cobra.Command{
	Use:   "grant <collection> <repo> <username>",
	Short: "Grant one user individual access to a repo, without changing its other restrictions",
	Args:  cobra.ExactArgs(3),
	RunE:  runRepoGrant,
}

var repoRevokeCmd = &cobra.Command{
	Use:   "revoke <collection> <repo> <username>",
	Short: "Revoke one user's individually granted access to a repo",
	Args:  cobra.ExactArgs(3),
	RunE:  runRepoRevoke,
}

func init() {
	repoAccessCmd.Flags().StringSliceVar(&repoAccessGroups, "groups", nil, "restrict access to these groups (comma-separated)")
	repoAccessCmd.Flags().StringSliceVar(&repoAccessUsers, "users", nil, "restrict access to these individual users (comma-separated)")
	repoAccessCmd.Flags().BoolVar(&repoAccessOpen, "open", false, "open the repo to all members")

	repoCmd.AddCommand(repoAccessCmd)
	repoCmd.AddCommand(repoShowCmd)
	repoCmd.AddCommand(repoGrantCmd)
	repoCmd.AddCommand(repoRevokeCmd)
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

func runRepoGrant(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoName := args[1]
	username := args[2]

	col, caller, callerID, client, err := loadForOwner("repo grant", name)
	if err != nil {
		return err
	}

	before, found := repoAccessOf(col, repoName)
	if !found {
		return fmt.Errorf("repo grant: %q is not in collection %q", repoName, name)
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("repo grant: only %s (the owner) can change access for %s", col.Logins[col.Owner], repoName)
	}

	// No-network pre-check, same reasoning as IDForLogin's doc comment:
	// "already granted" only needs to match something already recorded
	// locally, not a live platform lookup.
	if id := col.IDForLogin(username); id != "" && containsExact(before.Users, id) {
		output.Info("%s already has individual access to %s", username, repoName)
		return nil
	}

	if err := col.GrantRepoUser(repoName, username, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "repo.user.grant",
			Target:     fmt.Sprintf("%s → %s", username, repoName),
			Detail:     "Failed to grant individual access",
			Result:     "error: " + err.Error(),
		})
		switch {
		case errors.Is(err, collection.ErrRepoOpen):
			output.Error("repo grant: %s", err.Error())
			output.Suggestion(fmt.Sprintf("gitcollect repo access %s %s --users %s", name, repoName, username))
			return fmt.Errorf("repo grant: aborted")
		case errors.Is(err, collection.ErrNotMember):
			output.Error("repo grant: %q is not a member of %s", username, name)
			output.Suggestion(fmt.Sprintf("gitcollect member add %s %s", name, username))
			return fmt.Errorf("repo grant: aborted")
		default:
			return fmt.Errorf("repo grant: %w", err)
		}
	}

	after, _ := repoAccessOf(col, repoName)
	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "repo.user.grant",
		Target:     fmt.Sprintf("%s → %s", username, repoName),
		Detail:     fmt.Sprintf("%s → %s", describeAccess(col, before), describeAccess(col, after)),
		Result:     "ok",
	})

	output.Success("Granted %s individual access to %s", username, repoName)
	fmt.Printf("  Before: %s\n", describeAccess(col, before))
	fmt.Printf("  After:  %s\n", describeAccess(col, after))
	output.Suggestion(fmt.Sprintf("gitcollect inspect %s --repo %s", name, repoName))
	return nil
}

func runRepoRevoke(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoName := args[1]
	username := args[2]

	col, caller, callerID, client, err := loadForOwner("repo revoke", name)
	if err != nil {
		return err
	}

	before, found := repoAccessOf(col, repoName)
	if !found {
		return fmt.Errorf("repo revoke: %q is not in collection %q", repoName, name)
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("repo revoke: only %s (the owner) can change access for %s", col.Logins[col.Owner], repoName)
	}

	// Same no-network pre-check pattern as runRepoGrant.
	id := col.IDForLogin(username)
	if id == "" || !containsExact(before.Users, id) {
		output.Info("%s does not have individually granted access to %s", username, repoName)
		return nil
	}

	if err := col.RevokeRepoUser(repoName, username, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "repo.user.revoke",
			Target:     fmt.Sprintf("%s → %s", username, repoName),
			Detail:     "Failed to revoke individual access",
			Result:     "error: " + err.Error(),
		})
		if errors.Is(err, collection.ErrRepoWouldOpen) {
			output.Error("repo revoke: %s", err.Error())
			output.Suggestion(fmt.Sprintf("gitcollect repo access %s %s --users <remaining-users>", name, repoName))
			return fmt.Errorf("repo revoke: aborted")
		}
		return fmt.Errorf("repo revoke: %w", err)
	}

	after, _ := repoAccessOf(col, repoName)
	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "repo.user.revoke",
		Target:     fmt.Sprintf("%s → %s", username, repoName),
		Detail:     fmt.Sprintf("%s → %s", describeAccess(col, before), describeAccess(col, after)),
		Result:     "ok",
	})

	output.Success("Revoked %s's individual access to %s", username, repoName)
	fmt.Printf("  Before: %s\n", describeAccess(col, before))
	fmt.Printf("  After:  %s\n", describeAccess(col, after))
	output.Suggestion(fmt.Sprintf("gitcollect inspect %s --repo %s", name, repoName))
	return nil
}

func containsExact(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
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
