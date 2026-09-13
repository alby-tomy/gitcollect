package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/access"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/git"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	pullDest   string
	pullPrune  bool
	pullDryRun bool
	pullAll    bool

	// pruneHasUncommittedChangesFn is injectable so tests can control dirty-repo detection.
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) {
		return git.HasUncommittedChanges(dir)
	}
	// pruneIsTerminalFn is injectable so tests can simulate interactive mode.
	pruneIsTerminalFn = func() bool {
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
	// pruneConfirmFn is injectable so tests can control the y/N prompt.
	pruneConfirmFn = func(msg string) bool {
		return output.Confirm(msg)
	}
)

var pullCmd = &cobra.Command{
	Use:   "pull [collection]",
	Short: "git pull inside every accessible repo that's already cloned",
	Long: `Run git pull inside every repo from a collection that is already
cloned locally. Repos that are not cloned are skipped — use
gitcollect clone or gitcollect sync to clone them first.

Examples:
  gitcollect pull my-project
  gitcollect pull my-project --dest ~/projects
  gitcollect pull my-project --prune
  gitcollect pull --all

--prune removes local clones of repos that are no longer in the collection.
--all pulls across every collection you are a member of.

See also:
  gitcollect clone   — initial clone of all accessible repos
  gitcollect sync    — clone missing + pull existing in one pass`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVar(&pullDest, "dest", ".", "directory repos were cloned into")
	pullCmd.Flags().BoolVar(&pullPrune, "prune", false, "prompt to delete local clones of repos no longer in this collection")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "preview prune operations without executing")
	pullCmd.Flags().BoolVar(&pullAll, "all", false, "run across all collections you own or belong to")
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	if pullAll && len(args) > 0 {
		return NewUsageError(fmt.Errorf("pull: use either <collection> or --all, not both"))
	}
	if !pullAll && len(args) == 0 {
		return NewUsageError(fmt.Errorf("pull: provide a collection name or use --all"))
	}
	if pullAll {
		return runPullAll(pullDest)
	}
	name := args[0]

	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	col, caller, callerID, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	accessible, err := access.FilterAccessible(col, callerID, client)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))

	var pulled, missing, failed []string
	for _, repo := range accessible {
		dir := filepath.Join(pullDest, repo.Name)
		if !isDir(dir) {
			missing = append(missing, repo.Name)
			continue
		}
		if err := git.Pull(dir); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", repo.Name, err))
			continue
		}
		pulled = append(pulled, repo.Name)
	}

	output.Success("Pulled %d repo(s)", len(pulled))
	if len(missing) > 0 {
		output.Info("%d repo(s) not cloned locally, skipped: %v", len(missing), missing)
		output.Suggestion(fmt.Sprintf("gitcollect clone %s --pick %q", name, strings.Join(missing, " ")))
	}

	var pullErr error
	if len(failed) > 0 {
		output.Error("%d repo(s) failed to pull:", len(failed))
		for _, f := range failed {
			output.Dim("  ✗ %s", f)
		}
		pullErr = fmt.Errorf("pull: %d repo(s) failed", len(failed))
	}

	if pullPrune {
		if err := runPrune(col, pullDest, pullDryRun); err != nil {
			return err
		}
	}

	return pullErr
}

// runPullAll iterates every collection the caller can reach (determined by
// whether credentials exist for its host) and runs a pull on each.
func runPullAll(destDir string) error {
	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("pull --all: %w", err)
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
		output.Info("── %s ──", name)

		accessible, err := access.FilterAccessible(col, callerID, client)
		if err != nil {
			output.Warn("  %s: %v", name, err)
			anyFailed = true
			continue
		}
		printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))

		var pulled, missing, failed []string
		for _, repo := range accessible {
			dir := filepath.Join(destDir, repo.Name)
			if !isDir(dir) {
				missing = append(missing, repo.Name)
				continue
			}
			if err := git.Pull(dir); err != nil {
				failed = append(failed, fmt.Sprintf("%s (%v)", repo.Name, err))
				continue
			}
			pulled = append(pulled, repo.Name)
		}

		output.Success("  Pulled %d repo(s)", len(pulled))
		if len(missing) > 0 {
			output.Info("  %d not cloned locally, skipped", len(missing))
		}
		if len(failed) > 0 {
			anyFailed = true
			output.Error("  %d failed", len(failed))
		}
	}

	if anyFailed {
		return fmt.Errorf("pull --all: some collections had failures")
	}
	return nil
}

// runPrune scans destDir for directories that look like stale clones of col
// (present on disk, origin URL matches this namespace, but no longer in
// col.Repos) and offers to delete them.
func runPrune(col *collection.Collection, destDir string, dryRun bool) error {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return fmt.Errorf("prune: could not read %s: %w", destDir, err)
	}

	ns := col.RepoNamespace()

	currentRepos := make(map[string]bool, len(col.Repos))
	for _, r := range col.Repos {
		currentRepos[r.Name] = true
	}

	var stale []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(destDir, entry.Name())
		gitDir := filepath.Join(dir, ".git")
		if !isDir(gitDir) {
			continue
		}
		remoteURL := readGitRemoteURL(filepath.Join(gitDir, "config"))
		if remoteURL == "" {
			continue
		}
		// Only consider clones whose origin URL matches namespace/dirname — this
		// collection owns them. Dirs cloned from other namespaces are left alone.
		repoPattern := ns + "/" + entry.Name()
		if !strings.Contains(remoteURL, repoPattern) {
			continue
		}
		if currentRepos[entry.Name()] {
			continue
		}
		stale = append(stale, entry.Name())
	}

	if len(stale) == 0 {
		return nil
	}

	if dryRun {
		fmt.Printf("[dry-run] %d stale clone(s) would be prompted for deletion:\n", len(stale))
		for _, name := range stale {
			dirty, _ := pruneHasUncommittedChangesFn(filepath.Join(destDir, name))
			if dirty {
				fmt.Printf("  %s/  (has uncommitted changes — would be skipped)\n", name)
			} else {
				fmt.Printf("  %s/\n", name)
			}
		}
		return nil
	}

	if !pruneIsTerminalFn() {
		fmt.Println("Run interactively to prune stale clones")
		return nil
	}

	var pruned int
	for _, name := range stale {
		dir := filepath.Join(destDir, name)
		dirty, err := pruneHasUncommittedChangesFn(dir)
		if err != nil {
			output.Warn("prune: could not check %s: %v", name, err)
			continue
		}
		if dirty {
			fmt.Fprintf(os.Stderr, "⚠ Skipped %s/ — has uncommitted changes\n", name)
			continue
		}
		if pruneConfirmFn(fmt.Sprintf("Delete local clone %s/?", name)) {
			if err := os.RemoveAll(dir); err != nil {
				output.Warn("prune: could not delete %s: %v", name, err)
				continue
			}
			fmt.Printf("✓ Deleted %s/\n", name)
			pruned++
		}
	}

	if pruned > 0 {
		output.Success("Pruned %d stale clone(s)", pruned)
	}
	return nil
}

// readGitRemoteURL parses a .git/config file and returns the url under
// [remote "origin"], or "" if not found.
func readGitRemoteURL(configPath string) string {
	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var inRemoteOrigin bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == `[remote "origin"]` {
			inRemoteOrigin = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inRemoteOrigin = false
		}
		if inRemoteOrigin && strings.HasPrefix(line, "url = ") {
			return strings.TrimPrefix(line, "url = ")
		}
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
