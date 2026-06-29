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

// groupsContaining returns the names of every group username belongs to,
// in manifest order.
func (c *Collection) groupsContaining(username string) []string {
	var groups []string
	for name := range c.Groups {
		if c.IsInGroup(username, name) {
			groups = append(groups, name)
		}
	}
	return groups
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
//	collection public                       → true
//	user not a member                       → false
//	repo.Groups=[] AND repo.Users=[]         → true  (open to all members)
//	user in any repo.Groups                 → true
//	user in repo.Users                      → true
//	none of the above                       → false
func (c *Collection) CanAccessRepo(username, repoName string) bool {
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
// CanAccessRepo would make for username and repoName.
func (c *Collection) WhyCanAccess(username, repoName string) string {
	if c.Visibility == VisibilityPublic {
		return "open to all members"
	}
	if username == c.Owner {
		return "owner"
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
	if len(repo.Groups) > 0 {
		return fmt.Sprintf("no access — group %s required", repo.Groups[0])
	}
	return "no access — individual grant required"
}
