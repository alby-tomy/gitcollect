package collection

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alby-tomy/gitcollect/internal/api"
)

// maxConcurrentSyncs bounds how many collaborator API calls run at once
// during a sync, and how many GetUser resolutions run at once (resolveUsers
// below).
const maxConcurrentSyncs = 4

var (
	ErrNotMember     = errors.New("not a member")
	ErrGroupNotFound = errors.New("group not found")
	ErrGroupExists   = errors.New("group already exists")
	ErrGroupInUse    = errors.New("group is referenced by one or more repos")
	ErrRepoNotFound  = errors.New("repo not found")
)

func removeString(list []string, target string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// cloneLogins returns a shallow copy of m. Maps are reference types, so a
// mutation function that needs to roll back a failed sync must mutate a
// copy and restore the original map afterward — reassigning the same
// (already mutated) map back to itself would not undo anything, unlike
// the existing slice rollback pattern (original := c.Members; c.Members =
// append(append([]string{}, c.Members...), id)) used throughout this file,
// which works precisely because append([]string{}, ...) always allocates
// a fresh backing array.
func cloneLogins(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// resolveUsers concurrently resolves each username in usernames to its
// platform identity, at most maxConcurrentSyncs in flight at once (the
// same bound SyncCollaborators uses), returning results in the same order
// as the input. There is no bulk-lookup endpoint on either platform, so
// this is N individual GetUser calls, just not sequential — used by
// SetRepoAccess for its --users list, the one mutation that accepts more
// than one username in a single call.
func resolveUsers(usernames []string, client api.Client) ([]api.UserInfo, error) {
	results := make([]api.UserInfo, len(usernames))
	errs := make([]error, len(usernames))

	var (
		sem = make(chan struct{}, maxConcurrentSyncs)
		wg  sync.WaitGroup
	)
	for i, username := range usernames {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, username string) {
			defer wg.Done()
			defer func() { <-sem }()
			user, err := client.GetUser(username)
			if err != nil {
				errs[i] = fmt.Errorf("could not resolve %s on the platform: %w", username, err)
				return
			}
			results[i] = user
		}(i, username)
	}
	wg.Wait()

	var joined []error
	for _, e := range errs {
		if e != nil {
			joined = append(joined, e)
		}
	}
	if len(joined) > 0 {
		return nil, errors.Join(joined...)
	}
	return results, nil
}

// IDForLogin returns the platform ID whose cached Logins entry equals
// login, or "" if none is cached under that login (including: login was
// never a member of this collection at all). Used by every remove/revoke
// mutation below instead of client.GetUser — a removal only needs to
// match something already recorded locally, and must keep working even if
// the platform account behind login has since been renamed or deleted,
// which a live lookup would fail on. Exported so cmd/'s remove-style
// commands can do the same no-network "is this even a thing to remove"
// check before calling into this package.
func (c *Collection) IDForLogin(login string) string {
	for id, l := range c.Logins {
		if l == login {
			return id
		}
	}
	return ""
}

// Migrate resolves every legacy username referenced anywhere in c (Owner,
// every Members entry, every group's members, every repo's individually-
// granted users) to its platform ID via client, populates Logins, and
// bumps Version to CurrentVersion. No-op if c is already on CurrentVersion.
// Does not call Save — the caller decides when to persist, the same
// convention SyncCollaborators uses. Reuses resolveUsers so every
// username referenced anywhere in the file is resolved exactly once
// (concurrently, max maxConcurrentSyncs in flight) even if it appears in
// multiple places — e.g. the owner is almost always also a member.
//
// Called only by cmd/root.go's migrateIfNeeded, itself called only from
// call sites that already hold an authenticated client for c.Host — never
// from a context that's supposed to stay network-free or auth-free. See
// migrateIfNeeded's doc comment for exactly which call sites those are.
func (c *Collection) Migrate(client api.Client) error {
	if c.Version == CurrentVersion {
		return nil
	}

	usernameSet := map[string]bool{c.Owner: true}
	for _, m := range c.Members {
		usernameSet[m] = true
	}
	for _, users := range c.Groups {
		for _, u := range users {
			usernameSet[u] = true
		}
	}
	for _, r := range c.Repos {
		for _, u := range r.Users {
			usernameSet[u] = true
		}
	}

	usernames := make([]string, 0, len(usernameSet))
	for u := range usernameSet {
		usernames = append(usernames, u)
	}

	resolvedList, err := resolveUsers(usernames, client)
	if err != nil {
		return err
	}
	resolved := make(map[string]api.UserInfo, len(usernames))
	for i, u := range usernames {
		resolved[u] = resolvedList[i]
	}

	c.Owner = resolved[c.Owner].ID
	for i, m := range c.Members {
		c.Members[i] = resolved[m].ID
	}
	for group, users := range c.Groups {
		ids := make([]string, len(users))
		for i, u := range users {
			ids[i] = resolved[u].ID
		}
		c.Groups[group] = ids
	}
	for i, r := range c.Repos {
		ids := make([]string, len(r.Users))
		for j, u := range r.Users {
			ids[j] = resolved[u].ID
		}
		c.Repos[i].Users = ids
	}

	logins := make(map[string]string, len(resolved))
	for _, u := range resolved {
		logins[u.ID] = u.Login
	}
	c.Logins = logins
	c.Version = CurrentVersion
	return nil
}

// SyncCollaborators recomputes the correct platform collaborator state for
// every (member, repo) pair in the collection and drives the API to match
// it, running up to maxConcurrentSyncs calls concurrently. Pairs that
// already match the desired state are left untouched. Partial failures are
// collected and returned as a joined error; pairs that succeeded are still
// applied even if others failed. The collection itself is never modified —
// callers decide whether to persist after a successful sync.
//
// Precondition: c.Owner and every c.Members entry already have a Logins
// entry. This holds for any Collection reached through cmd/root.go's
// loadCollection/loadForGit/loadForRead (which migrate an old-format file
// to CurrentVersion, populating Logins, before any mutation method ever
// runs) — SyncCollaborators itself does no version branching and assumes
// the precondition rather than re-deriving it. A missing Logins entry for
// some member is treated as a per-job error, not a panic — see the
// memberLogin == "" check below — purely as a defensive backstop, since
// it should not be reachable given the precondition.
func (c *Collection) SyncCollaborators(client api.Client) (added, removed int, err error) {
	type job struct {
		member     string
		repo       string
		shouldHave bool
	}

	ownerLogin := c.RepoNamespace()

	var jobs []job
	for _, member := range c.Members {
		for _, repo := range c.Repos {
			jobs = append(jobs, job{member: member, repo: repo.Name, shouldHave: c.CanAccessRepo(member, repo.Name)})
		}
	}

	var (
		mu       sync.Mutex
		errs     []error
		addedN   int
		removedN int
		sem      = make(chan struct{}, maxConcurrentSyncs)
		wg       sync.WaitGroup
	)

	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()

			memberLogin := c.Logins[j.member]
			if memberLogin == "" {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: member %s has no cached login", c.Owner, j.repo, j.member))
				mu.Unlock()
				return
			}

			has, checkErr := client.CheckCollaborator(ownerLogin, j.repo, memberLogin)
			if checkErr != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: check %s: %w", ownerLogin, j.repo, memberLogin, checkErr))
				mu.Unlock()
				return
			}

			switch {
			case j.shouldHave && !has:
				if addErr := client.AddCollaborator(ownerLogin, j.repo, memberLogin, "pull"); addErr != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s/%s: grant %s: %w", ownerLogin, j.repo, memberLogin, addErr))
					mu.Unlock()
					return
				}
				mu.Lock()
				addedN++
				mu.Unlock()
			case !j.shouldHave && has:
				if rmErr := client.RemoveCollaborator(ownerLogin, j.repo, memberLogin); rmErr != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s/%s: revoke %s: %w", ownerLogin, j.repo, memberLogin, rmErr))
					mu.Unlock()
					return
				}
				mu.Lock()
				removedN++
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()

	if len(errs) > 0 {
		return addedN, removedN, errors.Join(errs...)
	}
	return addedN, removedN, nil
}

// revokeAllAccess unconditionally removes id's collaborator access from
// every repo in the collection, concurrently. Used when a member leaves
// the collection entirely, since once they're removed from Members they
// no longer appear in SyncCollaborators' iteration. Same Logins
// precondition as SyncCollaborators.
func (c *Collection) revokeAllAccess(id string, client api.Client) error {
	login := c.Logins[id]
	ownerLogin := c.RepoNamespace()

	var (
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, maxConcurrentSyncs)
		wg   sync.WaitGroup
	)

	for _, r := range c.Repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(repoName string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := client.RemoveCollaborator(ownerLogin, repoName, login); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: revoke %s: %w", ownerLogin, repoName, login, err))
				mu.Unlock()
			}
		}(r.Name)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// AddMember resolves username to its platform identity via a live lookup
// — never a cached Logins entry, since granting access is exactly the
// case where re-confirming against the real platform matters — adds the
// resolved ID to Members, caches the login, and syncs platform access.
// No-op if already a member. Rolls back both Members and Logins together
// if the sync fails, via cloneLogins, so a partial failure never leaves
// the cached login for an ID that didn't actually make it into Members.
func (c *Collection) AddMember(username string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	user, err := client.GetUser(username)
	if err != nil {
		return fmt.Errorf("could not resolve %s on the platform: %w", username, err)
	}
	if c.IsMember(user.ID) {
		return nil
	}

	originalMembers := c.Members
	originalLogins := c.Logins
	c.Members = append(append([]string{}, c.Members...), user.ID)
	c.Logins = cloneLogins(c.Logins)
	c.Logins[user.ID] = user.Login

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Members = originalMembers
		c.Logins = originalLogins
		return fmt.Errorf("could not sync access for %s: %w", username, err)
	}
	return c.Save()
}

// RemoveMember removes username from Members and all Groups, then calls
// the API to remove their collaborator access from every repo. No-op if
// not a member — including if username isn't found in Logins at all,
// which means they were never a member to begin with. Unlike AddMember,
// this never calls client.GetUser: see IDForLogin's doc comment for why
// removal works from the cache instead of a live resolve.
func (c *Collection) RemoveMember(username string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	id := c.IDForLogin(username)
	if id == "" || !c.IsMember(id) {
		return nil
	}

	if err := c.revokeAllAccess(id, client); err != nil {
		return fmt.Errorf("could not revoke access for %s: %w", username, err)
	}

	c.Members = removeString(c.Members, id)
	newGroups := make(map[string][]string, len(c.Groups))
	for g, users := range c.Groups {
		newGroups[g] = removeString(users, id)
	}
	c.Groups = newGroups

	return c.Save()
}

// AddToGroup adds username to group. username must already be a member.
// After success: calls SyncCollaborators to update platform access. A
// grant, like AddMember, so username is always resolved live.
func (c *Collection) AddToGroup(username, group string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if err := ValidateGroupName(group); err != nil {
		return err
	}
	if _, ok := c.Groups[group]; !ok {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, group)
	}

	user, err := client.GetUser(username)
	if err != nil {
		return fmt.Errorf("could not resolve %s on the platform: %w", username, err)
	}
	if !c.IsMember(user.ID) {
		return fmt.Errorf("%w: %s", ErrNotMember, username)
	}
	if c.IsInGroup(user.ID, group) {
		return nil
	}

	original := append([]string{}, c.Groups[group]...)
	c.Groups[group] = append(append([]string{}, c.Groups[group]...), user.ID)

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Groups[group] = original
		return fmt.Errorf("could not sync access for %s: %w", username, err)
	}
	return c.Save()
}

// RemoveFromGroup removes username from group. After success: calls
// SyncCollaborators to recalculate their repo access. A revoke, like
// RemoveMember, so the ID comes from IDForLogin, never a live resolve.
func (c *Collection) RemoveFromGroup(username, group string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if err := ValidateGroupName(group); err != nil {
		return err
	}
	if _, ok := c.Groups[group]; !ok {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, group)
	}

	id := c.IDForLogin(username)
	if id == "" || !c.IsInGroup(id, group) {
		return nil
	}

	original := append([]string{}, c.Groups[group]...)
	c.Groups[group] = removeString(c.Groups[group], id)

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Groups[group] = original
		return fmt.Errorf("could not sync access for %s: %w", username, err)
	}
	return c.Save()
}

// SetRepoAccess updates the Groups and Users for repoName. After success:
// calls SyncCollaborators so the platform reflects the new rules. Passing
// groups=[] and users=[] opens the repo to all members. users is resolved
// concurrently via resolveUsers — a grant, like AddMember/AddToGroup, so
// always a live lookup, never IDForLogin/Logins.
func (c *Collection) SetRepoAccess(repoName string, groups, users []string, client api.Client) error {
	if err := ValidateRepoName(repoName); err != nil {
		return err
	}
	for _, g := range groups {
		if err := ValidateGroupName(g); err != nil {
			return err
		}
		if _, ok := c.Groups[g]; !ok {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, g)
		}
	}
	for _, u := range users {
		if err := ValidateUsername(u); err != nil {
			return err
		}
	}

	idx := -1
	for i, r := range c.Repos {
		if r.Name == repoName {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("%w: %s", ErrRepoNotFound, repoName)
	}

	resolved, err := resolveUsers(users, client)
	if err != nil {
		return err
	}
	userIDs := make([]string, len(resolved))
	for i, u := range resolved {
		if !c.IsMember(u.ID) {
			return fmt.Errorf("%w: %s", ErrNotMember, users[i])
		}
		userIDs[i] = u.ID
	}

	original := c.Repos[idx]
	c.Repos[idx].Groups = append([]string{}, groups...)
	c.Repos[idx].Users = userIDs

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Repos[idx] = original
		return fmt.Errorf("could not sync access for repo %s: %w", repoName, err)
	}
	return c.Save()
}

// CreateGroup creates a new empty group. Fails if name already exists.
func (c *Collection) CreateGroup(group string) error {
	if err := ValidateGroupName(group); err != nil {
		return err
	}
	if _, ok := c.Groups[group]; ok {
		return fmt.Errorf("%w: %s", ErrGroupExists, group)
	}
	c.Groups[group] = []string{}
	return c.Save()
}

// DeleteGroup deletes group. Fails if any repo references it — the caller
// must clear those repo restrictions first.
func (c *Collection) DeleteGroup(group string) error {
	if err := ValidateGroupName(group); err != nil {
		return err
	}
	if _, ok := c.Groups[group]; !ok {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, group)
	}

	var blocking []string
	for _, r := range c.Repos {
		for _, g := range r.Groups {
			if g == group {
				blocking = append(blocking, r.Name)
				break
			}
		}
	}
	if len(blocking) > 0 {
		return fmt.Errorf("%w: %s (used by: %s)", ErrGroupInUse, group, strings.Join(blocking, ", "))
	}

	delete(c.Groups, group)
	return c.Save()
}
