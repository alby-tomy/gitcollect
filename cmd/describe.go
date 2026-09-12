package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var describeCmd = &cobra.Command{
	Use:   "describe <collection> [description]",
	Short: "Set or clear the description of a collection",
	Long: `Set a free-text description on a collection (owner only).
Pass an empty string or omit the argument to clear the existing description.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) error {
	name := args[0]
	var newDesc string
	if len(args) == 2 {
		newDesc = args[1]
	}

	col, caller, callerID, _, err := loadForOwner("describe", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("describe: only %s (the owner) can update the description of %q",
			col.Logins[col.Owner], name)
	}

	col.Description = newDesc
	col.UpdatedAt = time.Now()
	if err := col.Save(); err != nil {
		return fmt.Errorf("describe: %w", err)
	}

	detail := "Description updated"
	if newDesc == "" {
		detail = "Cleared"
		output.Success("Description cleared for %q", name)
	} else {
		output.Success("Description updated for %q: %s", name, newDesc)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "collection.describe",
		Target:     name,
		Detail:     detail,
		Result:     "ok",
	})

	return nil
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
