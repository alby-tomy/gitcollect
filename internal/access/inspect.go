package access

import "github.com/alby-tomy/gitcollect/internal/collection"

// RepoAccessDetail is one row of a per-user access report: a repo and
// whether/why the user can reach it.
type RepoAccessDetail struct {
	RepoName  string
	CanAccess bool
	Reason    string
}

// decide is the single place that resolves whether username can reach
// repoName, used by every function in this file. col.CanAccessRepo
// deliberately has no owner bypass of its own (PROMPT.md's documented
// decision table treats it as a pure member rule) — callers that need the
// owner to always pass, the way the platform itself always would, apply
// that bypass on top, same as enforce.go's CheckRepoAccess/FilterAccessible
// already do. Without it, col.CanAccessRepo(owner, ...) returns false
// whenever the owner isn't separately listed as a member, while
// col.WhyCanAccess still answers "owner" — a contradictory true/false-vs-
// reason pairing that used to leak into inspect's output before this fix.
func decide(col *collection.Collection, username, repoName string) (bool, string) {
	if col.Visibility == collection.VisibilityPublic {
		return true, "open to all members"
	}
	if username == col.Owner {
		return true, "owner"
	}
	return col.CanAccessRepo(username, repoName), col.WhyCanAccess(username, repoName)
}

// UserAccessMap returns the access detail for every repo for username.
func UserAccessMap(col *collection.Collection, username string) []RepoAccessDetail {
	details := make([]RepoAccessDetail, 0, len(col.Repos))
	for _, r := range col.Repos {
		can, reason := decide(col, username, r.Name)
		details = append(details, RepoAccessDetail{RepoName: r.Name, CanAccess: can, Reason: reason})
	}
	return details
}

// MemberAccessDetail is one row of a per-repo access report: a member and
// whether/why they can reach the repo.
type MemberAccessDetail struct {
	Username  string
	CanAccess bool
	Reason    string
}

// RepoAccessMap returns the access detail for every member for repoName.
func RepoAccessMap(col *collection.Collection, repoName string) []MemberAccessDetail {
	details := make([]MemberAccessDetail, 0, len(col.Members))
	for _, m := range col.Members {
		can, reason := decide(col, m, repoName)
		details = append(details, MemberAccessDetail{Username: m, CanAccess: can, Reason: reason})
	}
	return details
}

// AccessMatrix is a grid of every member x every repo for display as a
// table: Grid[i][j] is whether Members[i] can access Repos[j], and
// Reasons[i][j] explains why or why not.
type AccessMatrix struct {
	Members []string
	Repos   []string
	Grid    [][]bool
	Reasons [][]string
}

// FullMatrix returns the full member x repo access grid for col.
func FullMatrix(col *collection.Collection) AccessMatrix {
	repoNames := make([]string, len(col.Repos))
	for i, r := range col.Repos {
		repoNames[i] = r.Name
	}

	grid := make([][]bool, len(col.Members))
	reasons := make([][]string, len(col.Members))
	for i, m := range col.Members {
		grid[i] = make([]bool, len(repoNames))
		reasons[i] = make([]string, len(repoNames))
		for j, repoName := range repoNames {
			grid[i][j], reasons[i][j] = decide(col, m, repoName)
		}
	}

	return AccessMatrix{
		Members: append([]string{}, col.Members...),
		Repos:   repoNames,
		Grid:    grid,
		Reasons: reasons,
	}
}
