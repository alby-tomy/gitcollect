package cmd

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/git"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	healthDest string
)

var healthCmd = &cobra.Command{
	Use:   "health <collection>",
	Short: "Aggregated health dashboard: repos, branches, behind count, open PRs",
	Long: `Shows a consolidated health overview for a collection:

  • How many repos are cloned locally vs total accessible
  • How many cloned repos have local changes
  • How many are behind their remote
  • Total open pull/merge requests across all accessible repos

Repos that have not been cloned yet are counted but skipped for local checks.
Use "gitcollect sync <collection>" to clone any missing repos first.`,
	Args: cobra.ExactArgs(1),
	RunE: runHealth,
}

func init() {
	healthCmd.Flags().StringVar(&healthDest, "dest", ".", "directory repos were cloned into")
	rootCmd.AddCommand(healthCmd)
}

type healthResult struct {
	name    string
	cloned  bool
	dirty   bool
	behind  int
	openPRs int
	err     error
}

func runHealth(_ *cobra.Command, args []string) error {
	name := args[0]

	col, caller, callerID, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("health: %w", err)
	}

	accessible, err := access.FilterAccessible(col, callerID, client)
	if err != nil {
		return fmt.Errorf("health: %w", err)
	}

	printAccessSummary(col, caller, callerID, len(accessible), len(col.Repos))
	fmt.Println()

	ns := col.RepoNamespace()

	results := make([]healthResult, len(accessible))
	var wg sync.WaitGroup
	sem := make(chan struct{}, prMaxConcurrency)

	for i, repo := range accessible {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, repo collection.RepoAccess) {
			defer wg.Done()
			defer func() { <-sem }()

			r := healthResult{name: repo.Name}
			dir := filepath.Join(healthDest, repo.Name)
			r.cloned = isDir(dir)

			if r.cloned {
				out, err := git.Status(dir)
				if err == nil {
					r.dirty = out != ""
				}
				r.behind, _ = git.CommitsBehind(dir)
			}

			prs, err := client.ListOpenPRs(ns, repo.Name)
			if err == nil {
				r.openPRs = len(prs)
			}

			results[i] = r
		}(i, repo)
	}
	wg.Wait()

	var cloned, dirty, behind, totalPRs int
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		if r.cloned {
			cloned++
		}
		if r.dirty {
			dirty++
		}
		if r.behind > 0 {
			behind++
		}
		totalPRs += r.openPRs

		clonedMark := "no"
		if r.cloned {
			clonedMark = "yes"
		}
		dirtyMark := ""
		if r.dirty {
			dirtyMark = "yes"
		}
		behindStr := ""
		if r.behind > 0 {
			behindStr = fmt.Sprintf("%d", r.behind)
		}
		prStr := ""
		if r.openPRs > 0 {
			prStr = fmt.Sprintf("%d", r.openPRs)
		}
		rows = append(rows, []string{r.name, clonedMark, dirtyMark, behindStr, prStr})
	}

	output.Table([]string{"REPO", "CLONED", "DIRTY", "BEHIND", "OPEN PRS"}, rows)
	fmt.Println()

	output.Info("Summary for %s:", name)
	fmt.Printf("  Repos accessible : %d / %d\n", len(accessible), len(col.Repos))
	fmt.Printf("  Cloned locally   : %d / %d\n", cloned, len(accessible))
	if dirty > 0 {
		fmt.Printf("  Dirty (local changes): %d\n", dirty)
	}
	if behind > 0 {
		fmt.Printf("  Behind remote    : %d\n", behind)
	}
	fmt.Printf("  Open PRs/MRs     : %d\n", totalPRs)

	if cloned < len(accessible) {
		fmt.Println()
		output.Suggestion(fmt.Sprintf("gitcollect sync %s  # to clone missing repos", name))
	}
	return nil
}
