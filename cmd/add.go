package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	newRepoVisibility string
	errSkipped        = errors.New("skipped by user")

	addPattern string
	addTopic   string
	addOrg     string
	addDryRun  bool
	addLimit   int

	// addConfirmFn is injectable so tests can control the y/N prompt.
	addConfirmFn = func(msg string) bool { return output.Confirm(msg) }

	// addIsTerminalFn is injectable so tests can simulate interactive mode for
	// the archived-repo confirmation prompt.
	addIsTerminalFn = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
)

var addCmd = &cobra.Command{
	Use:   "add <collection> [repo...] [--pattern glob | --topic name]",
	Short: "Add repos to a collection, open to all members by default",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVar(
		&newRepoVisibility,
		"new-repo-visibility",
		"private",
		`visibility for auto-created repos: "public" or "private" (default "private")`,
	)
	addCmd.Flags().StringVar(&addPattern, "pattern", "", "add all repos matching this name pattern (supports * wildcard)")
	addCmd.Flags().StringVar(&addTopic, "topic", "", "add all repos with this GitHub topic")
	addCmd.Flags().StringVar(&addOrg, "org", "", "org to search in (default: collection's namespace)")
	addCmd.Flags().BoolVar(&addDryRun, "dry-run", false, "show which repos would be added without adding them")
	addCmd.Flags().IntVar(&addLimit, "limit", 50, "max repos to add via search (default 50, max 100)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoNames := args[1:]

	useSearch := addPattern != "" || addTopic != ""

	if useSearch && len(repoNames) > 0 {
		return NewUsageError(fmt.Errorf("add: cannot combine positional repo names with --pattern or --topic"))
	}
	if !useSearch && len(repoNames) == 0 {
		return NewUsageError(fmt.Errorf("add: provide at least one repo name, or use --pattern/--topic to search"))
	}

	for _, repoName := range repoNames {
		if err := collection.ValidateRepoName(repoName); err != nil {
			return NewUsageError(fmt.Errorf("add: %w", err))
		}
	}

	if newRepoVisibility != "public" && newRepoVisibility != "private" {
		return fmt.Errorf("add: invalid --new-repo-visibility %q: must be \"public\" or \"private\"", newRepoVisibility)
	}

	col, caller, callerID, client, err := loadForOwner("add", name)
	if err != nil {
		return err
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("add: only %s (the owner) can add repos to %q", col.Logins[col.Owner], name)
	}

	if useSearch {
		return runAddSearch(col, name, caller, callerID, client)
	}

	private := newRepoVisibility == "private"
	var failed []string
	var skipped []string
	for _, repoName := range repoNames {
		if err := addOneRepo(col, name, caller, callerID, repoName, client, private); err != nil {
			if errors.Is(err, errSkipped) {
				skipped = append(skipped, repoName)
				continue
			}
			failed = append(failed, fmt.Sprintf("%s (%v)", repoName, err))
		}
	}

	if len(skipped) > 0 {
		output.Dim("Skipped: %s (declined creation)", strings.Join(skipped, ", "))
	}
	if len(failed) > 0 {
		return fmt.Errorf("add: %d of %d failed: %s", len(failed), len(repoNames), strings.Join(failed, "; "))
	}
	return nil
}

func runAddSearch(col *collection.Collection, collectionName, caller, callerID string, client api.Client) error {
	org := addOrg
	if org == "" {
		org = col.RepoNamespace()
	}

	limit := addLimit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	if addPattern != "" {
		fmt.Printf("Searching %s for repos matching %q...\n", org, addPattern)
	} else {
		fmt.Printf("Searching %s for repos with topic %q...\n", org, addTopic)
	}

	repos, err := client.SearchRepos(org, addPattern, addTopic, limit)
	if err != nil {
		return fmt.Errorf("add: search: %w", err)
	}

	if len(repos) == 0 {
		fmt.Println("No repos found matching the search criteria.")
		return nil
	}

	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}

	if addDryRun {
		fmt.Printf("[dry-run] Would add %d repos to %s:\n  %s\n\nRun without --dry-run to apply.\n",
			len(repos), collectionName, strings.Join(names, ", "))
		return nil
	}

	fmt.Printf("Found %d repos:\n  %s\n\n", len(repos), strings.Join(names, ", "))

	if !addConfirmFn(fmt.Sprintf("Add all %d to %s?", len(repos), collectionName)) {
		output.Info("Aborted.")
		return nil
	}

	private := newRepoVisibility == "private"
	var failed []string
	for i, r := range repos {
		fmt.Printf("[%d/%d] Adding %s...\n", i+1, len(repos), r.Name)
		if err := addOneRepo(col, collectionName, caller, callerID, r.Name, client, private); err != nil {
			if errors.Is(err, errSkipped) {
				continue
			}
			failed = append(failed, fmt.Sprintf("%s (%v)", r.Name, err))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("add: %d of %d failed: %s", len(failed), len(repos), strings.Join(failed, "; "))
	}
	output.Success("Added %d repos to %s", len(repos), collectionName)
	return nil
}

// ensureRepoExists checks whether repoName exists under col.RepoNamespace().
// Returns (archived, nil) if the repo exists; archived reflects the platform
// archived flag so the caller can warn before adding. If the repo does not
// exist and the context is interactive, asks the owner whether to create it;
// on confirmation, creates the repo and audits the action. Returns errSkipped
// if the user declines. In non-interactive contexts returns an error immediately.
func ensureRepoExists(col *collection.Collection, repoName string, client api.Client, caller api.UserInfo, private bool) (archived bool, err error) {
	namespace := col.RepoNamespace()

	repoInfo, gerr := client.GetRepo(namespace, repoName)
	if gerr == nil {
		return repoInfo.Archived, nil
	}
	if !errors.Is(gerr, api.ErrNotFound) {
		return false, fmt.Errorf("checking repo %q: %w", repoName, gerr)
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false, fmt.Errorf("repo %q not found under %s (running non-interactively — create it manually first)", repoName, namespace)
	}

	output.Warn("repo %q does not exist under %s", repoName, namespace)
	if !output.Confirm(fmt.Sprintf("Create %s/%s as a %s repository?", namespace, repoName, visibilityWord(private))) {
		return false, errSkipped
	}

	_, createErr := client.CreateRepo(namespace, repoName, private, "")
	if errors.Is(createErr, api.ErrNameConflict) {
		output.Info("repo %q was just created by someone else — continuing", repoName)
		return false, nil
	}
	if createErr != nil {
		return false, fmt.Errorf("create repo %q: %w", repoName, createErr)
	}

	recordAudit(audit.AuditEntry{
		Collection: col.Name,
		Actor:      caller.Login,
		Action:     "repo.create",
		Target:     repoName,
		Detail:     fmt.Sprintf("Created %s/%s (%s)", namespace, repoName, visibilityWord(private)),
		Result:     "ok",
	})

	output.Success("Created %s/%s", namespace, repoName)
	return false, nil
}

func visibilityWord(private bool) string {
	if private {
		return "private"
	}
	return "public"
}

// addOneRepo adds a single repo to col, reporting and auditing the result.
// Factored out of runAdd so adding several repos in one invocation can
// continue past an individual failure instead of aborting the whole batch.
func addOneRepo(col *collection.Collection, name, caller, callerID, repoName string, client api.Client, private bool) error {
	for _, r := range col.Repos {
		if r.Name == repoName {
			return fmt.Errorf("already in collection %q", name)
		}
	}

	archived, err := ensureRepoExists(col, repoName, client, api.UserInfo{ID: callerID, Login: caller}, private)
	if err != nil {
		return err
	}

	if archived {
		output.Warn("%s is archived on GitHub (read-only — cannot push)", repoName)
		if addIsTerminalFn() {
			if !addConfirmFn("Add anyway?") {
				output.Info("Skipped %s", repoName)
				return errSkipped
			}
		}
	}

	col.Repos = append(col.Repos, collection.RepoAccess{Name: repoName, Groups: []string{}, Users: []string{}})

	added, _, syncErr := col.SyncCollaborators(client, nil)
	if syncErr != nil {
		col.Repos = col.Repos[:len(col.Repos)-1]
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "repo.add",
			Target:     repoName,
			Detail:     "Failed to sync access for new repo",
			Result:     "error: " + syncErr.Error(),
		})
		return fmt.Errorf("could not sync access for %s: %w", repoName, syncErr)
	}

	if err := col.Save(); err != nil {
		return err
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "repo.add",
		Target:     repoName,
		Detail:     fmt.Sprintf("Added repo, open to all members (%d granted)", added),
		Result:     "ok",
	})

	if archived {
		output.Success("Added %s to %q (archived — read-only)", repoName, name)
	} else {
		output.Success("Added %s to %q (open to all %d members)", repoName, name, len(col.Members))
	}
	output.Suggestion(fmt.Sprintf("gitcollect repo access %s %s --groups <g1,g2>", name, repoName))
	return nil
}
