package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	importFrom               string
	importOrg                string
	importTeam               string
	importDryRun             bool
	importFlatten            bool
	importOwnerFromMaintainer bool
	importNamespace          string
	importMerge              bool
	importOverwrite          bool
	importSkipExisting       bool
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import GitHub/GitLab org structure as gitcollect collections",
	Long: `Read an organisation's team structure from GitHub or GitLab and create
gitcollect collection files automatically, one per team.

Each team becomes one collection. All team members become collection members.
The team maintainer (if any) becomes the collection owner.`,
	RunE: runImport,
}

func init() {
	importCmd.Flags().StringVar(&importFrom, "from", "", "platform to import from: github or gitlab (required)")
	importCmd.Flags().StringVar(&importOrg, "org", "", "organisation or group name (required)")
	importCmd.Flags().StringVar(&importTeam, "team", "", "import only this team slug (default: all teams)")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "preview what would be imported without writing files")
	importCmd.Flags().BoolVar(&importFlatten, "flatten", true, "treat nested teams as top-level collections")
	importCmd.Flags().BoolVar(&importOwnerFromMaintainer, "owner-from-maintainer", true, "set collection owner from team maintainer (falls back to importer)")
	importCmd.Flags().StringVar(&importNamespace, "namespace", "", "override namespace for all collections (default: org name)")
	importCmd.Flags().BoolVar(&importMerge, "merge", false, "merge imported data into existing collections without prompting")
	importCmd.Flags().BoolVar(&importOverwrite, "overwrite", false, "overwrite existing collections without prompting")
	importCmd.Flags().BoolVar(&importSkipExisting, "skip-existing", false, "skip teams whose collection already exists locally")
	rootCmd.AddCommand(importCmd)
}

// checkImportScopes verifies the GitHub token has the scopes required for
// import. Returns a clear, actionable error if any required scope is missing.
// Only called for GitHub — GitLab tokens don't expose an OAuth scope header.
func checkImportScopes(client api.Client) error {
	scopes, err := client.GetTokenScopes()
	if err != nil {
		return fmt.Errorf("import: could not check token scopes: %w", err)
	}

	scopeSet := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = true
	}

	var missing []string
	if !scopeSet["read:org"] {
		missing = append(missing, "read:org")
	}
	if !scopeSet["repo"] && !scopeSet["public_repo"] {
		missing = append(missing, "repo (or public_repo for public-only orgs)")
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"import: GitHub token is missing required scopes: %s\n\n"+
				"  Generate a new token at:\n"+
				"  https://github.com/settings/tokens/new?scopes=read:org,repo\n\n"+
				"  Then run: gitcollect auth",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

// collectionNameForTeam computes the collection name for a team based on the
// flatten flag. When flatten is true (default), uses the team slug directly.
// When flatten is false and the team has a parent, prefixes with the parent
// slug to preserve the hierarchy.
func collectionNameForTeam(team api.TeamInfo, flatten bool) string {
	if !flatten && team.ParentSlug != "" {
		return team.ParentSlug + "-" + team.Slug
	}
	return team.Slug
}

// buildCollectionFromTeam creates a Collection struct from the imported team
// data. name is the pre-computed collection name (may differ from team.Slug
// when --flatten=false and the team has a parent). The collection's file path
// is set to org-name.yaml so imports from different orgs don't clobber each
// other even if two orgs happen to have same-named teams.
func buildCollectionFromTeam(
	team api.TeamInfo,
	members []api.UserInfo,
	maintainers []api.UserInfo,
	repos []api.RepoInfo,
	org string,
	name string,
	namespace string,
	host string,
	ownerFromMaintainer bool,
	callerID, callerLogin string,
) (*collection.Collection, error) {
	ownerID := callerID
	ownerLogin := callerLogin
	if ownerFromMaintainer && len(maintainers) > 0 {
		ownerID = maintainers[0].ID
		ownerLogin = maintainers[0].Login
	}

	logins := make(map[string]string)
	logins[ownerID] = ownerLogin
	for _, m := range members {
		logins[m.ID] = m.Login
	}

	var memberIDs []string
	for _, m := range members {
		if m.ID != ownerID {
			memberIDs = append(memberIDs, m.ID)
		}
	}
	if memberIDs == nil {
		memberIDs = []string{}
	}

	var repoAccess []collection.RepoAccess
	for _, r := range repos {
		repoAccess = append(repoAccess, collection.RepoAccess{
			Name:   r.Name,
			Groups: []string{},
			Users:  []string{},
		})
	}
	if repoAccess == nil {
		repoAccess = []collection.RepoAccess{}
	}

	now := time.Now().UTC()
	col := &collection.Collection{
		Version:     collection.CurrentVersion,
		Name:        name,
		Description: team.Description,
		Host:        host,
		Namespace:   namespace,
		Owner:       ownerID,
		Visibility:  collection.VisibilityPrivate,
		Members:     memberIDs,
		Groups:      map[string][]string{},
		Repos:       repoAccess,
		Logins:      logins,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	col.SetPath(org + "-" + name)

	return col, col.Validate()
}

type importResult struct {
	team api.TeamInfo
	name string
	col  *collection.Collection
	err  error
}

// resolveConflict returns what action to take when a collection already
// exists: "merge", "overwrite", or "skip". In non-interactive mode (or when a
// flag is explicitly set) it resolves without prompting. On a TTY with no
// flags, it asks the user.
func resolveConflict(existingName string, existing *collection.Collection, imported *collection.Collection) string {
	if importOverwrite {
		return "overwrite"
	}
	if importSkipExisting {
		return "skip"
	}
	if importMerge {
		return "merge"
	}

	// Interactive prompt.
	output.Warn("Collection %q already exists locally.", existingName)
	fmt.Fprintf(os.Stderr, "  Local:  %d repos · %d members\n",
		len(existing.Repos), len(existing.Members))
	fmt.Fprintf(os.Stderr, "  Import: %d repos · %d members (platform current state)\n",
		len(imported.Repos), len(imported.Members))
	fmt.Fprintf(os.Stderr, "\n  [o] Overwrite local with imported data\n")
	fmt.Fprintf(os.Stderr, "  [s] Skip this collection\n")
	fmt.Fprintf(os.Stderr, "  [m] Merge — add new repos/members, keep existing ones\n")
	fmt.Fprintf(os.Stderr, "  Choice [o/s/m]: ")
	choice, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "o", "overwrite":
		return "overwrite"
	case "s", "skip":
		return "skip"
	default:
		return "merge"
	}
}

// mergeCollections adds members and repos from imported into existing without
// removing anything already present. Logins cache is merged from both.
func mergeCollections(existing, imported *collection.Collection) {
	memberSet := make(map[string]bool, len(existing.Members))
	for _, m := range existing.Members {
		memberSet[m] = true
	}
	for _, m := range imported.Members {
		if !memberSet[m] {
			existing.Members = append(existing.Members, m)
		}
	}

	repoSet := make(map[string]bool, len(existing.Repos))
	for _, r := range existing.Repos {
		repoSet[r.Name] = true
	}
	for _, r := range imported.Repos {
		if !repoSet[r.Name] {
			existing.Repos = append(existing.Repos, r)
		}
	}

	if existing.Logins == nil {
		existing.Logins = make(map[string]string)
	}
	for id, login := range imported.Logins {
		if existing.Logins[id] == "" {
			existing.Logins[id] = login
		}
	}
}

func runImport(_ *cobra.Command, _ []string) error {
	if importFrom == "" {
		return NewUsageError(fmt.Errorf("import: --from is required (github or gitlab)"))
	}
	if importOrg == "" {
		return NewUsageError(fmt.Errorf("import: --org is required"))
	}
	if importOverwrite && importSkipExisting {
		return NewUsageError(fmt.Errorf("import: --overwrite and --skip-existing are mutually exclusive"))
	}

	var host string
	switch strings.ToLower(importFrom) {
	case "github":
		host = "github.com"
	case "gitlab":
		host = "gitlab.com"
	default:
		return NewUsageError(fmt.Errorf("import: --from must be github or gitlab, got %q", importFrom))
	}

	client, err := currentClient(host)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	caller, err := currentUserInfo(client)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	output.Info("Pre-flight checks...")
	output.Success("Authenticated as %s (%s)", caller.Login, host)

	if host == config.DefaultHost {
		if err := checkImportScopes(client); err != nil {
			return err
		}
		scopes, _ := client.GetTokenScopes()
		output.Success("Token scopes: %s", strings.Join(scopes, ", "))
	}

	output.Info("\nFetching org structure...")

	allTeams, err := client.ListOrgTeams(importOrg)
	if err != nil {
		return fmt.Errorf("import: could not list teams for %q: %w", importOrg, err)
	}

	// Filter to a single team if --team is set.
	var teams []api.TeamInfo
	if importTeam != "" {
		for _, t := range allTeams {
			if t.Slug == importTeam {
				teams = []api.TeamInfo{t}
				break
			}
		}
		if len(teams) == 0 {
			return fmt.Errorf("import: team %q not found in org %q", importTeam, importOrg)
		}
	} else {
		teams = allTeams
	}

	output.Info("  Teams:   %d", len(teams))

	// Concurrent fetch: members + repos per team, bounded to 4 goroutines.
	results := make([]importResult, len(teams))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	ns := importOrg
	if importNamespace != "" {
		ns = importNamespace
	}

	for i, team := range teams {
		wg.Add(1)
		go func(i int, team api.TeamInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			name := collectionNameForTeam(team, importFlatten)

			members, err := client.ListTeamMembers(importOrg, team.Slug, "")
			if err != nil {
				results[i] = importResult{team: team, name: name, err: fmt.Errorf("team %q: list members: %w", team.Slug, err)}
				return
			}
			maintainers, err := client.ListTeamMembers(importOrg, team.Slug, "maintainer")
			if err != nil {
				results[i] = importResult{team: team, name: name, err: fmt.Errorf("team %q: list maintainers: %w", team.Slug, err)}
				return
			}
			repos, err := client.ListTeamRepos(importOrg, team.Slug)
			if err != nil {
				results[i] = importResult{team: team, name: name, err: fmt.Errorf("team %q: list repos: %w", team.Slug, err)}
				return
			}

			col, err := buildCollectionFromTeam(
				team, members, maintainers, repos,
				importOrg, name, ns, host,
				importOwnerFromMaintainer, caller.ID, caller.Login,
			)
			results[i] = importResult{team: team, name: name, col: col, err: err}
		}(i, team)
	}
	wg.Wait()

	// Count totals for summary.
	var totalRepos, totalMembers int
	memberSeen := make(map[string]bool)
	for _, r := range results {
		if r.col != nil {
			totalRepos += len(r.col.Repos)
			for _, m := range r.col.Members {
				memberSeen[m] = true
			}
			memberSeen[r.col.Owner] = true
		}
	}
	totalMembers = len(memberSeen)

	if importTeam == "" {
		output.Info("  Repos:   %d", totalRepos)
		output.Info("  Members: %d unique\n", totalMembers)
	}

	if importDryRun {
		output.Info("[dry-run] Would create %d collection(s):\n", len(teams))
		for _, r := range results {
			if r.err != nil {
				output.Warn("  %-30s ERROR: %v", r.name, r.err)
				continue
			}
			ownerLogin := r.col.Logins[r.col.Owner]
			fmt.Printf("  %-30s %d repos · %d members · owner: %s\n",
				r.name, len(r.col.Repos), len(r.col.Members), ownerLogin)
		}
		fmt.Println()
		output.Info("[dry-run] No files written.")
		output.Dim("Run without --dry-run to apply.")
		return nil
	}

	// Write collections, handling conflicts.
	if importTeam == "" {
		output.Info("Importing collections (%d):\n", len(teams))
	}

	var imported, skipped, failed int
	for idx, r := range results {
		if r.err != nil {
			output.Warn("[%d/%d]  %-30s ERROR: %v", idx+1, len(teams), r.name, r.err)
			failed++
			continue
		}

		ownerLogin := r.col.Logins[r.col.Owner]

		exists, err := collection.Exists(importOrg + "-" + r.name)
		if err != nil {
			output.Warn("[%d/%d]  %-30s could not check existence: %v", idx+1, len(teams), r.name, err)
			failed++
			continue
		}

		if exists {
			existing, loadErr := collection.Load(importOrg + "-" + r.name)
			if loadErr != nil {
				output.Warn("[%d/%d]  %-30s could not load existing: %v", idx+1, len(teams), r.name, loadErr)
				failed++
				continue
			}

			action := resolveConflict(r.name, existing, r.col)
			switch action {
			case "skip":
				output.Dim("[%d/%d]  %-30s skipped (already exists)", idx+1, len(teams), r.name)
				skipped++
				continue
			case "overwrite":
				r.col.SetPath(importOrg + "-" + r.name)
				if saveErr := r.col.Save(); saveErr != nil {
					output.Warn("[%d/%d]  %-30s overwrite failed: %v", idx+1, len(teams), r.name, saveErr)
					failed++
					continue
				}
			case "merge":
				mergeCollections(existing, r.col)
				if saveErr := existing.Save(); saveErr != nil {
					output.Warn("[%d/%d]  %-30s merge failed: %v", idx+1, len(teams), r.name, saveErr)
					failed++
					continue
				}
			}
		} else {
			if saveErr := r.col.Save(); saveErr != nil {
				output.Warn("[%d/%d]  %-30s save failed: %v", idx+1, len(teams), r.name, saveErr)
				failed++
				continue
			}
		}

		if len(teams) > 1 {
			fmt.Printf("  [%d/%d]  %-22s %d repos · %d members · owner: %s\n",
				idx+1, len(teams), r.name, len(r.col.Repos), len(r.col.Members), ownerLogin)
		}
		imported++

		recordAudit(audit.AuditEntry{
			Collection: r.name,
			Actor:      caller.Login,
			Action:     "import",
			Target:     importOrg + "/" + r.team.Slug,
			Detail:     fmt.Sprintf("Imported from %s/%s: %d repos, %d members", importOrg, r.team.Slug, len(r.col.Repos), len(r.col.Members)),
			Result:     "ok",
		})
	}

	fmt.Println()
	if importTeam != "" && imported == 1 {
		r := results[0]
		output.Success("Imported %s", r.name)
		ownerLogin := r.col.Logins[r.col.Owner]
		collDir, _ := config.CollectionsDir()
		fmt.Printf("  %d repos · %d members · owner: %s\n", len(r.col.Repos), len(r.col.Members), ownerLogin)
		fmt.Printf("  Written to %s/%s-%s.yaml\n", collDir, importOrg, r.name)
	} else {
		output.Success("Imported %d collection(s)", imported)
		if skipped > 0 {
			output.Dim("  %d skipped (already existed)", skipped)
		}
		if failed > 0 {
			output.Warn("%d team(s) failed — see warnings above", failed)
		}
		collDir, _ := config.CollectionsDir()
		fmt.Printf("  Collections written to %s\n", collDir)
		fmt.Printf("  %d unique members across all collections\n", totalMembers)
		fmt.Printf("  %d repos across all collections\n", totalRepos)
	}

	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  gitcollect list\n")
	if importTeam != "" {
		fmt.Printf("  gitcollect clone %s\n", results[0].name)
	} else if len(results) > 0 {
		fmt.Printf("  gitcollect clone %s\n", results[0].name)
	}
	fmt.Printf("  gitcollect publish --repo %s/gitcollect-config\n", importOrg)

	if failed > 0 {
		return fmt.Errorf("import: %d team(s) failed", failed)
	}
	return nil
}
