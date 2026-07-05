package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/git"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	statusDest    string
	statusVerbose bool
	statusAll     bool

	// injectable for testing — real git calls by default
	statusGitStatusFn        = git.Status
	statusGitCommitsBehindFn = git.CommitsBehind
)

var statusCmd = &cobra.Command{
	Use:   "status <collection>",
	Short: "git status inside every accessible repo that's already cloned",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusDest, "dest", ".", "directory repos were cloned into")
	statusCmd.Flags().BoolVar(&statusVerbose, "verbose", false, "show raw git status output per repo")
	statusCmd.Flags().BoolVar(&statusAll, "all", false, "run across all collections you own or belong to")
	rootCmd.AddCommand(statusCmd)
}

// runStatusAll iterates every collection the caller can reach and prints
// a status summary for each.
func runStatusAll(destDir string) error {
	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("status --all: %w", err)
	}
	if len(names) == 0 {
		output.Info("no collections found")
		return nil
	}

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
			continue
		}
		printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))

		rows := make([][]string, 0, len(accessible))
		for _, repo := range accessible {
			dir := filepath.Join(destDir, repo.Name)
			if !isDir(dir) {
				continue
			}
			out, err := statusGitStatusFn(dir)
			if err != nil {
				continue
			}
			state := "clean"
			if out != "" {
				state = fmt.Sprintf("%d change(s)", len(strings.Split(strings.TrimSpace(out), "\n")))
			}
			behind, _ := statusGitCommitsBehindFn(dir)
			rows = append(rows, []string{repo.Name, state, fmt.Sprintf("%d", behind)})
		}
		if len(rows) > 0 {
			fmt.Println()
			output.Table([]string{"REPO", "STATUS", "BEHIND"}, rows)
		}
	}
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	if statusAll && len(args) > 0 {
		return NewUsageError(fmt.Errorf("status: use either <collection> or --all, not both"))
	}
	if !statusAll && len(args) == 0 {
		return NewUsageError(fmt.Errorf("status: provide a collection name or use --all"))
	}
	if statusAll {
		return runStatusAll(statusDest)
	}

	name := args[0]

	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("status: %w", err)
	}

	col, caller, callerID, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	accessible, err := access.FilterAccessible(col, callerID, client)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))

	var missing, failed []string
	rows := make([][]string, 0, len(accessible))
	for _, repo := range accessible {
		dir := filepath.Join(statusDest, repo.Name)
		if !isDir(dir) {
			missing = append(missing, repo.Name)
			continue
		}

		out, err := statusGitStatusFn(dir)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", repo.Name, err))
			continue
		}

		if statusVerbose {
			if out != "" {
				fmt.Printf("\n=== %s ===\n%s\n", repo.Name, out)
			} else {
				fmt.Printf("\n=== %s === (clean)\n", repo.Name)
			}
			continue
		}

		state := "clean"
		if out != "" {
			state = fmt.Sprintf("%d change(s)", len(strings.Split(strings.TrimSpace(out), "\n")))
		}

		behind, _ := statusGitCommitsBehindFn(dir)
		rows = append(rows, []string{repo.Name, state, fmt.Sprintf("%d", behind)})
	}

	if len(rows) > 0 {
		fmt.Println()
		output.Table([]string{"REPO", "STATUS", "BEHIND"}, rows)
	}
	if len(missing) > 0 {
		output.Info("%d repo(s) not cloned locally, skipped: %v", len(missing), missing)
	}
	if len(failed) > 0 {
		output.Error("%d repo(s) failed:", len(failed))
		for _, f := range failed {
			output.Dim("  ✗ %s", f)
		}
		return fmt.Errorf("status: %d repo(s) failed", len(failed))
	}

	if len(rows) > 0 || (!statusVerbose && len(accessible) > 0) {
		output.Success("Checked %d of %d repo(s)", len(rows)+len(missing)+len(failed), len(accessible))
	}

	return nil
}
