package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var visibilityCmd = &cobra.Command{
	Use:   "visibility <collection> <public|private>",
	Short: "Change a collection's visibility",
	Args:  cobra.ExactArgs(2),
	RunE:  runVisibility,
}

func init() {
	rootCmd.AddCommand(visibilityCmd)
}

func runVisibility(cmd *cobra.Command, args []string) error {
	name := args[0]
	want := args[1]

	var newVisibility collection.Visibility
	switch want {
	case string(collection.VisibilityPublic):
		newVisibility = collection.VisibilityPublic
	case string(collection.VisibilityPrivate):
		newVisibility = collection.VisibilityPrivate
	default:
		return NewUsageError(fmt.Errorf("visibility: invalid visibility %q: must be %q or %q", want, collection.VisibilityPublic, collection.VisibilityPrivate))
	}

	col, caller, callerID, _, err := loadForOwner("visibility", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("visibility: only %s (the owner) can change visibility of %q", col.Logins[col.Owner], name)
	}

	old := col.Visibility
	target := fmt.Sprintf("%s→%s", old, newVisibility)

	if old == newVisibility {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "visibility.change",
			Target:     target,
			Detail:     "No change",
			Result:     "ok",
		})
		output.Info("visibility of %q is already %s", name, newVisibility)
		return nil
	}

	if newVisibility == collection.VisibilityPublic {
		prompt := fmt.Sprintf("This will make %q public — anyone can discover it exists", name)
		if !output.Confirm(prompt) {
			return fmt.Errorf("visibility: aborted")
		}
	}

	col.Visibility = newVisibility
	if err := col.Save(); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "visibility.change",
			Target:     target,
			Detail:     "Failed to save",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("visibility: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "visibility.change",
		Target:     target,
		Detail:     fmt.Sprintf("Changed visibility from %s to %s", old, newVisibility),
		Result:     "ok",
	})

	output.Success("Changed %q visibility: %s → %s", name, old, newVisibility)
	return nil
}
