package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/git"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var pullDest string

var pullCmd = &cobra.Command{
	Use:   "pull <collection>",
	Short: "git pull inside every accessible repo that's already cloned",
	Args:  cobra.ExactArgs(1),
	RunE:  runPull,
}

func init() {
	pullCmd.Flags().StringVar(&pullDest, "dest", ".", "directory repos were cloned into")
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
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
	if len(failed) > 0 {
		output.Error("%d repo(s) failed to pull:", len(failed))
		for _, f := range failed {
			output.Dim("  ✗ %s", f)
		}
		return fmt.Errorf("pull: %d repo(s) failed", len(failed))
	}
	return nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
