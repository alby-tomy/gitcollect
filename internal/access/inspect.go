package access

import "github.com/alby-tomy/gitcollect/internal/collection"

// RepoAccessDetail is one row of a per-user access report: a repo and
// whether/why the user can reach it.
type RepoAccessDetail struct {
	RepoName  string
	CanAccess bool
	Reason    string
}

// UserAccessMap returns the access detail for every repo for username.
func UserAccessMap(col *collection.Collection, username string) []RepoAccessDetail {
	details := make([]RepoAccessDetail, 0, len(col.Repos))
	for _, r := range col.Repos {
		details = append(details, RepoAccessDetail{
			RepoName:  r.Name,
			CanAccess: col.CanAccessRepo(username, r.Name),
			Reason:    col.WhyCanAccess(username, r.Name),
		})
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
		details = append(details, MemberAccessDetail{
			Username:  m,
			CanAccess: col.CanAccessRepo(m, repoName),
			Reason:    col.WhyCanAccess(m, repoName),
		})
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
			grid[i][j] = col.CanAccessRepo(m, repoName)
			reasons[i][j] = col.WhyCanAccess(m, repoName)
		}
	}

	return AccessMatrix{
		Members: append([]string{}, col.Members...),
		Repos:   repoNames,
		Grid:    grid,
		Reasons: reasons,
	}
}
