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
)

var addCmd = &cobra.Command{
	Use:   "add <collection> <repo> [repo...]",
	Short: "Add one or more repos to a collection, open to all members by default",
	Args:  cobra.MinimumNArgs(2),
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
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoNames := args[1:]

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

// ensureRepoExists checks whether repoName exists under col.RepoNamespace().
// If it does, returns nil. If it does not exist and the context is interactive,
// asks the owner whether to create it; on confirmation, creates the repo and
// audits the action. Returns errSkipped if the user declines. In
// non-interactive contexts (stdout is not a TTY) returns an error immediately.
func ensureRepoExists(col *collection.Collection, repoName string, client api.Client, caller api.UserInfo, private bool) error {
	namespace := col.RepoNamespace()

	_, err := client.GetRepo(namespace, repoName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, api.ErrNotFound) {
		return fmt.Errorf("checking repo %q: %w", repoName, err)
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("repo %q not found under %s (running non-interactively — create it manually first)", repoName, namespace)
	}

	output.Warn("repo %q does not exist under %s", repoName, namespace)
	if !output.Confirm(fmt.Sprintf("Create %s/%s as a %s repository?", namespace, repoName, visibilityWord(private))) {
		return errSkipped
	}

	_, createErr := client.CreateRepo(namespace, repoName, private, "")
	if errors.Is(createErr, api.ErrNameConflict) {
		output.Info("repo %q was just created by someone else — continuing", repoName)
		return nil
	}
	if createErr != nil {
		return fmt.Errorf("create repo %q: %w", repoName, createErr)
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
	return nil
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

	if err := ensureRepoExists(col, repoName, client, api.UserInfo{ID: callerID, Login: caller}, private); err != nil {
		return err
	}

	col.Repos = append(col.Repos, collection.RepoAccess{Name: repoName, Groups: []string{}, Users: []string{}})

	added, _, syncErr := col.SyncCollaborators(client)
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

	output.Success("Added %s to %q (open to all %d members)", repoName, name, len(col.Members))
	output.Suggestion(fmt.Sprintf("gitcollect repo access %s %s --groups <g1,g2>", name, repoName))
	return nil
}
