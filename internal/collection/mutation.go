package collection

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alby-tomy/gitcollect/internal/api"
)

// maxConcurrentSyncs bounds how many collaborator API calls run at once
// during a sync.
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

// SyncCollaborators recomputes the correct platform collaborator state for
// every (member, repo) pair in the collection and drives the API to match
// it, running up to maxConcurrentSyncs calls concurrently. Pairs that
// already match the desired state are left untouched. Partial failures are
// collected and returned as a joined error; pairs that succeeded are still
// applied even if others failed. The collection itself is never modified —
// callers decide whether to persist after a successful sync.
func (c *Collection) SyncCollaborators(client api.Client) (added, removed int, err error) {
	type job struct {
		member     string
		repo       string
		shouldHave bool
	}

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

			has, checkErr := client.CheckCollaborator(c.Owner, j.repo, j.member)
			if checkErr != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: check %s: %w", c.Owner, j.repo, j.member, checkErr))
				mu.Unlock()
				return
			}

			switch {
			case j.shouldHave && !has:
				if addErr := client.AddCollaborator(c.Owner, j.repo, j.member, "pull"); addErr != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s/%s: grant %s: %w", c.Owner, j.repo, j.member, addErr))
					mu.Unlock()
					return
				}
				mu.Lock()
				addedN++
				mu.Unlock()
			case !j.shouldHave && has:
				if rmErr := client.RemoveCollaborator(c.Owner, j.repo, j.member); rmErr != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s/%s: revoke %s: %w", c.Owner, j.repo, j.member, rmErr))
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

// revokeAllAccess unconditionally removes username's collaborator access
// from every repo in the collection, concurrently. Used when a member
// leaves the collection entirely, since once they're removed from Members
// they no longer appear in SyncCollaborators' iteration.
func (c *Collection) revokeAllAccess(username string, client api.Client) error {
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
			if err := client.RemoveCollaborator(c.Owner, repoName, username); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: revoke %s: %w", c.Owner, repoName, username, err))
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

// AddMember adds username to Members, then syncs platform access.
// No-op if already a member.
func (c *Collection) AddMember(username string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if c.IsMember(username) {
		return nil
	}

	original := c.Members
	c.Members = append(append([]string{}, c.Members...), username)

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Members = original
		return fmt.Errorf("could not sync access for %s: %w", username, err)
	}
	return c.Save()
}

// RemoveMember removes username from Members and all Groups, then calls
// the API to remove their collaborator access from every repo. No-op if
// not a member.
func (c *Collection) RemoveMember(username string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if !c.IsMember(username) {
		return nil
	}

	if err := c.revokeAllAccess(username, client); err != nil {
		return fmt.Errorf("could not revoke access for %s: %w", username, err)
	}

	c.Members = removeString(c.Members, username)
	newGroups := make(map[string][]string, len(c.Groups))
	for g, users := range c.Groups {
		newGroups[g] = removeString(users, username)
	}
	c.Groups = newGroups

	return c.Save()
}

// AddToGroup adds username to group. username must already be a member.
// After success: calls SyncCollaborators to update platform access.
func (c *Collection) AddToGroup(username, group string, client api.Client) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if err := ValidateGroupName(group); err != nil {
		return err
	}
	if !c.IsMember(username) {
		return fmt.Errorf("%w: %s", ErrNotMember, username)
	}
	if _, ok := c.Groups[group]; !ok {
		return fmt.Errorf("%w: %s", ErrGroupNotFound, group)
	}
	if c.IsInGroup(username, group) {
		return nil
	}

	original := append([]string{}, c.Groups[group]...)
	c.Groups[group] = append(append([]string{}, c.Groups[group]...), username)

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Groups[group] = original
		return fmt.Errorf("could not sync access for %s: %w", username, err)
	}
	return c.Save()
}

// RemoveFromGroup removes username from group. After success: calls
// SyncCollaborators to recalculate their repo access.
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
	if !c.IsInGroup(username, group) {
		return nil
	}

	original := append([]string{}, c.Groups[group]...)
	c.Groups[group] = removeString(c.Groups[group], username)

	if _, _, err := c.SyncCollaborators(client); err != nil {
		c.Groups[group] = original
		return fmt.Errorf("could not sync access for %s: %w", username, err)
	}
	return c.Save()
}

// SetRepoAccess updates the Groups and Users for repoName. After success:
// calls SyncCollaborators so the platform reflects the new rules. Passing
// groups=[] and users=[] opens the repo to all members.
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
		if !c.IsMember(u) {
			return fmt.Errorf("%w: %s", ErrNotMember, u)
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

	original := c.Repos[idx]
	c.Repos[idx].Groups = append([]string{}, groups...)
	c.Repos[idx].Users = append([]string{}, users...)

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
