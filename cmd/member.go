package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var memberCmd = &cobra.Command{
	Use:   "member",
	Short: "Manage a collection's members",
}

var memberAddCmd = &cobra.Command{
	Use:   "add <collection> <username>",
	Short: "Add a member to a collection",
	Args:  cobra.ExactArgs(2),
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
	username := args[1]

	col, err := loadCollection(name)
	if err != nil {
		return fmt.Errorf("member add: %w", err)
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return fmt.Errorf("member add: %w", err)
	}
	caller, err := currentUser(client)
	if err != nil {
		return fmt.Errorf("member add: %w", err)
	}
	if caller != col.Owner {
		return fmt.Errorf("member add: only %s (the owner) can add members to %q", col.Owner, name)
	}

	if col.IsMember(username) {
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
		return fmt.Errorf("member add: %w", err)
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
	printAccessBreakdown(col, username)
	return nil
}

// printAccessBreakdown reports, for username, which of col's repos they can
// now reach and which they were skipped for (with why), so member add gives
// the caller an immediate, actionable picture of the new member's access.
func printAccessBreakdown(col *collection.Collection, username string) {
	if len(col.Repos) == 0 {
		return
	}

	var granted, skipped []string
	skippedReason := map[string]string{}
	for _, r := range col.Repos {
		if col.CanAccessRepo(username, r.Name) {
			granted = append(granted, r.Name)
		} else {
			skipped = append(skipped, r.Name)
			skippedReason[r.Name] = col.WhyCanAccess(username, r.Name)
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
}

func runMemberRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	username := args[1]

	col, err := loadCollection(name)
	if err != nil {
		return fmt.Errorf("member remove: %w", err)
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return fmt.Errorf("member remove: %w", err)
	}
	caller, err := currentUser(client)
	if err != nil {
		return fmt.Errorf("member remove: %w", err)
	}

	isSelf := caller == username
	if caller != col.Owner && !isSelf {
		return fmt.Errorf("member remove: only %s (the owner) can remove other members from %q", col.Owner, name)
	}
	if isSelf && !memberConfirmSelf {
		return NewUsageError(fmt.Errorf("member remove: removing yourself requires --confirm-self"))
	}

	if !col.IsMember(username) {
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

	col, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("member list: %w", err)
	}

	if len(col.Members) == 0 {
		output.Info("%q has no members", name)
		return nil
	}

	rows := make([][]string, 0, len(col.Members))
	for _, m := range col.Members {
		groups := groupsForMember(col, m)
		groupList := "—"
		if len(groups) > 0 {
			groupList = strings.Join(groups, ", ")
		}
		rows = append(rows, []string{m, groupList})
	}
	output.Table([]string{"MEMBER", "GROUPS"}, rows)
	return nil
}

func groupsForMember(col *collection.Collection, username string) []string {
	var groups []string
	for group := range col.Groups {
		if col.IsInGroup(username, group) {
			groups = append(groups, group)
		}
	}
	return groups
}
