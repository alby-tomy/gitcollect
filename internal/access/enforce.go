// Package access bridges the local collection manifest and the platform
// API: it decides whether a caller is allowed to do something, drives the
// platform to match the manifest's intent, and explains access decisions
// for the inspect commands.
package access

import (
	"errors"
	"fmt"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
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

// CheckCollectionAccess verifies callerID can use col. On private
// collections, "not found" and "not a member" both produce ErrForbidden.
// callerID is a platform ID, not a login — see collection.Collection's
// Owner/Members doc comments. Callers in cmd/ keep a separate, login-typed
// caller variable for display/audit; only the ID form is passed in here.
func CheckCollectionAccess(col *collection.Collection, callerID string) error {
	if col.Visibility == collection.VisibilityPublic {
		return nil
	}
	if col.IsOwner(callerID) {
		return nil
	}
	if !col.IsMember(callerID) {
		return ErrForbidden
	}
	return nil
}

// CheckRepoAccess verifies callerID can reach repoName in col: they must be
// a collection member (or owner), satisfy the local CanAccessRepo rule, and
// actually hold collaborator access on the platform. All three must pass.
// CanAccessRepo itself always passes the owner, so there's no separate
// owner check needed here. The platform check resolves both the owner's
// and callerID's logins via col.Logins before calling the API, since
// CheckCollaborator (like every api.Client method) addresses accounts by
// login, never by ID.
func CheckRepoAccess(col *collection.Collection, repoName, callerID string, client api.Client) error {
	if err := CheckCollectionAccess(col, callerID); err != nil {
		return err
	}

	if !col.CanAccessRepo(callerID, repoName) {
		return fmt.Errorf("%w: %s", ErrGroupDenied, col.WhyCanAccess(callerID, repoName))
	}

	ownerLogin := col.RepoNamespace()
	has, err := client.CheckCollaborator(ownerLogin, repoName, col.Logins[callerID])
	if err != nil {
		// Inconclusive, not denied. See the note on unverifiedWarning: the
		// local rules above have already authorised this caller, and the
		// platform independently enforces the actual clone or pull, so a
		// check that could not be completed must not stand in for a "no".
		unverifiedWarning(1, err)
		return nil
	}
	if !has {
		return fmt.Errorf("%w: not yet a collaborator on %s/%s — access has not synced", ErrNoAccess, ownerLogin, repoName)
	}
	return nil
}

// unverifiedWarning reports that a platform access check could not be
// completed, and that gitcollect proceeded anyway.
//
// This is deliberately not a failure. The collaborator check is a courtesy:
// it turns a bare "git: authentication failed" into a sentence that names
// the collection and says access has not synced yet. It is not the security
// boundary — the caller has already passed the collection and repo rules in
// the manifest, and the platform enforces the real operation regardless of
// what gitcollect believes.
//
// Treating a failed check as a denial therefore bought no safety and cost
// the tool its function. Any 403 would do it: an exhausted rate limit, a
// token missing a scope, or an endpoint that wants more than the read
// access gitcollect deliberately grants its members. In that last case
// every read-only member — the people this tool exists to serve — would be
// unable to clone, pull, sync, or see status, while the owner, whose token
// has admin, saw nothing wrong at all.
func unverifiedWarning(n int, err error) {
	switch {
	case n == 1:
		output.Warn("Could not confirm platform access: %v", err)
	default:
		output.Warn("Could not confirm platform access for %d repo(s): %v", n, err)
	}
	output.Dim("  Continuing — the platform enforces access on the operation itself.")
}

// FilterAccessible returns only the repos accessible to callerID, combining
// local rules with platform verification. col.AccessibleRepos already
// returns every repo for the owner (CanAccessRepo always passes them), so
// no separate owner branch is needed here.
func FilterAccessible(col *collection.Collection, callerID string, client api.Client) ([]collection.RepoAccess, error) {
	if err := CheckCollectionAccess(col, callerID); err != nil {
		return nil, err
	}

	candidates := col.AccessibleRepos(callerID)

	ownerLogin := col.RepoNamespace()
	callerLogin := col.Logins[callerID]

	accessible := make([]collection.RepoAccess, 0, len(candidates))
	var unverified int
	var firstErr error
	for _, repo := range candidates {
		has, err := client.CheckCollaborator(ownerLogin, repo.Name, callerLogin)
		if err != nil {
			// Keep the repo and carry on rather than abandoning the whole
			// listing on the first refusal: one unverifiable repo used to
			// empty the entire result, so a single rate-limited call made
			// status, clone and sync report nothing at all.
			unverified++
			if firstErr == nil {
				firstErr = err
			}
			accessible = append(accessible, repo)
			continue
		}
		if has {
			accessible = append(accessible, repo)
		}
	}
	if unverified > 0 {
		unverifiedWarning(unverified, firstErr)
	}
	return accessible, nil
}
