package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var memberCmd = &cobra.Command{
	Use:   "member",
	Short: "Manage a collection's members",
}

var memberAddCmd = &cobra.Command{
	Use:   "add <collection> <username> [username...]",
	Short: "Add one or more members to a collection",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runMemberAdd,
}

var memberConfirmSelf bool

var memberRemoveCmd = &cobra.Command{
	Use:   "remove <collection> <username>",
	Short: "Remove a member from a collection and revoke all their access",
	Args:  cobra.ExactArgs(2),
	RunE:  runMemberRemove,
}

var memberListCmd = &cobra.Command{
	Use:   "list <collection>",
	Short: "List members and their group memberships",
	Args:  cobra.ExactArgs(1),
	RunE:  runMemberList,
}

func init() {
	memberRemoveCmd.Flags().BoolVar(&memberConfirmSelf, "confirm-self", false, "required to remove your own username")

	memberCmd.AddCommand(memberAddCmd)
	memberCmd.AddCommand(memberRemoveCmd)
	memberCmd.AddCommand(memberListCmd)
	rootCmd.AddCommand(memberCmd)
}

func runMemberAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	usernames := args[1:]

	col, caller, callerID, client, err := loadForOwner("member add", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("member add: only %s (the owner) can add members to %q", col.Logins[col.Owner], name)
	}

	var failed []string
	for i, username := range usernames {
		if len(usernames) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("--- %s ---\n", username)
		}
		if err := addOneMember(col, name, caller, username, client); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", username, err))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("member add: %d of %d failed: %s", len(failed), len(usernames), strings.Join(failed, "; "))
	}
	return nil
}

// addOneMember adds a single username to col, reporting and auditing the
// result. Factored out of runMemberAdd so adding several members in one
// invocation can continue past an individual failure instead of aborting
// the whole batch.
func addOneMember(col *collection.Collection, name, caller, username string, client api.Client) error {
	// No-network pre-check (IDForLogin, not GetUser): "already a member"
	// only needs to match something already recorded locally.
	if id := col.IDForLogin(username); id != "" && col.IsMember(id) {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "member.add",
			Target:     username,
			Detail:     "Already a member",
			Result:     "ok",
		})
		output.Info("%s is already a member of %q", username, name)
		return nil
	}

	if err := col.AddMember(username, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "member.add",
			Target:     username,
			Detail:     "Failed to sync access for new member",
			Result:     "error: " + err.Error(),
		})
		return err
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "member.add",
		Target:     username,
		Detail:     "Added member",
		Result:     "ok",
	})

	output.Success("Added %s to %s", username, name)
	printAccessBreakdown(col, username, client)
	return nil
}

// printAccessBreakdown reports, for username, which of col's repos they can
// now reach and which they were skipped for (with why), so member add gives
// the caller an immediate, actionable picture of the new member's access.
// It also checks whether any newly granted repo left username with a
// pending (unaccepted) collaborator invite — see api.Client.GetPendingInvite
// — and if so, warns that they won't actually be able to clone until they
// accept it.
func printAccessBreakdown(col *collection.Collection, username string, client api.Client) {
	if len(col.Repos) == 0 {
		return
	}
	// AddMember (called just before this) already added username's ID to
	// col.Members and its login to col.Logins, so this is a no-network
	// lookup, not a fresh resolve.
	id := col.IDForLogin(username)

	var granted, skipped []string
	skippedReason := map[string]string{}
	for _, r := range col.Repos {
		if col.CanAccessRepo(id, r.Name) {
			granted = append(granted, r.Name)
		} else {
			skipped = append(skipped, r.Name)
			skippedReason[r.Name] = col.WhyCanAccess(id, r.Name)
		}
	}

	fmt.Fprintln(os.Stderr)
	if len(granted) > 0 {
		output.Dim("  Granted access: %s", strings.Join(granted, ", "))
	}
	for _, r := range skipped {
		output.Dim("  Skipped: %s (%s)", r, skippedReason[r])
	}
	if len(skipped) > 0 {
		output.Suggestion(fmt.Sprintf("gitcollect group add %s <group> %s", col.Name, username))
	}

	if hasPendingInvite(col, username, granted, client) {
		output.InviteWarning(username, col.Logins[col.Owner], api.GitHubNotificationsURL, "")
	}
}

// hasPendingInvite reports whether username has an unaccepted invite on
// any repo in granted. Stops at the first one found — in practice GitHub
// creates these for every repo in the same AddCollaborator burst, so one
// hit is enough to tell the caller they need to accept an invite.
func hasPendingInvite(col *collection.Collection, username string, granted []string, client api.Client) bool {
	ownerLogin := col.Logins[col.Owner]
	for _, repoName := range granted {
		has, err := client.GetPendingInvite(ownerLogin, repoName, username)
		if err == nil && has {
			return true
		}
	}
	return false
}

func runMemberRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	username := args[1]

	col, caller, callerID, client, err := loadForOwner("member remove", name)
	if err != nil {
		return err
	}

	isSelf := caller == username
	if !col.IsOwner(callerID) && !isSelf {
		return fmt.Errorf("member remove: only %s (the owner) can remove other members from %q", col.Logins[col.Owner], name)
	}
	if isSelf && !memberConfirmSelf {
		return NewUsageError(fmt.Errorf("member remove: removing yourself requires --confirm-self"))
	}

	if id := col.IDForLogin(username); id == "" || !col.IsMember(id) {
		return fmt.Errorf("member remove: %q is not a member of %q", username, name)
	}

	prompt := fmt.Sprintf("Remove %q from %q and revoke their access to all repos?", username, name)
	if !output.Confirm(prompt) {
		return fmt.Errorf("member remove: aborted")
	}

	if err := col.RemoveMember(username, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "member.remove",
			Target:     username,
			Detail:     "Failed to revoke access",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("member remove: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "member.remove",
		Target:     username,
		Detail:     "Removed member and revoked all access",
		Result:     "ok",
	})

	output.Success("Removed %s from %s and revoked all access", username, name)
	return nil
}

func runMemberList(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, _, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("member list: %w", err)
	}

	if len(col.Members) == 0 {
		output.Info("%q has no members", name)
		return nil
	}

	rows := make([][]string, 0, len(col.Members))
	for _, id := range col.Members {
		groups := groupsForMember(col, id)
		groupList := "—"
		if len(groups) > 0 {
			groupList = strings.Join(groups, ", ")
		}
		rows = append(rows, []string{col.Logins[id], groupList})
	}
	output.Table([]string{"MEMBER", "GROUPS"}, rows)
	return nil
}

// groupsForMember returns the names of every group id belongs to. id is a
// platform ID — see collection.Collection's Owner/Members doc comments —
// since it's compared against col.Groups' ID-based member lists via
// IsInGroup.
func groupsForMember(col *collection.Collection, id string) []string {
	var groups []string
	for group := range col.Groups {
		if col.IsInGroup(id, group) {
			groups = append(groups, group)
		}
	}
	return groups
}
