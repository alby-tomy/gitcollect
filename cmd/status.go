package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/git"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var statusDest string

var statusCmd = &cobra.Command{
	Use:   "status <collection>",
	Short: "git status inside every accessible repo that's already cloned",
	Args:  cobra.ExactArgs(1),
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusDest, "dest", ".", "directory repos were cloned into")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
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
		out, err := git.Status(dir)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", repo.Name, err))
			continue
		}
		state := "clean"
		if out != "" {
			state = fmt.Sprintf("%d change(s)", len(strings.Split(out, "\n")))
		}
		rows = append(rows, []string{repo.Name, state})
	}

	if len(rows) > 0 {
		fmt.Println()
		output.Table([]string{"REPO", "STATUS"}, rows)
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
	return nil
}
