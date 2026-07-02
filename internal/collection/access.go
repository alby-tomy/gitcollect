package collection

import (
	"errors"
	"fmt"
)

var (
	// ErrGroupAdminsDisabled is returned when a group-admin-only operation is
	// attempted but GroupAdminsEnabled is false on the collection.
	ErrGroupAdminsDisabled = errors.New(
		"group admin support is not enabled — run: gitcollect scale <collection> organisation")
	// ErrWrongGroup is returned when a group admin tries to manage a group
	// they are not an admin of.
	ErrWrongGroup = errors.New("you are a group admin but not of this group")
	// ErrSelfTransfer is returned when a transfer target is the current owner.
	ErrSelfTransfer = errors.New("cannot transfer ownership to yourself")
	// ErrAdminPrivilegeEscalation is returned when a group admin tries to
	// assign another group admin — only the owner can do that.
	ErrAdminPrivilegeEscalation = errors.New("group admins cannot assign other group admins")
)

// IsOwner returns true if id is the collection's owner. id is a platform
// ID once the collection is on CurrentVersion (still a legacy username on
// a "1" file) — comparison works identically either way, since both sides
// of the comparison are in the same format for a given file. Factored out
// so every owner check in this package and its callers goes through one
// place instead of comparing against c.Owner directly.
func (c *Collection) IsOwner(id string) bool {
	return id == c.Owner
}

// IsMember returns true if the collection is public (everyone is an
// implicit member) or id is in the Members list.
func (c *Collection) IsMember(id string) bool {
	if c.Visibility == VisibilityPublic {
		return true
	}
	for _, m := range c.Members {
		if m == id {
			return true
		}
	}
	return false
}

// IsInGroup returns true if id belongs to the named group.
func (c *Collection) IsInGroup(id, group string) bool {
	for _, u := range c.Groups[group] {
		if u == id {
			return true
		}
	}
	return false
}

// repoByName returns the RepoAccess for repoName, or false if no such repo
// is part of the collection.
func (c *Collection) repoByName(repoName string) (RepoAccess, bool) {
	for _, r := range c.Repos {
		if r.Name == repoName {
			return r, true
		}
	}
	return RepoAccess{}, false
}

// CanAccessRepo returns true if id can clone/pull repoName, per the
// decision table:
//
//	caller is owner                         → true  (always, regardless of membership)
//	collection public                       → true
//	user not a member                       → false
//	repo.Groups=[] AND repo.Users=[]         → true  (open to all members)
//	user in any repo.Groups                 → true
//	user in repo.Users                      → true
//	none of the above                       → false
//
// id is a platform ID once the collection is on CurrentVersion (a legacy
// username on a still-"1" file) — every list/map it's compared against
// (Owner, Members, Groups, repo.Users) is in the same format for a given
// file, so the comparisons themselves don't need to know which format
// they're in.
//
// The owner bypass lives here (not just in enforce.go's callers) so every
// caller of CanAccessRepo/WhyCanAccess — UserAccessMap, RepoAccessMap,
// FullMatrix, member.go's printAccessBreakdown, show.go's YOU column —
// gets a correct answer for the owner without needing its own copy of the
// bypass. Before this, only enforce.go's CheckRepoAccess/FilterAccessible
// applied it, and inspect.go's three functions didn't, producing a real,
// reachable bug: CanAccess=false paired with Reason="owner" whenever the
// owner wasn't separately listed as a member (fixed properly here instead
// of patched per-caller in session 9).
func (c *Collection) CanAccessRepo(id, repoName string) bool {
	if c.IsOwner(id) {
		return true
	}
	if c.Visibility == VisibilityPublic {
		return true
	}
	if !c.IsMember(id) {
		return false
	}

	repo, ok := c.repoByName(repoName)
	if !ok {
		return false
	}
	if len(repo.Groups) == 0 && len(repo.Users) == 0 {
		return true
	}
	for _, g := range repo.Groups {
		if c.IsInGroup(id, g) {
			return true
		}
	}
	for _, u := range repo.Users {
		if u == id {
			return true
		}
	}
	return false
}

// AccessibleRepos returns the repos id can access, preserving the
// manifest's repo order.
func (c *Collection) AccessibleRepos(id string) []RepoAccess {
	var accessible []RepoAccess
	for _, r := range c.Repos {
		if c.CanAccessRepo(id, r.Name) {
			accessible = append(accessible, r)
		}
	}
	return accessible
}

// WhyCanAccess returns a human-readable reason for the access decision
// CanAccessRepo would make for id and repoName. Checks are kept in the
// same order as CanAccessRepo so the two can never disagree. Unlike
// FixCmd below, this never needs to print id itself — every branch either
// returns a fixed string or interpolates a group name — so it takes no
// login parameter.
func (c *Collection) WhyCanAccess(id, repoName string) string {
	if c.IsOwner(id) {
		return "owner — full access"
	}
	if c.Visibility == VisibilityPublic {
		return "open to all members"
	}
	if !c.IsMember(id) {
		return "no access — not a member"
	}

	repo, ok := c.repoByName(repoName)
	if !ok {
		return "no access — repo not in collection"
	}
	if len(repo.Groups) == 0 && len(repo.Users) == 0 {
		return "open to all members"
	}
	for _, g := range repo.Groups {
		if c.IsInGroup(id, g) {
			return fmt.Sprintf("member of group %s", g)
		}
	}
	for _, u := range repo.Users {
		if u == id {
			return "individually granted"
		}
	}
	switch {
	case len(repo.Groups) > 0 && len(repo.Users) > 0:
		return fmt.Sprintf("no access — group %s or individual grant required", repo.Groups[0])
	case len(repo.Groups) > 0:
		return fmt.Sprintf("no access — group %s required", repo.Groups[0])
	default:
		return "no access — individual grant required"
	}
}

// IsGroupAdmin returns true if callerID is a group admin of the named group
// AND GroupAdminsEnabled is true on the collection. Returns false in all
// other cases, including when the collection is in TEAM mode.
func (c *Collection) IsGroupAdmin(callerID, group string) bool {
	if !c.GroupAdminsEnabled {
		return false
	}
	for _, id := range c.GroupAdmins[group] {
		if id == callerID {
			return true
		}
	}
	return false
}

// CanManageGroup returns true if callerID can add/remove members of the named
// group: the collection owner, or a group admin of that specific group when
// GroupAdminsEnabled is true.
func (c *Collection) CanManageGroup(callerID, group string) bool {
	return c.IsOwner(callerID) || c.IsGroupAdmin(callerID, group)
}

// GroupAdminOf returns the list of group names callerID is an admin of.
// Returns an empty slice if GroupAdminsEnabled is false or caller is not an
// admin of any group.
func (c *Collection) GroupAdminOf(callerID string) []string {
	if !c.GroupAdminsEnabled {
		return []string{}
	}
	var groups []string
	for group, admins := range c.GroupAdmins {
		for _, id := range admins {
			if id == callerID {
				groups = append(groups, group)
				break
			}
		}
	}
	return groups
}

// FixCmd returns the exact gitcollect command the collection owner must
// run to grant id access to repoName, or "" if id already has access
// (there's nothing to fix) or repoName isn't in the collection. login is
// id's current username, used only for building a typeable command — it
// is taken as a parameter rather than resolved via c.Logins[id] because
// the most common case needing a fix is exactly "id isn't a member of
// this collection yet," meaning c.Logins has no entry for them at all;
// the caller (cmd/show.go, cmd/inspect.go) already has the login on hand
// from resolving id in the first place and can pass it straight through.
// Used by show/inspect to turn a denial into an actionable next step
// instead of just an explanation.
func (c *Collection) FixCmd(id, login, repoName string) string {
	if c.CanAccessRepo(id, repoName) {
		return ""
	}
	if !c.IsMember(id) {
		return fmt.Sprintf("gitcollect member add %s %s", c.Name, login)
	}
	repo, ok := c.repoByName(repoName)
	if !ok || len(repo.Groups) == 0 {
		return fmt.Sprintf("gitcollect repo access %s %s --users %s", c.Name, repoName, login)
	}
	return fmt.Sprintf("gitcollect group add %s %s %s", c.Name, repo.Groups[0], login)
}
