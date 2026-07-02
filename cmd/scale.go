package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var scaleCmd = &cobra.Command{
	Use:   "scale <collection> organisation|team",
	Short: "Switch a collection between TEAM and ORGANISATION tiers",
	Args:  cobra.ExactArgs(2),
	RunE:  runScale,
}

func init() {
	rootCmd.AddCommand(scaleCmd)
}

func runScale(cmd *cobra.Command, args []string) error {
	name := args[0]
	tier := strings.ToLower(args[1])

	if tier != "organisation" && tier != "team" {
		return NewUsageError(fmt.Errorf("scale: tier must be \"organisation\" or \"team\", got %q", args[1]))
	}

	col, caller, err := requireOwner("scale", name)
	if err != nil {
		return err
	}

	switch tier {
	case "organisation":
		if col.GroupAdminsEnabled {
			output.Info("Group admin support is already enabled for %s", name)
			return nil
		}
		col.GroupAdminsEnabled = true
		if err := col.Save(); err != nil {
			return fmt.Errorf("scale: %w", err)
		}

		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "scale.organisation",
			Target:     name,
			Detail:     "Group admin support enabled",
			Result:     "ok",
		})

		output.Success("Group admin support enabled for %s", name)
		if len(col.GroupAdmins) == 0 {
			output.Dim("  No group admins are assigned yet.")
			output.Suggestion(fmt.Sprintf("gitcollect group admin add %s <group> <username>", name))
		} else {
			output.Dim("  Existing group admin assignments are now active.")
		}

	case "team":
		if !col.GroupAdminsEnabled {
			output.Info("Group admin support is already disabled for %s", name)
			return nil
		}

		// Count and list who loses admin rights.
		adminCount := 0
		var lostRights []string
		groupNames := make([]string, 0, len(col.GroupAdmins))
		for g := range col.GroupAdmins {
			groupNames = append(groupNames, g)
		}
		sort.Strings(groupNames)
		for _, group := range groupNames {
			admins := col.GroupAdmins[group]
			adminCount += len(admins)
			logins := make([]string, 0, len(admins))
			for _, id := range admins {
				login := col.Logins[id]
				if login == "" {
					login = id
				}
				logins = append(logins, login)
			}
			lostRights = append(lostRights, fmt.Sprintf("  %s: %s", group, strings.Join(logins, ", ")))
		}

		if len(lostRights) > 0 {
			output.Warn("This will remove group admin privileges from:")
			for _, line := range lostRights {
				fmt.Println(line)
			}
			fmt.Printf("  They will become regular members. Only you (%s) can manage groups.\n\n", caller)
			if !output.Confirm("Continue?") {
				return fmt.Errorf("scale: aborted")
			}
		}

		col.GroupAdminsEnabled = false
		col.GroupAdmins = nil
		if err := col.Save(); err != nil {
			return fmt.Errorf("scale: %w", err)
		}

		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "scale.team",
			Target:     name,
			Detail:     fmt.Sprintf("Group admin support disabled; %d admins revoked", adminCount),
			Result:     "ok",
		})

		output.Success("Group admin support disabled for %s", name)
	}

	return nil
}
