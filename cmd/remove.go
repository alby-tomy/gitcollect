package cmd

import (
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

const removeMaxConcurrency = 4

var removeCmd = &cobra.Command{
	Use:   "remove <collection> <repo>",
	Short: "Remove a repo from a collection and revoke everyone's access to it",
	Args:  cobra.ExactArgs(2),
	RunE:  runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoName := args[1]

	col, caller, callerID, client, err := loadForOwner("remove", name)
	if err != nil {
		return err
	}

	idx := -1
	for i, r := range col.Repos {
		if r.Name == repoName {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("remove: %q is not in collection %q", repoName, name)
	}
	if !col.IsOwner(callerID) {
		return fmt.Errorf("remove: only %s (the owner) can remove repos from %q", col.Logins[col.Owner], name)
	}

	prompt := fmt.Sprintf("This will remove %q from %q and revoke access for %d member(s)", repoName, name, len(col.Members))
	if !output.ConfirmWord(prompt, repoName) {
		return fmt.Errorf("remove: aborted")
	}

	if err := revokeRepoAccess(col, repoName, client); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller,
			Action:     "repo.remove",
			Target:     repoName,
			Detail:     "Failed to revoke access before removing repo",
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("remove: could not revoke access for %s: %w", repoName, err)
	}

	col.Repos = append(col.Repos[:idx], col.Repos[idx+1:]...)
	if err := col.Save(); err != nil {
		return fmt.Errorf("remove: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      caller,
		Action:     "repo.remove",
		Target:     repoName,
		Detail:     "Removed repo and revoked all access",
		Result:     "ok",
	})

	output.Success("Removed %s from %q and revoked access for %d member(s)", repoName, name, len(col.Members))
	return nil
}

// revokeRepoAccess unconditionally removes every member's collaborator
// access from repoName, concurrently, before the repo is dropped from the
// manifest.
func revokeRepoAccess(col *collection.Collection, repoName string, client api.Client) error {
	ownerLogin := col.Logins[col.Owner]
	var (
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, removeMaxConcurrency)
		wg   sync.WaitGroup
	)
	for _, member := range col.Members {
		memberLogin := col.Logins[member]
		wg.Add(1)
		sem <- struct{}{}
		go func(memberLogin string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := client.RemoveCollaborator(ownerLogin, repoName, memberLogin); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", memberLogin, err))
				mu.Unlock()
			}
		}(memberLogin)
	}
	wg.Wait()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
