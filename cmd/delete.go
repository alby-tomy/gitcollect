package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

const deleteMaxConcurrency = 4

var deleteDryRun bool

var deleteCmd = &cobra.Command{
	Use:   "delete <collection>",
	Short: "Delete a collection and revoke all access to its repos",
	Long: `Delete a collection and revoke every member's platform access.

Before deleting, gitcollect shows the full impact: how many members and
repos are affected. You must confirm before changes are made.

All platform collaborator grants are revoked via the GitHub/GitLab API
before the local YAML is deleted. The repos themselves are not deleted
on the platform — only access is revoked.

Examples:
  gitcollect delete my-project
  gitcollect delete my-project --dry-run

If you want to recover the collection after accidental deletion, restore
the YAML from backup and run gitcollect sync to re-apply platform access.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "preview impact without deleting")
	rootCmd.AddCommand(deleteCmd)
}

// printDeletionImpact writes an impact preview for collection delete,
// listing the members whose access will be revoked.
func printDeletionImpact(w io.Writer, col *collection.Collection) {
	if len(col.Members) == 0 {
		return
	}
	logins := make([]string, 0, len(col.Members))
	for _, id := range col.Members {
		if login := col.Logins[id]; login != "" {
			logins = append(logins, login)
		} else {
			logins = append(logins, id)
		}
	}
	fmt.Fprintf(w, "  Members whose access will be revoked: %s\n", strings.Join(logins, ", "))
}

func runDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, caller, callerID, client, err := loadForOwner("delete", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("delete: only %s (the owner) can delete %q", col.Logins[col.Owner], name)
	}

	printDeletionImpact(os.Stderr, col)

	if deleteDryRun {
		output.Info("[dry-run] Would delete %q and revoke access for %d member(s) across %d repo(s)",
			name, len(col.Members), len(col.Repos))
		return nil
	}

	prompt := fmt.Sprintf("This will delete %q and revoke access for %d member(s) across %d repo(s)", name, len(col.Members), len(col.Repos))
	if !output.ConfirmWord(prompt, name) {
		return fmt.Errorf("delete: aborted")
	}

	if err := revokeCollectionAccess(col, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "delete",
			Target:     name,
			Detail:     "Failed to revoke all access before delete",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("delete: could not revoke all access: %w", err)
	}

	if err := col.Delete(); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "delete",
		Target:     name,
		Detail:     "Collection deleted, all access revoked",
		Result:     "ok",
	})

	output.Success("Deleted collection %q and revoked access for %d member(s)", name, len(col.Members))
	return nil
}

// revokeCollectionAccess unconditionally removes every member's
// collaborator access from every repo in col, concurrently (max 4 at a
// time), before the manifest itself is deleted.
func revokeCollectionAccess(col *collection.Collection, client api.Client) error {
	type pair struct{ memberLogin, repo string }
	ownerLogin := col.RepoNamespace()
	var pairs []pair
	for _, m := range col.Members {
		for _, r := range col.Repos {
			pairs = append(pairs, pair{memberLogin: col.Logins[m], repo: r.Name})
		}
	}

	var (
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, deleteMaxConcurrency)
		wg   sync.WaitGroup
	)
	for _, p := range pairs {
		wg.Add(1)
		sem <- struct{}{}
		go func(p pair) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := client.RemoveCollaborator(ownerLogin, p.repo, p.memberLogin); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: revoke %s: %w", ownerLogin, p.repo, p.memberLogin, err))
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
