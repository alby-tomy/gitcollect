package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage groups within a collection",
}

var groupCreateCmd = &cobra.Command{
	Use:   "create <collection> <group>",
	Short: "Create a group",
	Args:  cobra.ExactArgs(2),
	RunE:  runGroupCreate,
}

var groupDeleteCmd = &cobra.Command{
	Use:   "delete <collection> <group>",
	Short: "Delete a group (blocked if any repo still uses it)",
	Args:  cobra.ExactArgs(2),
	RunE:  runGroupDelete,
}

var groupAddCmd = &cobra.Command{
	Use:   "add <collection> <group> <username> [username...]",
	Short: "Add one or more members to a group",
	Args:  cobra.MinimumNArgs(3),
	RunE:  runGroupAdd,
}

var groupRemoveCmd = &cobra.Command{
	Use:   "remove <collection> <group> <username>",
	Short: "Remove a member from a group",
	Args:  cobra.ExactArgs(3),
	RunE:  runGroupRemove,
}

var groupListCmd = &cobra.Command{
	Use:   "list <collection>",
	Short: "List groups and their members",
	Args:  cobra.ExactArgs(1),
	RunE:  runGroupList,
}

var groupShowCmd = &cobra.Command{
	Use:   "show <collection> <group>",
	Short: "Show a group's members and the repos they can reach",
	Args:  cobra.ExactArgs(2),
	RunE:  runGroupShow,
}

func init() {
	groupCmd.AddCommand(groupCreateCmd)
	groupCmd.AddCommand(groupDeleteCmd)
	groupCmd.AddCommand(groupAddCmd)
	groupCmd.AddCommand(groupRemoveCmd)
	groupCmd.AddCommand(groupListCmd)
	groupCmd.AddCommand(groupShowCmd)
	rootCmd.AddCommand(groupCmd)
}

// requireOwner loads name, resolves the caller (login + platform ID), and
// confirms the caller is the collection's owner — every group mutation in
// this file is owner-only. Built on loadForOwner, the shared helper every
// owner-perspective command in cmd/ uses for load+resolve+migrate; this
// wrapper adds back the one-line ownership check and message that
// loadForOwner deliberately leaves to its callers.
func requireOwner(verb, name string) (col *collection.Collection, caller string, err error) {
	col, caller, callerID, _, err := loadForOwner(verb, name)
	if err != nil {
		return nil, "", err
	}
	if !col.IsOwner(callerID) {
		return nil, "", fmt.Errorf("%s: only %s (the owner) can manage groups in %q", verb, col.Logins[col.Owner], name)
	}
	return col, caller, nil
}

func runGroupCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	group := args[1]

	col, caller, err := requireOwner("group create", name)
	if err != nil {
		return err
	}

	if err := col.CreateGroup(group); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "group.create",
			Target:     group,
			Detail:     "Failed to create group",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("group create: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "group.create",
		Target:     group,
		Detail:     "Created group",
		Result:     "ok",
	})

	output.Success("Created group %s in %s", group, name)
	output.Suggestion(fmt.Sprintf("gitcollect group add %s %s <username>", name, group))
	return nil
}

func runGroupDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	group := args[1]

	col, caller, err := requireOwner("group delete", name)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Delete group %q from %q?", group, name)
	if !output.Confirm(prompt) {
		return fmt.Errorf("group delete: aborted")
	}

	if err := col.DeleteGroup(group); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "group.delete",
			Target:     group,
			Detail:     "Failed to delete group",
			Result:     "error: " + err.Error(),
		})
		if errors.Is(err, collection.ErrGroupInUse) {
			output.Error("%s", err.Error())
			output.Suggestion(fmt.Sprintf("gitcollect repo access %s <repo> --groups <other-groups>  # to clear %s from a blocking repo", name, group))
			return fmt.Errorf("group delete: aborted")
		}
		return fmt.Errorf("group delete: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "group.delete",
		Target:     group,
		Detail:     "Deleted group",
		Result:     "ok",
	})

	output.Success("Deleted group %s from %s", group, name)
	return nil
}

func runGroupAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	group := args[1]
	usernames := args[2:]

	col, caller, err := requireOwner("group add", name)
	if err != nil {
		return err
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return fmt.Errorf("group add: %w", err)
	}

	var failed []string
	for _, username := range usernames {
		if err := addOneToGroup(col, name, group, caller, username, client); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", username, err))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("group add: %d of %d failed: %s", len(failed), len(usernames), strings.Join(failed, "; "))
	}
	return nil
}

// addOneToGroup adds a single username to group within col, reporting and
// auditing the result. Factored out of runGroupAdd so adding several members
// to a group in one invocation can continue past an individual failure
// instead of aborting the whole batch.
func addOneToGroup(col *collection.Collection, name, group, caller, username string, client api.Client) error {
	if err := col.AddToGroup(username, group, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "member.add_to_group",
			Target:     fmt.Sprintf("%s → %s", username, group),
			Detail:     "Failed to add to group",
			Result:     "error: " + err.Error(),
		})
		if errors.Is(err, collection.ErrNotMember) {
			output.Error("group add: %q is not a member of %s", username, name)
			output.Suggestion(fmt.Sprintf("gitcollect member add %s %s", name, username))
			return errors.New("not a member")
		}
		return err
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "member.add_to_group",
		Target:     fmt.Sprintf("%s → %s", username, group),
		Detail:     fmt.Sprintf("Added %s to %s", username, group),
		Result:     "ok",
	})

	output.Success("Added %s to group %s in %s", username, group, name)
	return nil
}

func runGroupRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	group := args[1]
	username := args[2]

	col, caller, err := requireOwner("group remove", name)
	if err != nil {
		return err
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return fmt.Errorf("group remove: %w", err)
	}

	if err := col.RemoveFromGroup(username, group, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "member.remove_from_group",
			Target:     fmt.Sprintf("%s → %s", username, group),
			Detail:     "Failed to remove from group",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("group remove: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "member.remove_from_group",
		Target:     fmt.Sprintf("%s → %s", username, group),
		Detail:     fmt.Sprintf("Removed %s from %s", username, group),
		Result:     "ok",
	})

	output.Success("Removed %s from group %s in %s", username, group, name)
	return nil
}

func runGroupList(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, _, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("group list: %w", err)
	}

	if len(col.Groups) == 0 {
		output.Info("%q has no groups", name)
		return nil
	}

	rows := make([][]string, 0, len(col.Groups))
	for group, ids := range col.Groups {
		memberList := "—"
		if len(ids) > 0 {
			memberList = strings.Join(loginsFor(col, ids), ", ")
		}
		rows = append(rows, []string{group, fmt.Sprintf("%d", len(ids)), memberList})
	}
	output.Table([]string{"GROUP", "MEMBERS", "USERS"}, rows)
	return nil
}

func runGroupShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	group := args[1]

	col, _, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("group show: %w", err)
	}

	ids, ok := col.Groups[group]
	if !ok {
		return fmt.Errorf("group show: %q has no group %q", name, group)
	}

	fmt.Printf("Group:   %s\n", group)
	fmt.Printf("Members: %d\n", len(ids))
	if len(ids) > 0 {
		fmt.Println()
		rows := make([][]string, 0, len(ids))
		for _, login := range loginsFor(col, ids) {
			rows = append(rows, []string{login})
		}
		output.Table([]string{"MEMBER"}, rows)
	}

	var accessible []string
	for _, r := range col.Repos {
		for _, g := range r.Groups {
			if g == group {
				accessible = append(accessible, r.Name)
				break
			}
		}
	}
	fmt.Println()
	if len(accessible) == 0 {
		fmt.Println("Accessible repos: none directly restricted to this group")
		return nil
	}
	fmt.Printf("Accessible repos: %s\n", strings.Join(accessible, ", "))
	return nil
}
