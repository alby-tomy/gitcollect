package cmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/access"
	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

const prMaxConcurrency = 4

var (
	prJSON   bool
	prAuthor string
)

var prCmd = &cobra.Command{
	Use:   "pr <collection>",
	Short: "List open pull/merge requests across all repos in a collection",
	Long: `Fetches open pull requests (GitHub) or merge requests (GitLab) for every
accessible repo in the collection and displays them as a unified table,
sorted newest-first.

Use --author to narrow to PRs opened by a specific GitHub/GitLab login.`,
	Args: cobra.ExactArgs(1),
	RunE: runPRList,
}

func init() {
	prCmd.Flags().BoolVar(&prJSON, "json", false, "machine-readable output")
	prCmd.Flags().StringVar(&prAuthor, "author", "", "show only PRs opened by this login")
	rootCmd.AddCommand(prCmd)
}

type prRow struct {
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

func runPRList(_ *cobra.Command, args []string) error {
	name := args[0]

	col, _, callerID, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("pr: %w", err)
	}

	accessible, err := access.FilterAccessible(col, callerID, client)
	if err != nil {
		return fmt.Errorf("pr: %w", err)
	}

	ns := col.RepoNamespace()
	prs, failed := fetchOpenPRs(client, ns, accessible)

	if prAuthor != "" {
		filtered := prs[:0]
		for _, p := range prs {
			if strings.EqualFold(p.Author, prAuthor) {
				filtered = append(filtered, p)
			}
		}
		prs = filtered
	}

	sort.Slice(prs, func(i, j int) bool {
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})

	if prJSON {
		rows := make([]prRow, 0, len(prs))
		for _, p := range prs {
			rows = append(rows, prRow{
				Repo:      p.Repo,
				Number:    p.Number,
				Title:     p.Title,
				Author:    p.Author,
				URL:       p.URL,
				UpdatedAt: p.UpdatedAt.Format("2006-01-02"),
			})
		}
		reportUnreadableRepos(failed)
		return output.JSON(rows)
	}

	if len(prs) == 0 {
		if prAuthor != "" {
			output.Info("no open PRs by %s in %s", prAuthor, name)
		} else {
			output.Info("no open PRs found in %s", name)
		}
		reportUnreadableRepos(failed)
		return nil
	}

	tableRows := make([][]string, 0, len(prs))
	for _, p := range prs {
		tableRows = append(tableRows, []string{
			p.Repo,
			fmt.Sprintf("#%d", p.Number),
			truncatePRTitle(p.Title),
			p.Author,
			p.UpdatedAt.Format("2006-01-02"),
		})
	}
	output.Table([]string{"REPO", "#", "TITLE", "AUTHOR", "UPDATED"}, tableRows)
	output.Dim("%d open PR(s) across %d repo(s)", len(prs), len(accessible)-len(failed))
	reportUnreadableRepos(failed)
	return nil
}

// fetchOpenPRs fetches open PRs for all accessible repos concurrently,
// returning everything it could reach plus the repos it could not.
//
// The failures are returned rather than discarded because a 403, a rate
// limit and a genuinely empty repo used to render identically: as no open
// PRs. Under-reporting without saying so is worse than refusing to render,
// so the caller names the repos it could not read.
func fetchOpenPRs(client api.Client, ns string, repos []collection.RepoAccess) (prs []api.PRInfo, failed []string) {
	type result struct {
		prs []api.PRInfo
		err error
	}
	results := make([]result, len(repos))

	var wg sync.WaitGroup
	sem := make(chan struct{}, prMaxConcurrency)

	for i, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, repoName string) {
			defer wg.Done()
			defer func() { <-sem }()
			found, err := client.ListOpenPRs(ns, repoName)
			if err != nil {
				results[i] = result{err: err}
				return
			}
			for j := range found {
				found[j].Repo = repoName
			}
			results[i] = result{prs: found}
		}(i, repo.Name)
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			failed = append(failed, repos[i].Name)
			continue
		}
		prs = append(prs, r.prs...)
	}
	return prs, failed
}

func truncatePRTitle(s string) string {
	runes := []rune(s)
	if len(runes) <= 50 {
		return s
	}
	return string(runes[:50]) + "..."
}

// reportUnreadableRepos warns about repos whose PR listing failed, so an
// incomplete result is never mistaken for an empty one.
func reportUnreadableRepos(failed []string) {
	if len(failed) == 0 {
		return
	}
	output.Warn("could not read PRs for %d repo(s): %s", len(failed), strings.Join(failed, ", "))
	output.Dim("  The counts above exclude them.")
}
