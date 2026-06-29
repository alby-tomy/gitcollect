package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
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

const defaultCloneConcurrency = 4

var (
	clonePick        []string
	cloneDryRun      bool
	cloneConcurrency int
	cloneDest        string
)

var cloneCmd = &cobra.Command{
	Use:   "clone <collection>",
	Short: "Clone every repo you can access in a collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runClone,
}

func init() {
	cloneCmd.Flags().StringArrayVar(&clonePick, "pick", nil, `clone only these repos: space-separated within one value (--pick "r1 r2"), and/or --pick repeated`)
	cloneCmd.Flags().BoolVar(&cloneDryRun, "dry-run", false, "preview what would be cloned without doing it")
	cloneCmd.Flags().IntVar(&cloneConcurrency, "concurrency", defaultCloneConcurrency, "max repos to clone in parallel")
	cloneCmd.Flags().StringVar(&cloneDest, "dest", ".", "directory to clone repos into")
	rootCmd.AddCommand(cloneCmd)
}

// splitPick flattens --pick's raw values into individual repo names: each
// value may itself be a whitespace-separated list (--pick "r1 r2"), and
// the flag may also be repeated (--pick r1 --pick r2). Either form, or a
// mix of both, works.
func splitPick(raw []string) []string {
	var picks []string
	for _, v := range raw {
		picks = append(picks, strings.Fields(v)...)
	}
	return picks
}

func runClone(cmd *cobra.Command, args []string) error {
	name := args[0]

	if cloneConcurrency < 1 {
		return NewUsageError(fmt.Errorf("clone: --concurrency must be at least 1"))
	}
	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	col, caller, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	accessible, err := access.FilterAccessible(col, caller, client)
	if err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	targets, skipped, err := selectCloneTargets(col, accessible, splitPick(clonePick))
	if err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	printAccessSummary(col, caller, len(accessible), len(col.Repos))

	if len(targets) == 0 {
		output.Info("no repos to clone")
		return nil
	}

	results := cloneAll(col, client, targets, cloneDest, cloneConcurrency, cloneDryRun)

	var cloned, failed []string
	var totalDur time.Duration
	for _, r := range results {
		totalDur += r.duration
		if r.err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", r.name, r.err))
		} else {
			cloned = append(cloned, r.name)
		}
	}

	if cloneDryRun {
		output.Success("Dry run: would clone %d repo(s)", len(targets))
		return nil
	}

	if len(failed) > 0 {
		output.Error("Cloned %d repo(s), %d failed", len(cloned), len(failed))
		for _, f := range failed {
			output.Dim("  ✗ %s", f)
		}
	} else {
		output.Success("Cloned %d repo(s) in %.1fs", len(cloned), totalDur.Seconds())
	}

	if len(skipped) > 0 {
		fmt.Printf("  %d repo(s) skipped (no access): %s\n", len(skipped), strings.Join(skipped, ", "))
		output.Suggestion(fmt.Sprintf("gitcollect inspect %s --user %s", name, caller))

		// A repo can be "skipped" here for two very different reasons: the
		// caller genuinely isn't entitled to it (FilterAccessible's local
		// rule check), or they ARE entitled but GitHub still has them as a
		// pending, unaccepted invite rather than a confirmed collaborator
		// (CheckCollaborator reports false either way). Distinguish the
		// second case so they're not left thinking "no access" forever.
		if repo := firstPendingInvite(col, caller, skipped, client); repo != "" {
			output.InviteWarning(caller, col.Owner, api.GitHubNotificationsURL, fmt.Sprintf("gitcollect clone %s", name))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("clone: %d repo(s) failed", len(failed))
	}
	return nil
}

// firstPendingInvite returns the first repo in skipped where caller has an
// unaccepted GitHub collaborator invite, or "" if none do.
func firstPendingInvite(col *collection.Collection, caller string, skipped []string, client api.Client) string {
	for _, repoName := range skipped {
		has, err := client.GetPendingInvite(col.Owner, repoName, caller)
		if err == nil && has {
			return repoName
		}
	}
	return ""
}

// printAccessSummary prints the "access verified" header line shared by
// clone/pull/status: who the caller is, what groups grant them access, and
// how many of the collection's repos they can reach.
func printAccessSummary(col *collection.Collection, caller string, accessibleCount, totalCount int) {
	switch {
	case col.Visibility == collection.VisibilityPublic:
		output.Success("Public collection — %d of %d repos accessible", accessibleCount, totalCount)
	default:
		groups := strings.Join(groupsForMember(col, caller), ", ")
		if groups == "" {
			groups = "no groups"
		}
		output.Success("Access verified (%s · %s)", caller, groups)
		fmt.Printf("  %d of %d repos accessible\n", accessibleCount, totalCount)
	}
}

// selectCloneTargets narrows accessible down to pick (if given), validating
// every picked name is both a real repo in col and one the caller can
// access. It also returns the names of repos the caller cannot reach, for
// the "N repos skipped" summary.
func selectCloneTargets(col *collection.Collection, accessible []collection.RepoAccess, pick []string) (targets []collection.RepoAccess, skipped []string, err error) {
	accessibleSet := make(map[string]collection.RepoAccess, len(accessible))
	for _, r := range accessible {
		accessibleSet[r.Name] = r
	}

	for _, r := range col.Repos {
		if _, ok := accessibleSet[r.Name]; !ok {
			skipped = append(skipped, r.Name)
		}
	}
	sort.Strings(skipped)

	if len(pick) == 0 {
		return accessible, skipped, nil
	}

	targets = make([]collection.RepoAccess, 0, len(pick))
	for _, name := range pick {
		r, ok := accessibleSet[name]
		if !ok {
			return nil, nil, fmt.Errorf("%q is not an accessible repo in this collection", name)
		}
		targets = append(targets, r)
	}
	return targets, skipped, nil
}

type cloneResult struct {
	name     string
	duration time.Duration
	err      error
}

// cloneAll clones targets into dest, at most concurrency at a time, and
// returns one result per repo, printing a progress line as each completes.
// In dry-run mode no git command is run and each repo "completes" instantly.
func cloneAll(col *collection.Collection, client api.Client, targets []collection.RepoAccess, dest string, concurrency int, dryRun bool) []cloneResult {
	results := make([]cloneResult, len(targets))

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
			err := cloneOne(col, client, repo.Name, dest, dryRun)
			dur := time.Since(start)

			mu.Lock()
			done++
			label := fmt.Sprintf("Cloning %s...", repo.Name)
			if dryRun {
				label = fmt.Sprintf("Would clone %s", repo.Name)
			}
			status := "✓ done"
			if err != nil {
				status = "✗ failed: " + err.Error()
			}
			fmt.Printf("[%d/%d] %-30s %s  (%.1fs)\n", done, len(targets), label, status, dur.Seconds())
			results[i] = cloneResult{name: repo.Name, duration: dur, err: err}
			mu.Unlock()
		}(i, repo)
	}
	wg.Wait()
	return results
}

// cloneOne resolves repoName's HTTPS clone URL via the platform API and
// clones it into <dest>/<repoName>.
func cloneOne(col *collection.Collection, client api.Client, repoName, dest string, dryRun bool) error {
	info, err := client.GetRepo(col.Owner, repoName)
	if err != nil {
		return fmt.Errorf("could not look up %s/%s: %w", col.Owner, repoName, err)
	}
	if dryRun {
		return nil
	}
	target := filepath.Join(dest, repoName)
	return git.Clone(info.CloneURL, target)
}
