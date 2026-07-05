package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var renameCmd = &cobra.Command{
	Use:   "rename <collection> <new-name>",
	Short: "Rename a collection",
	Args:  cobra.ExactArgs(2),
	RunE:  runRename,
}

func init() {
	rootCmd.AddCommand(renameCmd)
}

func runRename(cmd *cobra.Command, args []string) error {
	name, newName := args[0], args[1]

	col, caller, callerID, _, err := loadForOwner("rename", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("rename: only %s (the owner) can rename %q", col.Logins[col.Owner], name)
	}

	oldPath := col.Path()
	if err := col.SaveAs(newName); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename: could not remove old manifest: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: newName,
		Actor:      caller,
		Action:     "rename",
		Target:     name,
		Detail:     fmt.Sprintf("Renamed %q to %q", name, newName),
		Result:     "ok",
	})

	output.Success("Renamed %q to %q", name, newName)
	return nil
}
