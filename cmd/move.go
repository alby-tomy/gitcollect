package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/access"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var moveDryRun bool

var moveCmd = &cobra.Command{
	Use:   "move <source-collection> <repo> <dest-collection>",
	Short: "Move a repo from one collection to another",
	Args:  cobra.ExactArgs(3),
	RunE:  runMove,
}

func init() {
	moveCmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "preview access changes without executing")
	rootCmd.AddCommand(moveCmd)
}

// moveDestSaveFn and moveSrcSaveFn are the functions used to persist the
// dest and source collections respectively. Replaced in tests to simulate
// write failures without filesystem manipulation.
var (
	moveDestSaveFn = func(col *collection.Collection) error { return col.Save() }
	moveSrcSaveFn  = func(col *collection.Collection) error { return col.Save() }
)

func runMove(cmd *cobra.Command, args []string) error {
	srcName, repoName, dstName := args[0], args[1], args[2]

	// ── Phase 1: Validate ──────────────────────────────────────────────────

	srcCol, caller, callerID, client, err := loadForOwner("move", srcName)
	if err != nil {
		return err
	}
	if !srcCol.IsOwner(callerID) {
		return fmt.Errorf("move: only %s (the owner) can move repos from %q",
			srcCol.Logins[srcCol.Owner], srcName)
	}

	dstCol, err := collection.Load(dstName)
	if err != nil {
		return fmt.Errorf("move: dest collection %q: %w", dstName, err)
	}
	if !dstCol.IsOwner(callerID) {
		return fmt.Errorf("move: you must be owner of both %q and %q — %s does not own %q",
			srcName, dstName, caller, dstName)
	}

	if !collectionHasRepo(srcCol, repoName) {
		return fmt.Errorf("move: repo %q not found in %q", repoName, srcName)
	}
	if collectionHasRepo(dstCol, repoName) {
		return fmt.Errorf("move: repo %q already exists in %q", repoName, dstName)
	}

	// ── Phase 2: Calculate access diff ────────────────────────────────────

	srcMemberSet := memberIDSet(srcCol)
	dstMemberSet := memberIDSet(dstCol)

	var gaining, losing, unchanged []string
	for id := range dstMemberSet {
		if _, inSrc := srcMemberSet[id]; !inSrc {
			if login := dstCol.Logins[id]; login != "" {
				gaining = append(gaining, login)
			}
		}
	}
	for id := range srcMemberSet {
		if _, inDst := dstMemberSet[id]; !inDst {
			if srcCol.CanAccessRepo(id, repoName) {
				if login := srcCol.Logins[id]; login != "" {
					losing = append(losing, login)
				}
			}
		}
	}
	for id := range srcMemberSet {
		if _, inDst := dstMemberSet[id]; inDst {
			if login := srcCol.Logins[id]; login != "" {
				unchanged = append(unchanged, login)
			}
		}
	}

	// ── Phase 3: Preview ──────────────────────────────────────────────────

	fmt.Printf("Moving %s: %s → %s\n\n", repoName, srcName, dstName)
	fmt.Println("Access changes:")
	if len(gaining) > 0 {
		fmt.Printf("  + Gaining access (in %s, not in %s):\n", dstName, srcName)
		fmt.Printf("    %s (%d member(s))\n", joinLogins(gaining), len(gaining))
	}
	if len(losing) > 0 {
		fmt.Printf("  - Losing access (in %s, not in %s):\n", srcName, dstName)
		fmt.Printf("    %s (%d member(s))\n", joinLogins(losing), len(losing))
	}
	if len(unchanged) > 0 {
		fmt.Printf("  = No change (in both collections):\n")
		fmt.Printf("    %s (%d member(s))\n", joinLogins(unchanged), len(unchanged))
	}
	if len(gaining) == 0 && len(losing) == 0 && len(unchanged) == 0 {
		fmt.Println("  (no member changes)")
	}
	fmt.Println()

	if moveDryRun {
		output.Info("dry-run: no changes made")
		return nil
	}

	// ── Phase 4: Execute ──────────────────────────────────────────────────

	fmt.Println("Applying...")

	// Backup source state before any mutation (for rollback).
	sourceBackup := *srcCol

	// Add repo to dest collection with open access.
	dstCol.Repos = append(dstCol.Repos, collection.RepoAccess{
		Name:   repoName,
		Groups: []string{},
		Users:  []string{},
	})
	dstCol.UpdatedAt = time.Now().UTC()

	// Grant access to dest members who are gaining (dest-only members).
	if _, _, syncErr := access.SyncCollaborators(dstCol, client, false); syncErr != nil {
		output.Warn("could not sync collaborator access for %s: %v", dstName, syncErr)
	} else if len(gaining) > 0 {
		output.Dim("  ✓ Granted access: %s", joinLogins(gaining))
	}

	// Remove repo from source collection.
	srcCol.Repos = removeRepoByName(srcCol.Repos, repoName)
	srcCol.UpdatedAt = time.Now().UTC()

	// Revoke access for members losing access (src-only members who had access).
	ns := srcCol.RepoNamespace()
	for id := range srcMemberSet {
		if _, inDst := dstMemberSet[id]; inDst {
			continue // shared member — no change
		}
		if !sourceBackup.CanAccessRepo(id, repoName) {
			continue // didn't have access anyway
		}
		login := srcCol.Logins[id]
		if login == "" {
			continue
		}
		if rmErr := client.RemoveCollaborator(ns, repoName, login); rmErr != nil {
			output.Warn("could not revoke %s from %s/%s: %v", login, ns, repoName, rmErr)
		}
	}
	if len(losing) > 0 {
		output.Dim("  ✓ Revoked access: %s", joinLogins(losing))
	}

	// ── Phase 5: Save both collections atomically ─────────────────────────

	if err := moveDestSaveFn(dstCol); err != nil {
		// Dest write failed — nothing persisted yet, abort cleanly.
		return fmt.Errorf("move: could not save %q: %w", dstName, err)
	}

	if err := moveSrcSaveFn(srcCol); err != nil {
		// Dest is written but source isn't — attempt rollback.
		*srcCol = sourceBackup
		rollbackErr := srcCol.Save()
		if rollbackErr != nil {
			// Cannot auto-recover — tell user exactly what to do.
			fmt.Fprintf(os.Stderr, "✗ move: critical: source collection may be in inconsistent state\n")
			fmt.Fprintf(os.Stderr, "  dest collection was written but source collection could not be updated\n")
			fmt.Fprintf(os.Stderr, "  Manual fix: remove %s from ~/.gitcollect/collections/%s.yaml\n",
				repoName, srcName)
			return errors.Join(err, rollbackErr)
		}
		return fmt.Errorf("move: could not save %q (dest written, source rolled back): %w", srcName, err)
	}

	// ── Audit ──────────────────────────────────────────────────────────────

	recordAudit(audit.AuditEntry{
		Collection: srcName,
		Actor:      caller,
		Action:     "repo.move.out",
		Target:     repoName,
		Detail:     fmt.Sprintf("Moved %q from %q to %q", repoName, srcName, dstName),
		Result:     "ok",
	})
	recordAudit(audit.AuditEntry{
		Collection: dstName,
		Actor:      caller,
		Action:     "repo.move.in",
		Target:     repoName,
		Detail:     fmt.Sprintf("Received %q moved from %q", repoName, srcName),
		Result:     "ok",
	})

	output.Success("%s moved to %s", repoName, dstName)
	output.Suggestion(fmt.Sprintf("gitcollect show %s  to verify", dstName))
	return nil
}

// collectionHasRepo returns true if col has a repo named name.
func collectionHasRepo(col *collection.Collection, name string) bool {
	for _, r := range col.Repos {
		if r.Name == name {
			return true
		}
	}
	return false
}

// removeRepoByName returns a new slice with the named repo removed.
func removeRepoByName(repos []collection.RepoAccess, name string) []collection.RepoAccess {
	out := make([]collection.RepoAccess, 0, len(repos)-1)
	for _, r := range repos {
		if r.Name != name {
			out = append(out, r)
		}
	}
	return out
}

// memberIDSet builds a set of member IDs from a collection.
func memberIDSet(col *collection.Collection) map[string]struct{} {
	ids := col.MemberIDs()
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// joinLogins joins a slice of login names with ", ".
func joinLogins(logins []string) string {
	result := ""
	for i, l := range logins {
		if i > 0 {
			result += ", "
		}
		result += l
	}
	return result
}
