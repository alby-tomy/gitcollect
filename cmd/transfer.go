package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var transferCmd = &cobra.Command{
	Use:   "transfer <collection> <new-owner-username>",
	Short: "Transfer collection ownership to another member",
	Args:  cobra.ExactArgs(2),
	RunE:  runTransfer,
}

func init() {
	rootCmd.AddCommand(transferCmd)
}

func runTransfer(cmd *cobra.Command, args []string) error {
	name := args[0]
	newOwnerUsername := args[1]

	if err := collection.ValidateUsername(newOwnerUsername); err != nil {
		return NewUsageError(fmt.Errorf("transfer: %w", err))
	}

	col, caller, callerID, client, err := loadForOwner("transfer", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("transfer: only %s (the owner) can transfer %q", col.Logins[col.Owner], name)
	}

	// Resolve new owner's platform identity.
	newOwner, err := client.GetUser(newOwnerUsername)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			return fmt.Errorf("transfer: user %q not found on %s", newOwnerUsername, col.Host)
		}
		return fmt.Errorf("transfer: resolve %s: %w", newOwnerUsername, err)
	}

	// Cannot transfer to yourself.
	if newOwner.ID == callerID {
		return fmt.Errorf("transfer: %w", collection.ErrSelfTransfer)
	}

	// New owner must already be a member.
	if !col.IsMember(newOwner.ID) {
		return fmt.Errorf(
			"transfer: %s is not a member of %q\n  Run: gitcollect member add %s %s",
			newOwnerUsername, name, name, newOwnerUsername,
		)
	}

	// If group admins are enabled, the new owner must not be a group admin
	// (role ambiguity: an owner who is also a group admin of a specific group
	// is confusing — remove them as group admin first).
	if col.GroupAdminsEnabled {
		if groups := col.GroupAdminOf(newOwner.ID); len(groups) > 0 {
			return fmt.Errorf(
				"transfer: %s is a group admin of %s — remove their group admin role first:\n  gitcollect group admin remove %s %s %s",
				newOwnerUsername, strings.Join(groups, ", "), name, groups[0], newOwnerUsername,
			)
		}
	}

	// Typed confirmation.
	output.Warn("This will transfer ownership of %s to %s.", name, newOwnerUsername)
	output.Dim("  You (%s) will become a regular member.", caller)
	output.Dim("  This action cannot be undone by you — only %s can transfer it back.", newOwnerUsername)
	fmt.Println()
	if !output.ConfirmWord(fmt.Sprintf("Type %q to confirm", newOwnerUsername), newOwnerUsername) {
		return fmt.Errorf("transfer: aborted")
	}

	// Apply the transfer: previous owner becomes a regular member.
	previousOwnerID := col.Owner
	col.Owner = newOwner.ID
	col.Logins[newOwner.ID] = newOwner.Login

	// Ensure previous owner is still in Members.
	inMembers := false
	for _, m := range col.Members {
		if m == previousOwnerID {
			inMembers = true
			break
		}
	}
	if !inMembers {
		col.Members = append(col.Members, previousOwnerID)
	}

	// Remove new owner from Members (they are now the owner, not a member).
	col.Members = removeStringSlice(col.Members, newOwner.ID)

	if err := col.Save(); err != nil {
		return fmt.Errorf("transfer: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "collection.transfer",
		Target:     newOwnerUsername,
		Detail:     fmt.Sprintf("Transferred ownership from %s to %s", caller, newOwnerUsername),
		Result:     "ok",
	})

	output.Success("Transferred %s to %s", name, newOwnerUsername)
	output.Dim("  You have been added as a member with full access")
	output.Suggestion(fmt.Sprintf("gitcollect show %s  to verify the new state", name))
	return nil
}

// removeStringSlice removes all occurrences of target from list.
func removeStringSlice(list []string, target string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}
