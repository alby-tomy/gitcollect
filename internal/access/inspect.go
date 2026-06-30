package access

import "github.com/alby-tomy/gitcollect/internal/collection"

// RepoAccessDetail is one row of a per-user access report: a repo and
// whether/why the user can reach it.
type RepoAccessDetail struct {
	RepoName  string
	CanAccess bool
	Reason    string
	FixCmd    string
}

// UserAccessMap returns the access detail for every repo for the user
// identified by id. login is that user's current username, needed only to
// build FixCmd's suggested commands (see collection.Collection.FixCmd's
// doc comment for why login can't be derived from col.Logins[id] here —
// id is frequently someone who isn't a member of col at all, the most
// common case needing a fix in the first place).
func UserAccessMap(col *collection.Collection, id, login string) []RepoAccessDetail {
	details := make([]RepoAccessDetail, 0, len(col.Repos))
	for _, r := range col.Repos {
		details = append(details, RepoAccessDetail{
			RepoName:  r.Name,
			CanAccess: col.CanAccessRepo(id, r.Name),
			Reason:    col.WhyCanAccess(id, r.Name),
			FixCmd:    col.FixCmd(id, login, r.Name),
		})
	}
	return details
}

// MemberAccessDetail is one row of a per-repo access report: a member and
// whether/why they can reach the repo.
type MemberAccessDetail struct {
	Username  string // the member's current cached login (col.Logins[id]), for display
	CanAccess bool
	Reason    string
	FixCmd    string
}

// RepoAccessMap returns the access detail for every member for repoName.
func RepoAccessMap(col *collection.Collection, repoName string) []MemberAccessDetail {
	details := make([]MemberAccessDetail, 0, len(col.Members))
	for _, id := range col.Members {
		login := col.Logins[id]
		details = append(details, MemberAccessDetail{
			Username:  login,
			CanAccess: col.CanAccessRepo(id, repoName),
			Reason:    col.WhyCanAccess(id, repoName),
			FixCmd:    col.FixCmd(id, login, repoName),
		})
	}
	return details
}

// AccessMatrix is a grid of every member x every repo for display as a
// table: Grid[i][j] is whether Members[i] can access Repos[j], and
// Reasons[i][j] explains why or why not. Members holds each member's
// current cached login (for display) — never their platform ID.
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

	logins := make([]string, len(col.Members))
	grid := make([][]bool, len(col.Members))
	reasons := make([][]string, len(col.Members))
	for i, id := range col.Members {
		logins[i] = col.Logins[id]
		grid[i] = make([]bool, len(repoNames))
		reasons[i] = make([]string, len(repoNames))
		for j, repoName := range repoNames {
			grid[i][j] = col.CanAccessRepo(id, repoName)
			reasons[i][j] = col.WhyCanAccess(id, repoName)
		}
	}

	return AccessMatrix{
		Members: logins,
		Repos:   repoNames,
		Grid:    grid,
		Reasons: reasons,
	}
}
