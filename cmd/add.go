package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var addCmd = &cobra.Command{
	Use:   "add <collection> <repo>",
	Short: "Add a repo to a collection, open to all members by default",
	Args:  cobra.ExactArgs(2),
	RunE:  runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoName := args[1]

	if err := collection.ValidateRepoName(repoName); err != nil {
		return NewUsageError(fmt.Errorf("add: %w", err))
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

	for _, r := range col.Repos {
		if r.Name == repoName {
			return fmt.Errorf("add: %q is already in collection %q", repoName, name)
		}
	}

	if _, err := client.GetRepo(col.Owner, repoName); err != nil {
		return fmt.Errorf("add: could not find %s/%s: %w", col.Owner, repoName, err)
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
		return fmt.Errorf("add: could not sync access for %s: %w", repoName, syncErr)
	}

	if err := col.Save(); err != nil {
		return fmt.Errorf("add: %w", err)
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
