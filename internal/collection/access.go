package collection

import "fmt"

// IsMember returns true if the collection is public (everyone is an
// implicit member) or username is in the Members list.
func (c *Collection) IsMember(username string) bool {
	if c.Visibility == VisibilityPublic {
		return true
	}
	for _, m := range c.Members {
		if m == username {
			return true
		}
	}
	return false
}

// IsInGroup returns true if username belongs to the named group.
func (c *Collection) IsInGroup(username, group string) bool {
	for _, u := range c.Groups[group] {
		if u == username {
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

// CanAccessRepo returns true if username can clone/pull repoName, per the
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
// The owner bypass lives here (not just in enforce.go's callers) so every
// caller of CanAccessRepo/WhyCanAccess — UserAccessMap, RepoAccessMap,
// FullMatrix, member.go's printAccessBreakdown, show.go's YOU column —
// gets a correct answer for the owner without needing its own copy of the
// bypass. Before this, only enforce.go's CheckRepoAccess/FilterAccessible
// applied it, and inspect.go's three functions didn't, producing a real,
// reachable bug: CanAccess=false paired with Reason="owner" whenever the
// owner wasn't separately listed as a member (fixed properly here instead
// of patched per-caller in session 9).
func (c *Collection) CanAccessRepo(username, repoName string) bool {
	if username == c.Owner {
		return true
	}
	if c.Visibility == VisibilityPublic {
		return true
	}
	if !c.IsMember(username) {
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
		if c.IsInGroup(username, g) {
			return true
		}
	}
	for _, u := range repo.Users {
		if u == username {
			return true
		}
	}
	return false
}

// AccessibleRepos returns the repos username can access, preserving the
// manifest's repo order.
func (c *Collection) AccessibleRepos(username string) []RepoAccess {
	var accessible []RepoAccess
	for _, r := range c.Repos {
		if c.CanAccessRepo(username, r.Name) {
			accessible = append(accessible, r)
		}
	}
	return accessible
}

// WhyCanAccess returns a human-readable reason for the access decision
// CanAccessRepo would make for username and repoName. Checks are kept in
// the same order as CanAccessRepo so the two can never disagree.
func (c *Collection) WhyCanAccess(username, repoName string) string {
	if username == c.Owner {
		return "owner — full access"
	}
	if c.Visibility == VisibilityPublic {
		return "open to all members"
	}
	if !c.IsMember(username) {
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
		if c.IsInGroup(username, g) {
			return fmt.Sprintf("member of group %s", g)
		}
	}
	for _, u := range repo.Users {
		if u == username {
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

// FixCmd returns the exact gitcollect command the collection owner must
// run to grant username access to repoName, or "" if username already has
// access (there's nothing to fix) or repoName isn't in the collection.
// Used by show/inspect to turn a denial into an actionable next step
// instead of just an explanation.
func (c *Collection) FixCmd(username, repoName string) string {
	if c.CanAccessRepo(username, repoName) {
		return ""
	}
	if !c.IsMember(username) {
		return fmt.Sprintf("gitcollect member add %s %s", c.Name, username)
	}
	repo, ok := c.repoByName(repoName)
	if !ok || len(repo.Groups) == 0 {
		return fmt.Sprintf("gitcollect repo grant %s %s %s", c.Name, repoName, username)
	}
	return fmt.Sprintf("gitcollect group add %s %s %s", c.Name, repo.Groups[0], username)
}
