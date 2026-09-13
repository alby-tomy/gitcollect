package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

// verifyResult holds the platform check result for one repo.
type verifyResult struct {
	Repo    string `json:"repo"`
	Status  string `json:"status"` // "ok" | "archived" | "not_found" | "forbidden"
	Message string `json:"message"`
}

var (
	verifyJSON bool
	verifyFix  bool
)

// verifyCheckFn is injectable so tests can return pre-built results without
// making real API calls.
var verifyCheckFn = func(col *collection.Collection, client api.Client) []verifyResult {
	return checkAllRepos(col, client)
}

var verifyCmd = &cobra.Command{
	Use:   "verify <collection>",
	Short: "Check every repo in a collection is still accessible on the platform",
	Long: `Check every repo in a collection against the platform API.

Detects repos that have been renamed, deleted, archived, or made
private since being added to the collection.

Examples:
  gitcollect verify my-project
  gitcollect verify my-project --json
  gitcollect verify my-project --fix

Exit codes:
  0  all repos accessible or only archived (warning only)
  1  at least one repo is not_found or forbidden`,
	Args: cobra.ExactArgs(1),
	RunE: runVerify,
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyJSON, "json", false, "machine-readable output")
	verifyCmd.Flags().BoolVar(&verifyFix, "fix", false, "prompt to remove not_found repos")
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, _, _, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	results := verifyCheckFn(col, client)

	if verifyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nVerifying %d repos against %s...\n\n", len(results), col.Host)
		for _, r := range results {
			icon := verifyIcon(r.Status)
			fmt.Fprintf(os.Stderr, "%s %-30s — %s\n", icon, r.Repo, r.Message)
		}
		fmt.Fprintln(os.Stderr)

		ok, archived, notFound, forbidden := tabulateVerify(results)
		parts := []string{}
		if notFound > 0 {
			parts = append(parts, fmt.Sprintf("%d missing", notFound))
		}
		if forbidden > 0 {
			parts = append(parts, fmt.Sprintf("%d forbidden", forbidden))
		}
		if archived > 0 {
			parts = append(parts, fmt.Sprintf("%d archived", archived))
		}
		parts = append(parts, fmt.Sprintf("%d accessible", ok))
		for _, p := range parts {
			fmt.Fprintf(os.Stderr, "%s · ", p)
		}
		fmt.Fprintln(os.Stderr)

		for _, r := range results {
			if r.Status == "not_found" {
				output.Suggestion(fmt.Sprintf("gitcollect remove %s %s", name, r.Repo))
			}
		}
	}

	if verifyFix {
		for _, r := range results {
			if r.Status != "not_found" {
				continue
			}
			if output.Confirm(fmt.Sprintf("Remove %s from %s?", r.Repo, name)) {
				if err := runRemove(nil, []string{name, r.Repo}); err != nil {
					output.Error("could not remove %s: %v", r.Repo, err)
				}
			}
		}
	}

	for _, r := range results {
		if r.Status == "not_found" || r.Status == "forbidden" {
			return fmt.Errorf("verify: %d repo(s) inaccessible", notFoundCount(results))
		}
	}
	return nil
}

func checkAllRepos(col *collection.Collection, client api.Client) []verifyResult {
	type work struct {
		idx  int
		repo collection.RepoAccess
	}

	results := make([]verifyResult, len(col.Repos))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup

	namespace := col.RepoNamespace()

	for i, r := range col.Repos {
		wg.Add(1)
		go func(idx int, repo collection.RepoAccess) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := client.GetRepo(namespace, repo.Name)
			switch {
			case err == nil && info.Archived:
				results[idx] = verifyResult{Repo: repo.Name, Status: "archived", Message: "archived (cloneable but read-only)"}
			case err == nil:
				results[idx] = verifyResult{Repo: repo.Name, Status: "ok", Message: "accessible"}
			case errors.Is(err, api.ErrNotFound):
				results[idx] = verifyResult{Repo: repo.Name, Status: "not_found", Message: "not found (renamed or deleted on platform)"}
			case errors.Is(err, api.ErrForbidden):
				results[idx] = verifyResult{Repo: repo.Name, Status: "forbidden", Message: "forbidden (made private or access revoked)"}
			default:
				results[idx] = verifyResult{Repo: repo.Name, Status: "not_found", Message: fmt.Sprintf("error: %v", err)}
			}
		}(i, r)
	}
	wg.Wait()
	return results
}

func verifyIcon(status string) string {
	switch status {
	case "ok":
		return "✓"
	case "archived":
		return "⚠"
	default:
		return "✗"
	}
}

func tabulateVerify(results []verifyResult) (ok, archived, notFound, forbidden int) {
	for _, r := range results {
		switch r.Status {
		case "ok":
			ok++
		case "archived":
			archived++
		case "not_found":
			notFound++
		case "forbidden":
			forbidden++
		}
	}
	return
}

func notFoundCount(results []verifyResult) int {
	n := 0
	for _, r := range results {
		if r.Status == "not_found" || r.Status == "forbidden" {
			n++
		}
	}
	return n
}
