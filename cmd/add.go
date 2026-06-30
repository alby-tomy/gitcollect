package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var addCmd = &cobra.Command{
	Use:   "add <collection> <repo> [repo...]",
	Short: "Add one or more repos to a collection, open to all members by default",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoNames := args[1:]

	for _, repoName := range repoNames {
		if err := collection.ValidateRepoName(repoName); err != nil {
			return NewUsageError(fmt.Errorf("add: %w", err))
		}
	}

	col, err := loadCollection(name)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	caller, err := currentUser(client)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if caller != col.Owner {
		return fmt.Errorf("add: only %s (the owner) can add repos to %q", col.Owner, name)
	}

	var failed []string
	for _, repoName := range repoNames {
		if err := addOneRepo(col, name, caller, repoName, client); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", repoName, err))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("add: %d of %d failed: %s", len(failed), len(repoNames), strings.Join(failed, "; "))
	}
	return nil
}

// addOneRepo adds a single repo to col, reporting and auditing the result.
// Factored out of runAdd so adding several repos in one invocation can
// continue past an individual failure instead of aborting the whole batch.
func addOneRepo(col *collection.Collection, name, caller, repoName string, client api.Client) error {
	for _, r := range col.Repos {
		if r.Name == repoName {
			return fmt.Errorf("already in collection %q", name)
		}
	}

	if _, err := client.GetRepo(col.Owner, repoName); err != nil {
		return fmt.Errorf("could not find %s/%s: %w", col.Owner, repoName, err)
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
