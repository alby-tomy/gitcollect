package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
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
		// api.ErrUserNotFound, not collection.ErrNotFound — GetUser is an
		// API call and never returns the collection package's sentinel, so
		// testing for that one made this branch unreachable and a typo'd
		// username surfaced as a raw "resolve" error instead.
		if errors.Is(err, api.ErrUserNotFound) {
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

	// Pin the namespace before the owner changes. RepoNamespace() falls
	// back to the owner's login when Namespace is empty, so transferring
	// would otherwise silently repoint every API path at the new owner's
	// account — where the repos do not live. Transferring a collection
	// does not move repositories on the platform.
	if col.Namespace == "" {
		col.Namespace = col.Logins[previousOwnerID]
	}

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

	// The new owner stays in Members. Removing them broke every transfer
	// to anyone who belonged to a group or held an individual repo grant:
	// Validate requires each group member and each RepoAccess.User to
	// appear in Members, so Save failed with "group X references Y, who is
	// not a member" — which is the common case, since the natural
	// successor is an established team member. Owner and member are not
	// exclusive anywhere else in the model either: CanAccessRepo passes
	// the owner regardless, so the extra entry grants nothing new.

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
