package cmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/activity"
	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

const (
	defaultActivityConcurrency = 4
	defaultActivityLimit       = 10
)

var (
	activityRepo  string
	activitySince string
	activityLimit int
	activityJSON  bool
)

var activityCmd = &cobra.Command{
	Use:   "activity <collection>",
	Short: "Show commits across a collection's repos, fetched live from the platform",
	Long: `Fetches the most recent commits on each accessible repo's default branch
directly from GitHub/GitLab, records any genuinely new ones to
~/.gitcollect/activity/<collection>.log, and prints the combined history
(this run's fetch plus everything previously recorded) filtered by --repo
and --since.

Unlike "gitcollect audit", which tracks access changes gitcollect itself
made, "activity" tracks git commits gitcollect observed in the repos —
code changes, not access changes.`,
	Args: cobra.ExactArgs(1),
	RunE: runActivity,
}

func init() {
	activityCmd.Flags().StringVar(&activityRepo, "repo", "", "show activity for only this repo")
	activityCmd.Flags().StringVar(&activitySince, "since", "", "filter to commits within this duration: 1h, 24h, 7d, 30d, or 90d")
	activityCmd.Flags().IntVar(&activityLimit, "limit", defaultActivityLimit, "max commits to fetch per repo this run")
	activityCmd.Flags().BoolVar(&activityJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(activityCmd)
}

func runActivity(cmd *cobra.Command, args []string) error {
	name := args[0]

	since, err := parseSince(activitySince)
	if err != nil {
		return NewUsageError(fmt.Errorf("activity: %w", err))
	}
	if activityLimit < 1 {
		return NewUsageError(fmt.Errorf("activity: --limit must be at least 1"))
	}

	col, _, callerID, client, err := loadForGit(name)
	if err != nil {
		return fmt.Errorf("activity: %w", err)
	}

	accessible, err := access.FilterAccessible(col, callerID, client)
	if err != nil {
		return fmt.Errorf("activity: %w", err)
	}

	targets := accessible
	if activityRepo != "" {
		targets = nil
		for _, r := range accessible {
			if r.Name == activityRepo {
				targets = append(targets, r)
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("activity: %q is not an accessible repo in this collection", activityRepo)
		}
	}

	existing, err := activity.Read(name)
	if err != nil {
		return fmt.Errorf("activity: %w", err)
	}
	known := activity.KnownSHAs(existing)

	fetched, fetchErrs := fetchActivity(col, client, targets, activityLimit)

	newCount := 0
	for _, e := range fetched {
		if known[e.Repo+"\x00"+e.CommitSHA] {
			continue
		}
		if err := activity.Append(e); err != nil {
			output.Warn("could not record activity for %s@%s: %v", e.Repo, shortSHA(e.CommitSHA), err)
			continue
		}
		newCount++
	}

	combined := mergeActivity(existing, fetched)
	if activityRepo != "" {
		combined = activity.Filter(combined, activityRepo, "", since)
	} else {
		combined = activity.Filter(combined, "", "", since)
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].CommittedAt.After(combined[j].CommittedAt)
	})

	if activityJSON {
		return output.JSON(combined)
	}

	if len(combined) == 0 {
		output.Info("no commits found")
	} else {
		rows := make([][]string, 0, len(combined))
		for _, e := range combined {
			rows = append(rows, []string{e.Repo, e.Branch, e.Author, shortSHA(e.CommitSHA), e.Message, e.CommittedAt.Local().Format("2006-01-02 15:04")})
		}
		output.Table([]string{"REPO", "BRANCH", "AUTHOR", "SHA", "MESSAGE", "WHEN"}, rows)
	}

	if newCount > 0 {
		output.Success("recorded %d new commit(s) to the activity log", newCount)
	}
	if len(fetchErrs) > 0 {
		output.Warn("%d repo(s) failed to check: %s", len(fetchErrs), strings.Join(fetchErrs, "; "))
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// mergeActivity combines previously recorded entries with this run's fresh
// fetch, de-duplicating by (repo, commit SHA) so a commit already on disk
// isn't shown twice just because it was still within this run's fetch
// window.
func mergeActivity(existing, fetched []activity.Entry) []activity.Entry {
	seen := make(map[string]bool, len(existing)+len(fetched))
	combined := make([]activity.Entry, 0, len(existing)+len(fetched))
	for _, e := range existing {
		key := e.Repo + "\x00" + e.CommitSHA
		if !seen[key] {
			seen[key] = true
			combined = append(combined, e)
		}
	}
	for _, e := range fetched {
		key := e.Repo + "\x00" + e.CommitSHA
		if !seen[key] {
			seen[key] = true
			combined = append(combined, e)
		}
	}
	return combined
}

// fetchActivity concurrently fetches the latest commits on each target
// repo's default branch and turns them into activity.Entry values, ready
// to be displayed and/or appended to the log.
func fetchActivity(col *collection.Collection, client api.Client, targets []collection.RepoAccess, limit int) ([]activity.Entry, []string) {
	type result struct {
		entries []activity.Entry
		err     error
	}
	results := make([]result, len(targets))
	ownerLogin := col.Logins[col.Owner]

	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultActivityConcurrency)
	for i, repo := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, repoName string) {
			defer wg.Done()
			defer func() { <-sem }()

			info, err := client.GetRepo(ownerLogin, repoName)
			if err != nil {
				results[i] = result{err: fmt.Errorf("%s: could not look up repo: %w", repoName, err)}
				return
			}
			branch := info.DefaultBranch
			if branch == "" {
				branch = "main"
			}

			commits, err := client.ListCommits(ownerLogin, repoName, branch, limit)
			if err != nil {
				results[i] = result{err: fmt.Errorf("%s: %w", repoName, err)}
				return
			}

			now := time.Now().UTC()
			entries := make([]activity.Entry, 0, len(commits))
			for _, c := range commits {
				entries = append(entries, activity.Entry{
					Timestamp:   now,
					Collection:  col.Name,
					Repo:        repoName,
					Branch:      branch,
					CommitSHA:   c.SHA,
					Author:      c.Author,
					Message:     c.Message,
					CommittedAt: c.CommittedAt,
				})
			}
			results[i] = result{entries: entries}
		}(i, repo.Name)
	}
	wg.Wait()

	var all []activity.Entry
	var errs []string
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err.Error())
			continue
		}
		all = append(all, r.entries...)
	}
	return all, errs
}
