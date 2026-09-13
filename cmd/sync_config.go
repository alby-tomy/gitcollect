package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	syncConfigAll    bool
	syncConfigFrom   string
	syncConfigOrg    string
	syncConfigDryRun bool
)

var syncConfigCmd = &cobra.Command{
	Use:   "sync-config [<collection>]",
	Short: "Refresh local collections from the platform org state",
	Long: `For each local collection that has a namespace set (imported from an org),
re-fetch the current team state from GitHub or GitLab, print what changed,
and update the local YAML with the current membership and repo list.

Run this when the org's team membership or repo list has changed and you
want to propagate those changes to the local collection files.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSyncConfig,
}

func init() {
	syncConfigCmd.Flags().BoolVar(&syncConfigAll, "all", false, "sync all collections (default when no collection name is given)")
	syncConfigCmd.Flags().StringVar(&syncConfigFrom, "from", "", "platform to sync from: github or gitlab (default: collection's host)")
	syncConfigCmd.Flags().StringVar(&syncConfigOrg, "org", "", "org to sync from (default: collection's namespace)")
	syncConfigCmd.Flags().BoolVar(&syncConfigDryRun, "dry-run", false, "show what would change without applying")
	rootCmd.AddCommand(syncConfigCmd)
}

// syncConfigResult carries the diff for one collection along with the
// platform data fetched to compute it, so callers can reuse it rather
// than issuing a second round of API calls to apply the sync.
type syncConfigResult struct {
	name            string
	addedMembers    []string
	removedMembers  []string
	addedRepos      []string
	removedRepos    []string
	platformMembers []api.UserInfo
	platformRepos   []api.RepoInfo
}

func (r syncConfigResult) hasChanges() bool {
	return len(r.addedMembers) > 0 || len(r.removedMembers) > 0 ||
		len(r.addedRepos) > 0 || len(r.removedRepos) > 0
}

// syncOneCollection re-fetches the platform state for col and returns a diff.
// It does NOT write anything to disk; the caller decides whether to apply.
func syncOneCollection(col *collection.Collection, client api.Client, org, teamSlug string) (syncConfigResult, error) {
	res := syncConfigResult{name: col.Name}

	allMembers, err := client.ListTeamMembers(org, teamSlug, "")
	if err != nil {
		return res, fmt.Errorf("list members: %w", err)
	}
	allRepos, err := client.ListTeamRepos(org, teamSlug)
	if err != nil {
		return res, fmt.Errorf("list repos: %w", err)
	}

	// Current state in the local collection.
	localMemberSet := make(map[string]bool, len(col.Members)+1)
	localMemberSet[col.Owner] = true
	for _, m := range col.Members {
		localMemberSet[m] = true
	}
	localRepoSet := make(map[string]bool, len(col.Repos))
	for _, r := range col.Repos {
		localRepoSet[r.Name] = true
	}

	// Platform state (by ID for members, by name for repos).
	platformMemberSet := make(map[string]bool, len(allMembers))
	for _, m := range allMembers {
		platformMemberSet[m.ID] = true
	}
	platformRepoSet := make(map[string]bool, len(allRepos))
	for _, r := range allRepos {
		platformRepoSet[r.Name] = true
	}

	// New members: on platform but not in local collection.
	for _, m := range allMembers {
		if !localMemberSet[m.ID] {
			login := m.Login
			if login == "" {
				login = m.ID
			}
			res.addedMembers = append(res.addedMembers, fmt.Sprintf("%s (id: %s)", login, m.ID))
		}
	}
	// Removed members: in local collection but not on platform.
	for _, id := range col.Members {
		if !platformMemberSet[id] {
			login := col.Logins[id]
			if login == "" {
				login = id
			}
			res.removedMembers = append(res.removedMembers, login)
		}
	}

	// New repos: on platform but not in local collection.
	for _, r := range allRepos {
		if !localRepoSet[r.Name] {
			res.addedRepos = append(res.addedRepos, r.Name)
		}
	}
	// Removed repos: in local collection but not on platform.
	for _, r := range col.Repos {
		if !platformRepoSet[r.Name] {
			res.removedRepos = append(res.removedRepos, r.Name)
		}
	}

	// Stash the fetched platform data so callers can apply the sync
	// without a second round of API calls.
	res.platformMembers = allMembers
	res.platformRepos = allRepos

	return res, nil
}

// applySync updates col in-place to match the platform state fetched by
// syncOneCollection, without discarding anything the platform does not
// know about.
//
// That last part is the whole point. This function previously rebuilt
// col.Repos from the platform list with empty Groups and Users on every
// entry, which silently reset every repo to "open to all members" — so a
// routine refresh destroyed exactly the per-repo access control the
// collection exists to express. The platform knows which repos a team can
// reach; it knows nothing about gitcollect's groups, so it can only ever
// add and remove repos, never restate their access rules.
//
// Membership is authoritative from the platform, so departing members are
// dropped. Their references elsewhere in the manifest have to go with
// them: Validate requires every group member, group admin and per-repo
// grantee to appear in Members, so leaving a stale reference behind would
// make the very next Save fail.
func applySync(col *collection.Collection, platformMembers []api.UserInfo, platformRepos []api.RepoInfo) {
	// Members: the platform list is authoritative. The owner is tracked
	// separately from Members here, matching how import builds a
	// collection in the first place.
	memberIDs := make([]string, 0, len(platformMembers))
	retained := make(map[string]bool, len(platformMembers))
	for _, m := range platformMembers {
		if m.ID == col.Owner {
			continue
		}
		memberIDs = append(memberIDs, m.ID)
		retained[m.ID] = true
	}
	col.Members = memberIDs

	// Merge logins cache — additive, so an ID that left the team keeps a
	// readable name in any audit output that still references it.
	if col.Logins == nil {
		col.Logins = make(map[string]string)
	}
	for _, m := range platformMembers {
		if m.Login != "" {
			col.Logins[m.ID] = m.Login
		}
	}

	// Repos: add what is new, drop what is gone, and carry the existing
	// access rule across for everything that survives.
	previous := make(map[string]collection.RepoAccess, len(col.Repos))
	for _, r := range col.Repos {
		previous[r.Name] = r
	}
	repos := make([]collection.RepoAccess, 0, len(platformRepos))
	for _, pr := range platformRepos {
		if prior, ok := previous[pr.Name]; ok {
			repos = append(repos, prior)
			continue
		}
		repos = append(repos, collection.RepoAccess{
			Name:   pr.Name,
			Groups: []string{},
			Users:  []string{},
		})
	}
	col.Repos = repos

	pruneDepartedMembers(col, retained)
}

// pruneDepartedMembers removes every reference to an ID that is no longer
// in retained from the collection's groups, group admins and per-repo
// user grants. Groups themselves are kept even when they empty out — a
// group is a deliberate structure the owner created, not a side effect of
// who currently happens to be in it.
func pruneDepartedMembers(col *collection.Collection, retained map[string]bool) {
	filter := func(ids []string) []string {
		kept := make([]string, 0, len(ids))
		for _, id := range ids {
			if retained[id] {
				kept = append(kept, id)
			}
		}
		return kept
	}

	for group, ids := range col.Groups {
		col.Groups[group] = filter(ids)
	}
	for group, ids := range col.GroupAdmins {
		if admins := filter(ids); len(admins) > 0 {
			col.GroupAdmins[group] = admins
		} else {
			delete(col.GroupAdmins, group)
		}
	}
	for i := range col.Repos {
		col.Repos[i].Users = filter(col.Repos[i].Users)
	}
}

func runSyncConfig(_ *cobra.Command, args []string) error {
	// Gather collection names to sync.
	var names []string
	if len(args) == 1 {
		names = []string{args[0]}
	} else {
		listed, err := collection.List()
		if err != nil {
			return fmt.Errorf("sync-config: %w", err)
		}
		names = listed
	}

	if len(names) == 0 {
		return fmt.Errorf("sync-config: no collections found")
	}

	var anyFailed bool
	for _, name := range names {
		col, err := loadCollection(name)
		if err != nil {
			output.Warn("sync-config: could not load %q: %v", name, err)
			anyFailed = true
			continue
		}

		org := syncConfigOrg
		if org == "" {
			org = col.Namespace
		}
		if org == "" {
			output.Warn("sync-config: %q has no namespace — cannot determine org. Set namespace with: gitcollect init --namespace <org>", name)
			anyFailed = true
			continue
		}

		host := syncConfigFrom
		if host == "" {
			host = col.Host
		}
		if host == "" {
			host = config.DefaultHost
		}
		if host == "github" {
			host = "github.com"
		}
		if host == "gitlab" {
			host = "gitlab.com"
		}

		client, err := currentClient(host)
		if err != nil {
			output.Warn("sync-config: %q: could not get client: %v", name, err)
			anyFailed = true
			continue
		}
		caller, _ := currentUserInfo(client)

		// Derive team slug from the collection name: if it's "org-teamslug",
		// strip the "org-" prefix; otherwise use the name as-is.
		teamSlug := name
		if org != "" && len(name) > len(org)+1 && name[:len(org)+1] == org+"-" {
			teamSlug = name[len(org)+1:]
		}

		output.Info("Syncing %s from %s/%s...\n", name, host, org)

		diff, err := syncOneCollection(col, client, org, teamSlug)
		if err != nil {
			output.Warn("sync-config: %q: %v", name, err)
			anyFailed = true
			continue
		}

		if !diff.hasChanges() {
			output.Success("%s is up to date (last synced: %s)", name, humanDuration(time.Since(col.UpdatedAt)))
			continue
		}

		fmt.Printf("Changes detected:\n")
		for _, m := range diff.addedMembers {
			fmt.Printf("  + new member: %s\n", m)
		}
		for _, m := range diff.removedMembers {
			fmt.Printf("  - removed member: %s (no longer in platform team)\n", m)
		}
		for _, r := range diff.addedRepos {
			fmt.Printf("  + new repo:   %s\n", r)
		}
		for _, r := range diff.removedRepos {
			fmt.Printf("  - removed repo: %s (no longer in platform team)\n", r)
		}

		if syncConfigDryRun {
			output.Dim("[dry-run] No changes applied.")
			continue
		}

		applySync(col, diff.platformMembers, diff.platformRepos)

		if err := col.Save(); err != nil {
			output.Warn("sync-config: %q: could not save: %v", name, err)
			anyFailed = true
			continue
		}

		fmt.Printf("Applying changes...\n")
		output.Success("%s synced", name)
		fmt.Printf("\nRun: gitcollect sync %s   to clone new repos and pull existing\n", name)

		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      caller.Login,
			Action:     "sync-config",
			Target:     org + "/" + teamSlug,
			Detail: fmt.Sprintf("+%d members, -%d members, +%d repos, -%d repos",
				len(diff.addedMembers), len(diff.removedMembers),
				len(diff.addedRepos), len(diff.removedRepos)),
			Result: "ok",
		})
	}

	if anyFailed {
		return errors.New("sync-config: one or more collections could not be synced")
	}
	return nil
}

// humanDuration formats a duration as a short human-readable string.
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
