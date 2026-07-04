package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

const diffMaxConcurrency = 4

var (
	diffReposOnly   bool
	diffMembersOnly bool
	diffJSON        bool
)

var diffCmd = &cobra.Command{
	Use:   "diff <collection>",
	Short: "Compare a local collection against GitHub/GitLab reality",
	Args:  cobra.ExactArgs(1),
	RunE:  runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffReposOnly, "repos-only", false, "only compare repos, skip member check")
	diffCmd.Flags().BoolVar(&diffMembersOnly, "members-only", false, "only compare members, skip repo check")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "machine-readable JSON output")
	rootCmd.AddCommand(diffCmd)
}

type repoDiffResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok" | "archived" | "missing" | "forbidden"
}

type memberDiffResult struct {
	Username  string `json:"username"`
	ID        string `json:"id"`
	Status    string `json:"status"`    // "ok" | "drift"
	DriftRepo string `json:"drift_repo,omitempty"` // first repo where check failed
}

type diffOutput struct {
	Collection string             `json:"collection"`
	Repos      []repoDiffResult   `json:"repos"`
	Members    []memberDiffResult `json:"members"`
}

func runDiff(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, _, _, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	var (
		repoResults   []repoDiffResult
		memberResults []memberDiffResult
		apiErr        error
	)

	if !diffMembersOnly {
		repoResults, err = checkRepoDiff(col, client)
		if err != nil {
			apiErr = err
		}
	}
	if !diffReposOnly {
		memberResults, err = checkMemberDiff(col, client)
		if err != nil && apiErr == nil {
			apiErr = err
		}
	}

	if diffJSON {
		out := diffOutput{
			Collection: name,
			Repos:      repoResults,
			Members:    memberResults,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return fmt.Errorf("diff: json encode: %w", err)
		}
		if apiErr != nil {
			return fmt.Errorf("diff: %w", apiErr)
		}
		return nil
	}

	fmt.Printf("Comparing %s against %s...\n\n", name, col.Host)

	if !diffMembersOnly {
		printRepoDiff(repoResults)
	}
	if !diffReposOnly {
		printMemberDiff(memberResults)
	}

	printDiffSummary(name, repoResults, memberResults)

	if apiErr != nil {
		return fmt.Errorf("diff: %w", apiErr)
	}
	return nil
}

func checkRepoDiff(col *collection.Collection, client api.Client) ([]repoDiffResult, error) {
	type indexedResult struct {
		idx int
		r   repoDiffResult
		err error
	}

	repos := col.Repos
	results := make([]repoDiffResult, len(repos))
	ch := make(chan indexedResult, len(repos))

	sem := make(chan struct{}, diffMaxConcurrency)
	var wg sync.WaitGroup
	ns := col.RepoNamespace()

	for i, r := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, repoName string) {
			defer wg.Done()
			defer func() { <-sem }()

			info, err := client.GetRepo(ns, repoName)
			var res repoDiffResult
			res.Name = repoName
			var apiErr error
			switch {
			case err == nil && info.Archived:
				res.Status = "archived"
			case err == nil:
				res.Status = "ok"
			case errors.Is(err, api.ErrNotFound):
				res.Status = "missing"
			case errors.Is(err, api.ErrForbidden):
				res.Status = "forbidden"
			default:
				res.Status = "error"
				apiErr = fmt.Errorf("repo %s: %w", repoName, err)
			}
			ch <- indexedResult{idx: idx, r: res, err: apiErr}
		}(i, r.Name)
	}

	wg.Wait()
	close(ch)

	var firstErr error
	for ir := range ch {
		results[ir.idx] = ir.r
		if ir.err != nil && firstErr == nil {
			firstErr = ir.err
		}
	}
	return results, firstErr
}

func checkMemberDiff(col *collection.Collection, client api.Client) ([]memberDiffResult, error) {
	ns := col.RepoNamespace()

	type indexedResult struct {
		idx int
		r   memberDiffResult
		err error
	}

	members := col.Members
	results := make([]memberDiffResult, len(members))
	ch := make(chan indexedResult, len(members))

	sem := make(chan struct{}, diffMaxConcurrency)
	var wg sync.WaitGroup

	for i, id := range members {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, memberID string) {
			defer wg.Done()
			defer func() { <-sem }()

			login := col.Logins[memberID]
			var res memberDiffResult
			res.ID = memberID
			res.Username = login
			var apiErr error

			// Use the first repo this specific member can access as the
			// sample — avoids false drift on repos they were never entitled to.
			accessible := col.AccessibleRepos(memberID)
			if len(accessible) == 0 {
				// No accessible repos: nothing to check against.
				res.Status = "ok"
				ch <- indexedResult{idx: idx, r: res}
				return
			}
			sampleRepo := accessible[0].Name

			has, err := client.CheckCollaborator(ns, sampleRepo, login)
			switch {
			case err != nil:
				res.Status = "error"
				apiErr = fmt.Errorf("member %s: %w", login, err)
			case !has:
				res.Status = "drift"
				res.DriftRepo = sampleRepo
			default:
				res.Status = "ok"
			}
			ch <- indexedResult{idx: idx, r: res, err: apiErr}
		}(i, id)
	}

	wg.Wait()
	close(ch)

	var firstErr error
	for ir := range ch {
		results[ir.idx] = ir.r
		if ir.err != nil && firstErr == nil {
			firstErr = ir.err
		}
	}
	return results, firstErr
}

func printRepoDiff(results []repoDiffResult) {
	fmt.Println("REPOS")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, r := range results {
		switch r.Status {
		case "ok":
			fmt.Fprintf(w, "  ✓ %s\t in sync\n", r.Name)
		case "archived":
			fmt.Fprintf(w, "  ⚠ %s\t archived on GitHub (still cloneable)\n", r.Name)
		case "missing":
			fmt.Fprintf(w, "  ✗ %s\t not found (renamed or deleted on GitHub)\n", r.Name)
		case "forbidden":
			fmt.Fprintf(w, "  ⚠ %s\t access changed on GitHub\n", r.Name)
		default:
			fmt.Fprintf(w, "  ✗ %s\t API error\n", r.Name)
		}
	}
	w.Flush()
	fmt.Println()
}

func printMemberDiff(results []memberDiffResult) {
	fmt.Println("MEMBERS")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, r := range results {
		switch r.Status {
		case "ok":
			fmt.Fprintf(w, "  ✓ %s\t collaborator on all accessible repos\n", r.Username)
		case "drift":
			fmt.Fprintf(w, "  ⚠ %s\t removed as collaborator on %s directly\n", r.Username, r.DriftRepo)
		default:
			fmt.Fprintf(w, "  ✗ %s\t API error\n", r.Username)
		}
	}
	w.Flush()
	fmt.Println()
}

func printDiffSummary(name string, repos []repoDiffResult, members []memberDiffResult) {
	var missing, archived, drift int
	for _, r := range repos {
		switch r.Status {
		case "missing":
			missing++
		case "archived":
			archived++
		}
	}
	for _, m := range members {
		if m.Status == "drift" {
			drift++
		}
	}

	if missing == 0 && archived == 0 && drift == 0 {
		fmt.Println("Summary: all in sync")
		return
	}

	parts := []string{}
	if missing > 0 {
		parts = append(parts, fmt.Sprintf("%d missing repo(s)", missing))
	}
	if archived > 0 {
		parts = append(parts, fmt.Sprintf("%d archived", archived))
	}
	if drift > 0 {
		parts = append(parts, fmt.Sprintf("%d member drift", drift))
	}

	summary := "Summary:"
	for i, p := range parts {
		if i == 0 {
			summary += " " + p
		} else {
			summary += " · " + p
		}
	}
	fmt.Println(summary)

	if missing > 0 {
		fmt.Printf("Run: gitcollect sync-config %s  to update from GitHub state\n", name)
	}
}
