package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var archiveCmd = &cobra.Command{
	Use:   "archive <collection>",
	Short: "Mark a collection as archived (hidden from list/sync/status --all)",
	Long: `Soft-archives a collection: sets archived: true in its YAML manifest.

Archived collections are excluded from list, sync --all, and status --all
unless --include-archived is passed. The YAML file and all its repos remain
on disk — archive is not delete.

Only the collection owner can archive it.`,
	Args: cobra.ExactArgs(1),
	RunE: runArchive,
}

var unarchiveCmd = &cobra.Command{
	Use:   "unarchive <collection>",
	Short: "Remove the archived flag from a collection",
	Long: `Clears archived: true from a collection's YAML manifest, making it visible
again in list, sync --all, and status --all.

Only the collection owner can unarchive it.`,
	Args: cobra.ExactArgs(1),
	RunE: runUnarchive,
}

func init() {
	rootCmd.AddCommand(archiveCmd)
	rootCmd.AddCommand(unarchiveCmd)
}

// setArchived loads name, confirms the caller owns it, and sets the
// Archived flag to want. Reports whether it actually changed anything, so
// the caller can print "Archived x" only when it did and "already
// archived" otherwise. Shared by both commands, which differ only in the
// target value and their wording.
//
// The ownership check lives here rather than being assumed from
// loadForOwner: that helper resolves the caller and migrates the file but
// deliberately leaves the ownership test to each command (see its doc
// comment). Both commands previously discarded the caller ID it returns
// and never tested it, so any member could hide a collection from
// everyone's list/sync/status despite the help text promising otherwise.
func setArchived(verb, name string, want bool) (changed bool, err error) {
	col, caller, callerID, _, err := loadForOwner(verb, name)
	if err != nil {
		return false, err
	}
	if !col.IsOwner(callerID) {
		return false, fmt.Errorf("%s: only %s (the owner) can %s %q",
			verb, col.Logins[col.Owner], verb, name)
	}

	if col.Archived == want {
		return false, nil
	}

	col.Archived = want
	entry := audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "collection." + verb,
		Target:     name,
	}
	if err := col.Save(); err != nil {
		entry.Detail = "Failed to save"
		entry.Result = "error: " + err.Error()
		recordAudit(entry)
		return false, fmt.Errorf("%s: could not save: %w", verb, err)
	}

	entry.Detail = fmt.Sprintf("Set archived=%t", want)
	entry.Result = "ok"
	recordAudit(entry)
	return true, nil
}

func runArchive(_ *cobra.Command, args []string) error {
	name := args[0]
	changed, err := setArchived("archive", name, true)
	if err != nil {
		return err
	}
	if !changed {
		output.Info("%s is already archived", name)
		return nil
	}
	output.Success("Archived %s", name)
	output.Dim("  Pass --include-archived to list/sync/status to include it.")
	return nil
}

func runUnarchive(_ *cobra.Command, args []string) error {
	name := args[0]
	changed, err := setArchived("unarchive", name, false)
	if err != nil {
		return err
	}
	if !changed {
		output.Info("%s is not archived", name)
		return nil
	}
	output.Success("Unarchived %s", name)
	return nil
}
