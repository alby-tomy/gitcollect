package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/git"
	"github.com/alby-tomy/gitcollect/internal/output"
)

const defaultSyncConcurrency = 4

var (
	syncDryRun             bool
	syncConcurrency        int
	syncDest               string
	syncAll                bool
	syncIncludeArchived    bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [collection]",
	Short: "Clone every repo not yet present locally, pull every repo that already is",
	Long: `One command instead of two: for every accessible repo, sync clones it if
it isn't present yet at --dest, or runs "git pull" if it already is.
Equivalent to running clone and pull back to back, but in a single pass
and a single access check.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "preview what would be cloned/pulled without doing it")
	syncCmd.Flags().IntVar(&syncConcurrency, "concurrency", defaultSyncConcurrency, "max repos to sync in parallel")
	syncCmd.Flags().StringVar(&syncDest, "dest", ".", "directory to clone into, or where repos were already cloned")
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "run across all collections you own or belong to")
	syncCmd.Flags().BoolVar(&syncIncludeArchived, "include-archived", false, "include archived collections when using --all")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	if syncAll && len(args) > 0 {
		return NewUsageError(fmt.Errorf("sync: use either <collection> or --all, not both"))
	}
	if !syncAll && len(args) == 0 {
		return NewUsageError(fmt.Errorf("sync: provide a collection name or use --all"))
	}
	if syncAll {
		return runSyncAll(syncDest, syncDryRun, syncConcurrency)
	}
	name := args[0]

	if syncConcurrency < 1 {
		return NewUsageError(fmt.Errorf("sync: --concurrency must be at least 1"))
	}
	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	col, caller, callerID, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	accessible, err := access.FilterAccessible(col, callerID, client)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	_, skipped, _ := selectCloneTargets(col, accessible, nil)

	printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))

	if len(accessible) == 0 {
		output.Info("no repos to sync")
		return nil
	}

	results := syncTargets(col, client, accessible, syncDest, syncConcurrency, syncDryRun)

	var synced, failed []string
	for _, r := range results {
		if r.err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", r.name, r.err))
		} else {
			synced = append(synced, r.name)
		}
	}

	if syncDryRun {
		output.Success("Dry run: would sync %d repo(s)", len(accessible))
		return nil
	}

	if len(failed) > 0 {
		output.Error("Synced %d repo(s), %d failed", len(synced), len(failed))
		for _, f := range failed {
			output.Dim("  ✗ %s", f)
		}
	} else {
		output.Success("Synced %d repo(s)", len(synced))
		if absPath, err := filepath.Abs(syncDest); err == nil {
			output.Dim("  Location: %s", absPath)
		}
	}

	if len(skipped) > 0 {
		fmt.Printf("  %d repo(s) skipped (no access): %s\n", len(skipped), strings.Join(skipped, ", "))
		output.Suggestion(fmt.Sprintf("gitcollect inspect %s --user %s", name, caller))
	}

	if len(failed) > 0 {
		return fmt.Errorf("sync: %d repo(s) failed", len(failed))
	}
	return nil
}

// runSyncAll iterates every collection the caller can reach and runs sync on each.
func runSyncAll(destDir string, dryRun bool, concurrency int) error {
	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("sync --all: %w", err)
	}
	if len(names) == 0 {
		output.Info("no collections found")
		return nil
	}

	var anyFailed bool
	for _, name := range names {
		col, caller, callerID, client, err := loadForGit(name)
		if err != nil {
			output.Dim("Skipping %s: %v", name, err)
			continue
		}
		if col.Archived && !syncIncludeArchived {
			output.Dim("Skipping %s (archived)", name)
			continue
		}
		output.Info("── %s ──", name)

		accessible, err := access.FilterAccessible(col, callerID, client)
		if err != nil {
			output.Warn("  %s: %v", name, err)
			anyFailed = true
			continue
		}
		printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))

		if len(accessible) == 0 {
			output.Info("  no repos to sync")
			continue
		}

		results := syncTargets(col, client, accessible, destDir, concurrency, dryRun)
		var synced, failed int
		for _, r := range results {
			if r.err != nil {
				failed++
			} else {
				synced++
			}
		}
		if dryRun {
			output.Success("  Dry run: would sync %d repo(s)", len(accessible))
		} else {
			output.Success("  Synced %d repo(s)", synced)
		}
		if failed > 0 {
			anyFailed = true
			output.Error("  %d failed", failed)
		}
	}

	if anyFailed {
		return fmt.Errorf("sync --all: some collections had failures")
	}
	return nil
}

// syncKind is which of the two operations syncOne actually performed for
// a given repo, decided purely by whether it was already present at dest.
type syncKind int

const (
	syncKindClone syncKind = iota
	syncKindPull
)

type syncResult struct {
	name       string
	kind       syncKind
	newCommits int // only meaningful for syncKindPull on a real (non-dry-run) run
	duration   time.Duration
	err        error
}

// syncTargets syncs targets into dest, at most concurrency at a time,
// printing one progress line per repo as it completes.
func syncTargets(col *collection.Collection, client api.Client, targets []collection.RepoAccess, dest string, concurrency int, dryRun bool) []syncResult {
	results := make([]syncResult, len(targets))

	var (
		mu   sync.Mutex
		done int
		sem  = make(chan struct{}, concurrency)
		wg   sync.WaitGroup
	)

	for i, repo := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, repo collection.RepoAccess) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			kind, newCommits, err := syncOne(col, client, repo.Name, dest, dryRun)
			dur := time.Since(start)

			result := syncResult{name: repo.Name, kind: kind, newCommits: newCommits, duration: dur, err: err}

			mu.Lock()
			done++
			fmt.Printf("[%d/%d] %s\n", done, len(targets), formatSyncLine(result, dryRun))
			results[i] = result
			mu.Unlock()
		}(i, repo)
	}
	wg.Wait()
	return results
}

// syncOne clones repoName if it isn't present at <dest>/<repoName>, or
// pulls it if it is. In dry-run mode it only checks which of the two it
// would do — no git command runs.
func syncOne(col *collection.Collection, client api.Client, repoName, dest string, dryRun bool) (kind syncKind, newCommits int, err error) {
	dir := filepath.Join(dest, repoName)
	if !isDir(dir) {
		if dryRun {
			return syncKindClone, 0, nil
		}
		err := cloneOne(col, client, repoName, dest, false)
		return syncKindClone, 0, err
	}

	if dryRun {
		return syncKindPull, 0, nil
	}
	n, err := git.PullWithSummary(dir)
	return syncKindPull, n, err
}

// formatSyncLine renders one repo's sync.go progress line: name, what's
// about to happen / happened, and the result.
func formatSyncLine(r syncResult, dryRun bool) string {
	verb := "already cloned → pulling..."
	if r.kind == syncKindClone {
		verb = "not cloned     → cloning..."
	}

	var status string
	switch {
	case dryRun:
		status = "would sync"
	case r.err != nil:
		status = "✗ failed: " + r.err.Error()
	case r.kind == syncKindClone:
		status = fmt.Sprintf("✓ done (%.1fs)", r.duration.Seconds())
	case r.newCommits == 0:
		status = "✓ up to date"
	default:
		status = fmt.Sprintf("✓ %d new commit(s)", r.newCommits)
	}

	return fmt.Sprintf("%-20s %-28s %s", r.name, verb, status)
}
