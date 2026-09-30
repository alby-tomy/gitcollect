package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/access"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	moveDryRun bool
	moveGroup  string
	moveYes    bool
)

var moveCmd = &cobra.Command{
	Use:   "move <source-collection> <repo> <dest-collection>",
	Short: "Move a repo, or a whole module, from one collection to another",
	Long: `Move repositories between collections, carrying their platform access with
them: members of the destination gain access, members of only the source
lose it.

Name a single repo, or use --group to move every repo a group can reach -
a whole module handed from one team to another in one operation, rather
than repo by repo.

Because a move revokes real access for real people, it asks you to type the
name back before anything happens, the way GitHub does for deleting a repo.
--yes skips that for scripts; --dry-run previews and never prompts.

Examples:
  gitcollect move platform-team checkout-api payments-team
  gitcollect move platform-team payments-team --group pricing
  gitcollect move platform-team payments-team --group pricing --dry-run`,
	Args: moveArgs,
	RunE: runMove,
}

func init() {
	moveCmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "preview access changes without executing")
	moveCmd.Flags().StringVar(&moveGroup, "group", "", "move every repo this group can reach, instead of one named repo")
	moveCmd.Flags().BoolVar(&moveYes, "yes", false, "skip the typed confirmation")
	rootCmd.AddCommand(moveCmd)
}

// moveArgs accepts the repo form and the group form, which differ by one
// positional: --group replaces the repo name rather than adding to it.
func moveArgs(_ *cobra.Command, args []string) error {
	if moveGroup != "" {
		if len(args) != 2 {
			return fmt.Errorf("move --group takes <source-collection> <dest-collection>, got %d argument(s)", len(args))
		}
		return nil
	}
	if len(args) != 3 {
		return fmt.Errorf("move takes <source-collection> <repo> <dest-collection>, got %d argument(s)", len(args))
	}
	return nil
}

// moveDestSaveFn and moveSrcSaveFn are the functions used to persist the
// dest and source collections respectively. Replaced in tests to simulate
// write failures without filesystem manipulation.
var (
	moveDestSaveFn = func(col *collection.Collection) error { return col.Save() }
	moveSrcSaveFn  = func(col *collection.Collection) error { return col.Save() }
)

func runMove(_ *cobra.Command, args []string) error {
	var srcName, dstName, repoArg string
	if moveGroup != "" {
		srcName, dstName = args[0], args[1]
	} else {
		srcName, repoArg, dstName = args[0], args[1], args[2]
	}

	// ── Phase 1: Validate ──────────────────────────────────────────────────

	if srcName == dstName {
		return NewUsageError(fmt.Errorf("move: source and destination are both %q", srcName))
	}

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

	// Resolve what is being moved. Every repo is checked before anything is
	// mutated: a half-moved module is far worse than a refused one.
	repoNames, err := resolveMoveTargets(srcCol, repoArg, moveGroup)
	if err != nil {
		return err
	}
	for _, name := range repoNames {
		if collectionHasRepo(dstCol, name) {
			return fmt.Errorf("move: repo %q already exists in %q", name, dstName)
		}
	}

	// ── Phase 2: Calculate access diff ────────────────────────────────────

	srcMemberSet := memberIDSet(srcCol)
	dstMemberSet := memberIDSet(dstCol)

	// losingByRepo is per-repo because access is per-repo: a member may
	// reach one repo of a module and not another.
	losingByRepo := make(map[string][]string, len(repoNames))
	losingAny := make(map[string]bool)
	for _, name := range repoNames {
		for id := range srcMemberSet {
			if _, inDst := dstMemberSet[id]; inDst {
				continue
			}
			if !srcCol.CanAccessRepo(id, name) {
				continue
			}
			if login := srcCol.Logins[id]; login != "" {
				losingByRepo[name] = append(losingByRepo[name], login)
				losingAny[login] = true
			}
		}
		sort.Strings(losingByRepo[name])
	}

	var gaining, losing, unchanged []string
	for id := range dstMemberSet {
		if _, inSrc := srcMemberSet[id]; !inSrc {
			if login := dstCol.Logins[id]; login != "" {
				gaining = append(gaining, login)
			}
		}
	}
	for login := range losingAny {
		losing = append(losing, login)
	}
	for id := range srcMemberSet {
		if _, inDst := dstMemberSet[id]; inDst {
			if login := srcCol.Logins[id]; login != "" {
				unchanged = append(unchanged, login)
			}
		}
	}
	sort.Strings(gaining)
	sort.Strings(losing)
	sort.Strings(unchanged)

	// ── Phase 3: Preview ──────────────────────────────────────────────────

	what := repoArg
	if moveGroup != "" {
		what = fmt.Sprintf("module %q (%d repo%s)", moveGroup, len(repoNames), plural(len(repoNames), "", "s"))
	}
	fmt.Printf("Moving %s: %s → %s\n\n", what, srcName, dstName)
	if moveGroup != "" {
		fmt.Println("Repos:")
		for _, name := range repoNames {
			fmt.Printf("  • %s\n", name)
		}
		fmt.Println()
	}
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

	// A move revokes real access on real repositories, so it is confirmed
	// the way GitHub confirms deleting one: by typing the name back. The
	// word is the module name for a group move and the repo name otherwise,
	// so what you type is what you are actually moving.
	confirmWord := repoArg
	if moveGroup != "" {
		confirmWord = moveGroup
	}
	if !moveYes && !moveConfirmFn(fmt.Sprintf("This moves %s out of %q", what, srcName), confirmWord) {
		output.Info("Not moving.")
		return nil
	}

	// ── Phase 4: Execute ──────────────────────────────────────────────────

	fmt.Println("Applying...")

	// Backup source state before any mutation (for rollback).
	sourceBackup := *srcCol
	backupRepos := append([]collection.RepoAccess(nil), srcCol.Repos...)
	sourceBackup.Repos = backupRepos

	for _, name := range repoNames {
		dstCol.Repos = append(dstCol.Repos, collection.RepoAccess{
			Name:   name,
			Groups: []string{},
			Users:  []string{},
		})
	}
	dstCol.UpdatedAt = time.Now().UTC()

	// One sync covers every repo just added.
	if _, _, syncErr := access.SyncCollaborators(dstCol, client, false); syncErr != nil {
		output.Warn("could not sync collaborator access for %s: %v", dstName, syncErr)
	} else if len(gaining) > 0 {
		output.Dim("  ✓ Granted access: %s", joinLogins(gaining))
	}

	for _, name := range repoNames {
		srcCol.Repos = removeRepoByName(srcCol.Repos, name)
	}
	srcCol.UpdatedAt = time.Now().UTC()

	ns := srcCol.RepoNamespace()
	for _, name := range repoNames {
		for _, login := range losingByRepo[name] {
			if rmErr := client.RemoveCollaborator(ns, name, login); rmErr != nil {
				output.Warn("could not revoke %s from %s/%s: %v", login, ns, name, rmErr)
			}
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
				joinLogins(repoNames), srcName)
			return errors.Join(err, rollbackErr)
		}
		return fmt.Errorf("move: could not save %q (dest written, source rolled back): %w", srcName, err)
	}

	// ── Audit ──────────────────────────────────────────────────────────────

	for _, name := range repoNames {
		detail := fmt.Sprintf("Moved %q from %q to %q", name, srcName, dstName)
		if moveGroup != "" {
			detail = fmt.Sprintf("Moved %q from %q to %q as part of module %q", name, srcName, dstName, moveGroup)
		}
		recordAudit(audit.AuditEntry{
			Collection: srcName,
			Actor:      caller,
			Action:     "repo.move.out",
			Target:     name,
			Detail:     detail,
			Result:     "ok",
		})
		recordAudit(audit.AuditEntry{
			Collection: dstName,
			Actor:      caller,
			Action:     "repo.move.in",
			Target:     name,
			Detail:     fmt.Sprintf("Received %q moved from %q", name, srcName),
			Result:     "ok",
		})
	}

	if moveGroup != "" {
		output.Success("module %q (%d repo%s) moved to %s", moveGroup,
			len(repoNames), plural(len(repoNames), "", "s"), dstName)
	} else {
		output.Success("%s moved to %s", repoArg, dstName)
	}
	output.Suggestion(fmt.Sprintf("gitcollect show %s  to verify", dstName))
	return nil
}

// moveConfirmFn is the typed confirmation, injectable so tests can drive it
// without a terminal.
var moveConfirmFn = func(prompt, word string) bool { return output.ConfirmWord(prompt, word) }

// resolveMoveTargets returns the repos a move should carry: one named repo,
// or every repo the named group can reach.
//
// Moving a module is the operation a team hand-off actually needs — "these
// twenty pricing repos now belong to that team" — and doing it repo by repo
// is both tedious and easy to leave half-done.
func resolveMoveTargets(srcCol *collection.Collection, repoArg, group string) ([]string, error) {
	if group == "" {
		if !collectionHasRepo(srcCol, repoArg) {
			return nil, fmt.Errorf("move: repo %q not found in %q", repoArg, srcCol.Name)
		}
		return []string{repoArg}, nil
	}

	if _, ok := srcCol.Groups[group]; !ok {
		return nil, fmt.Errorf("move: group %q not found in %q\n  Run: gitcollect show %s",
			group, srcCol.Name, srcCol.Name)
	}

	var names []string
	for _, r := range srcCol.Repos {
		for _, g := range r.Groups {
			if g == group {
				names = append(names, r.Name)
				break
			}
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("move: group %q reaches no repos in %q — nothing to move",
			group, srcCol.Name)
	}
	sort.Strings(names)
	return names, nil
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
