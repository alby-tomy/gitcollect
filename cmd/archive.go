package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/output"
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
	Long:  `Clears archived: true from a collection's YAML manifest, making it visible again in list, sync --all, and status --all.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runUnarchive,
}

func init() {
	rootCmd.AddCommand(archiveCmd)
	rootCmd.AddCommand(unarchiveCmd)
}

func runArchive(_ *cobra.Command, args []string) error {
	name := args[0]
	col, _, _, _, err := loadForOwner("archive", name)
	if err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	if col.Archived {
		output.Info("%s is already archived", name)
		return nil
	}
	col.Archived = true
	if err := col.Save(); err != nil {
		return fmt.Errorf("archive: could not save: %w", err)
	}
	output.Success("Archived %s", name)
	output.Dim("  Pass --include-archived to list/sync/status to include it.")
	return nil
}

func runUnarchive(_ *cobra.Command, args []string) error {
	name := args[0]
	col, _, _, _, err := loadForOwner("unarchive", name)
	if err != nil {
		return fmt.Errorf("unarchive: %w", err)
	}
	if !col.Archived {
		output.Info("%s is not archived", name)
		return nil
	}
	col.Archived = false
	if err := col.Save(); err != nil {
		return fmt.Errorf("unarchive: could not save: %w", err)
	}
	output.Success("Unarchived %s", name)
	return nil
}

