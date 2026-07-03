package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var copyCmd = &cobra.Command{
	Use:   "copy <collection> <new-name>",
	Short: "Copy a collection to a new name",
	Args:  cobra.ExactArgs(2),
	RunE:  runCopy,
}

func init() {
	rootCmd.AddCommand(copyCmd)
}

func runCopy(cmd *cobra.Command, args []string) error {
	name, newName := args[0], args[1]

	col, caller, callerID, _, err := loadForOwner("copy", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("copy: only %s (the owner) can copy %q", col.Logins[col.Owner], name)
	}

	if err := col.SaveAs(newName); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: newName,
		Actor:      caller,
		Action:     "copy",
		Target:     name,
		Detail:     fmt.Sprintf("Copied %q to %q", name, newName),
		Result:     "ok",
	})

	output.Success("Copied %q to %q", name, newName)
	return nil
}
