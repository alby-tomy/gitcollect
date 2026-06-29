// Package access bridges the local collection manifest and the platform
// API: it decides whether a caller is allowed to do something, drives the
// platform to match the manifest's intent, and explains access decisions
// for the inspect commands.
package access

import (
	"errors"
	"fmt"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

var (
	// ErrForbidden covers both "collection not found" and "not a member" on
	// private collections, so non-members can never distinguish the two —
	// gitcollect never confirms a private collection's existence to them.
	ErrForbidden = errors.New("collection not found or access denied")
	// ErrGroupDenied indicates the caller is a collection member but does
	// not satisfy any repo's group/user restriction.
	ErrGroupDenied = errors.New("access denied: required group membership not held")
	// ErrNoAccess indicates the local manifest grants access but the
	// platform has not (yet) synced it.
	ErrNoAccess = errors.New("access denied")
)

// CheckCollectionAccess verifies caller can use col. On private
// collections, "not found" and "not a member" both produce ErrForbidden.
func CheckCollectionAccess(col *collection.Collection, caller string) error {
	if col.Visibility == collection.VisibilityPublic {
		return nil
	}
	if caller == col.Owner {
		return nil
	}
	if !col.IsMember(caller) {
		return ErrForbidden
	}
	return nil
}

// CheckRepoAccess verifies caller can reach repoName in col: they must be a
// collection member (or owner), satisfy the local CanAccessRepo rule, and
// actually hold collaborator access on the platform. All three must pass.
// CanAccessRepo itself always passes the owner, so there's no separate
// owner check needed here.
func CheckRepoAccess(col *collection.Collection, repoName, caller string, client api.Client) error {
	if err := CheckCollectionAccess(col, caller); err != nil {
		return err
	}

	if !col.CanAccessRepo(caller, repoName) {
		return fmt.Errorf("%w: %s", ErrGroupDenied, col.WhyCanAccess(caller, repoName))
	}

	has, err := client.CheckCollaborator(col.Owner, repoName, caller)
	if err != nil {
		return fmt.Errorf("could not verify platform access to %s: %w", repoName, err)
	}
	if !has {
		return fmt.Errorf("%w: not yet a collaborator on %s/%s — access has not synced", ErrNoAccess, col.Owner, repoName)
	}
	return nil
}

// FilterAccessible returns only the repos accessible to caller, combining
// local rules with platform verification. col.AccessibleRepos already
// returns every repo for the owner (CanAccessRepo always passes them), so
// no separate owner branch is needed here.
func FilterAccessible(col *collection.Collection, caller string, client api.Client) ([]collection.RepoAccess, error) {
	if err := CheckCollectionAccess(col, caller); err != nil {
		return nil, err
	}

	candidates := col.AccessibleRepos(caller)

	accessible := make([]collection.RepoAccess, 0, len(candidates))
	for _, repo := range candidates {
		has, err := client.CheckCollaborator(col.Owner, repo.Name, caller)
		if err != nil {
			return nil, fmt.Errorf("could not verify platform access to %s: %w", repo.Name, err)
		}
		if has {
			accessible = append(accessible, repo)
		}
	}
	return accessible, nil
}
