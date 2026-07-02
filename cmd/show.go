package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var showJSON bool

var showCmd = &cobra.Command{
	Use:   "show <collection>",
	Short: "Show a summary of a collection: repos, members, and groups",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(showCmd)
}

type showOutput struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Host        string              `json:"host"`
	Owner       string              `json:"owner"`
	Namespace   string              `json:"namespace,omitempty"`
	Visibility  string              `json:"visibility"`
	Members     []string            `json:"members"`
	Groups      map[string][]string `json:"groups"`
	Repos       []showRepo          `json:"repos"`
	StaleDays   int                 `json:"stale_days,omitempty"`
}

type showRepo struct {
	Name         string   `json:"name"`
	Groups       []string `json:"groups"`
	Users        []string `json:"users"`
	YouCanAccess bool     `json:"you_can_access"`
	YouReason    string   `json:"you_reason"`
	YouFixCmd    string   `json:"you_fix_cmd,omitempty"`
	WhoHasAccess []string `json:"who_has_access"`
}

func runShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, caller, callerID, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("show: %w", err)
	}

	if showJSON {
		return output.JSON(toShowOutput(col, caller, callerID))
	}

	if days := staleDays(col.UpdatedAt); days > 0 {
		output.StaleWarning(col.Name, days)
	}

	isOwner := callerID != "" && col.IsOwner(callerID)
	ownerLogin := col.Logins[col.Owner]

	fmt.Printf("Collection:  %s\n", col.Name)
	if col.Description != "" {
		fmt.Printf("Description: %s\n", col.Description)
	}
	fmt.Printf("Host:        %s\n", col.Host)
	if isOwner {
		fmt.Printf("Owner:       %s (you)\n", ownerLogin)
	} else {
		fmt.Printf("Owner:       %s\n", ownerLogin)
	}
	if col.Namespace != "" {
		fmt.Printf("Namespace:   %s\n", col.Namespace)
	}
	fmt.Printf("Visibility:  %s\n", col.Visibility)
	fmt.Printf("Members:     %d\n", len(col.Members))
	fmt.Printf("Groups:      %d\n", len(col.Groups))
	fmt.Printf("Repos:       %d\n", len(col.Repos))

	if len(col.Members) > 0 {
		rows := make([][]string, 0, len(col.Members))
		for _, login := range loginsFor(col, col.Members) {
			rows = append(rows, []string{login})
		}
		fmt.Println()
		output.Table([]string{"MEMBER"}, rows)
	}

	if len(col.Groups) > 0 {
		rows := make([][]string, 0, len(col.Groups))
		for group, users := range col.Groups {
			rows = append(rows, []string{group, fmt.Sprintf("%d", len(users))})
		}
		fmt.Println()
		output.Table([]string{"GROUP", "MEMBERS"}, rows)
	}

	if len(col.Repos) == 0 {
		return nil
	}

	fmt.Println()
	if isOwner {
		// Owners always pass CanAccessRepo, so a YOU column would just be
		// "✓ yes" on every row — useless. WHO HAS ACCESS is the
		// information an owner actually wants: who can reach each repo.
		output.Table([]string{"REPO", "ACCESS RULE", "WHO HAS ACCESS"}, buildOwnerShowRepoRows(col))
		return nil
	}

	details := access.UserAccessMap(col, callerID, caller)
	rows, denied := buildShowRepoRows(col, col.Repos, details)
	output.Table([]string{"REPO", "ACCESS RULE", "YOU"}, rows)

	if len(denied) > 0 {
		fmt.Println()
		fmt.Printf("  You can't access %d repo(s):\n", len(denied))
		for _, d := range denied {
			fmt.Printf("    %-20s (%s) → %s\n", d.repo, d.reason, d.fixCmd)
		}
		output.Suggestion(fmt.Sprintf("gitcollect inspect %s --user %s", name, caller))
	}

	return nil
}

// describeAccessRule renders r's access rule for display. r.Users holds
// platform IDs (see collection.RepoAccess's doc comment), so col is
// needed to resolve them to logins before printing.
func describeAccessRule(col *collection.Collection, r collection.RepoAccess) string {
	switch {
	case len(r.Groups) > 0 && len(r.Users) > 0:
		return fmt.Sprintf("groups: %v, users: %v", r.Groups, loginsFor(col, r.Users))
	case len(r.Groups) > 0:
		return fmt.Sprintf("groups: %v", r.Groups)
	case len(r.Users) > 0:
		return fmt.Sprintf("users: %v", loginsFor(col, r.Users))
	default:
		return "open to all members"
	}
}

type deniedRepo struct {
	repo   string
	reason string
	fixCmd string
}

// buildShowRepoRows pairs repos (in collection order) with the calling
// user's per-repo access.RepoAccessDetail (same order, same length — both
// come from iterating the same col.Repos slice) into REPO/ACCESS RULE/YOU
// table rows, and separately collects the repos the caller is denied
// (with reason and exact fix command) for the footer below the table.
func buildShowRepoRows(col *collection.Collection, repos []collection.RepoAccess, details []access.RepoAccessDetail) (rows [][]string, denied []deniedRepo) {
	rows = make([][]string, 0, len(repos))
	for i, r := range repos {
		you := "✓ yes"
		if i < len(details) && !details[i].CanAccess {
			you = "✗ no — " + details[i].Reason
			denied = append(denied, deniedRepo{repo: r.Name, reason: details[i].Reason, fixCmd: details[i].FixCmd})
		}
		rows = append(rows, []string{r.Name, describeAccessRule(col, r), you})
	}
	return rows, denied
}

// buildOwnerShowRepoRows builds the owner-only REPO/ACCESS RULE/WHO HAS
// ACCESS rows: for each repo, every member who can reach it, joined with a
// trailing count.
func buildOwnerShowRepoRows(col *collection.Collection) [][]string {
	rows := make([][]string, 0, len(col.Repos))
	for _, r := range col.Repos {
		var who []string
		for _, m := range access.RepoAccessMap(col, r.Name) {
			if m.CanAccess {
				who = append(who, m.Username)
			}
		}
		whoCol := "—"
		if len(who) > 0 {
			whoCol = fmt.Sprintf("%s (%d)", strings.Join(who, ", "), len(who))
		}
		rows = append(rows, []string{r.Name, describeAccessRule(col, r), whoCol})
	}
	return rows
}

// toShowOutput builds show --json's payload for caller (their login) /
// callerID (their platform ID). Every ID-based field on col (Owner,
// Members, Groups' member lists, each repo's Users) is resolved to logins
// here — the JSON output is user-facing the same way the table is, so it
// stays login-based rather than exposing raw platform IDs.
func toShowOutput(col *collection.Collection, caller, callerID string) showOutput {
	details := access.UserAccessMap(col, callerID, caller)

	groupLogins := make(map[string][]string, len(col.Groups))
	for group, ids := range col.Groups {
		groupLogins[group] = loginsFor(col, ids)
	}

	repos := make([]showRepo, 0, len(col.Repos))
	for i, r := range col.Repos {
		repo := showRepo{Name: r.Name, Groups: r.Groups, Users: loginsFor(col, r.Users)}
		if i < len(details) {
			repo.YouCanAccess = details[i].CanAccess
			repo.YouReason = details[i].Reason
			repo.YouFixCmd = details[i].FixCmd
		}
		for _, m := range access.RepoAccessMap(col, r.Name) {
			if m.CanAccess {
				repo.WhoHasAccess = append(repo.WhoHasAccess, m.Username)
			}
		}
		repos = append(repos, repo)
	}
	return showOutput{
		Name:        col.Name,
		Description: col.Description,
		Host:        col.Host,
		Owner:       col.Logins[col.Owner],
		Namespace:   col.Namespace,
		Visibility:  string(col.Visibility),
		Members:     loginsFor(col, col.Members),
		Groups:      groupLogins,
		Repos:       repos,
		StaleDays:   staleDays(col.UpdatedAt),
	}
}
