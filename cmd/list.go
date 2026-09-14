package cmd

import (
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

// staleAfter is how long since a collection's UpdatedAt before list/show
// warn that the local manifest might no longer reflect the owner's latest
// changes. Informational only — never blocks anything.
const staleAfter = 30 * 24 * time.Hour

// staleDays returns the whole number of days since updatedAt, or 0 if
// that's under staleAfter (i.e. not stale).
func staleDays(updatedAt time.Time) int {
	age := time.Since(updatedAt)
	if age < staleAfter {
		return 0
	}
	return int(age.Hours() / 24)
}

var (
	listPrivate         bool
	listPublic          bool
	listJSON            bool
	listIncludeArchived bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List your collections (owned + member)",
	Long: `Lists every collection you own or are a member of, public or private,
reading only local manifests under ~/.gitcollect/collections — no network
calls are made.

Pass --private or --public to narrow the list to just one visibility.
Passing both is the same as passing neither (no filter).`,
	Args: cobra.NoArgs,
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listPrivate, "private", false, "show only private collections")
	listCmd.Flags().BoolVar(&listPublic, "public", false, "show only public collections")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "machine-readable output")
	listCmd.Flags().BoolVar(&listIncludeArchived, "include-archived", false, "include archived collections")
	rootCmd.AddCommand(listCmd)
}

type listRow struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Visibility  string `json:"visibility"`
	Role        string `json:"role"`
	Description string `json:"description"`
	Members     int    `json:"members"`
	Repos       int    `json:"repos"`
	Archived    bool   `json:"archived,omitempty"`
	StaleDays   int    `json:"stale_days,omitempty"`
}

// truncateDesc truncates s to 40 runes with a "..." suffix; returns s unchanged
// if it's 40 runes or fewer, and returns "" if s is empty.
func truncateDesc(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= 40 {
		return s
	}
	return string(runes[:40]) + "..."
}

func runList(cmd *cobra.Command, args []string) error {
	if IsOffline() {
		output.Info("offline mode: reading local manifests only")
	}

	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

	// Passing both --private and --public is the same as passing neither:
	// no filter, since a collection can't be excluded by both at once.
	filterVisibility := listPrivate != listPublic

	rows := make([]listRow, 0, len(names))
	for _, name := range names {
		col, err := collection.Load(name)
		if err != nil {
			output.Warn("skipping %q: %v", name, err)
			continue
		}

		if col.Archived && !listIncludeArchived {
			continue
		}

		role := roleFor(col)

		if filterVisibility {
			if listPrivate && col.Visibility != collection.VisibilityPrivate {
				continue
			}
			if listPublic && col.Visibility != collection.VisibilityPublic {
				continue
			}
		}

		rows = append(rows, listRow{
			Name:        col.Name,
			Host:        col.Host,
			Visibility:  string(col.Visibility),
			Role:        role,
			Description: truncateDesc(col.Description),
			Members:     len(col.Members),
			Repos:       len(col.Repos),
			Archived:    col.Archived,
			StaleDays:   staleDays(col.UpdatedAt),
		})
	}

	if listJSON {
		return output.JSON(rows)
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		desc := color.HiBlackString("(no description)")
		if r.Description != "" {
			desc = `"` + r.Description + `"`
		}
		tableRows = append(tableRows, []string{r.Name, r.Visibility, r.Role, desc, fmt.Sprintf("%d", r.Members), fmt.Sprintf("%d", r.Repos)})
	}
	output.Table([]string{"NAME", "VISIBILITY", "ROLE", "DESCRIPTION", "MEMBERS", "REPOS"}, tableRows)

	for _, r := range rows {
		if r.StaleDays > 0 {
			output.StaleWarning(r.Name, r.StaleDays)
		}
	}
	return nil
}

// roleFor determines the caller's role in col: "owner", "member", "public"
// for a public collection they are not listed in, or "—" when no identity
// is cached at all. It never touches the network, since
// list must stay usable offline across however many different hosts the
// caller's local collections span (its own doc comment promises "no
// network calls are made"). A col already on CurrentVersion compares the
// cached platform ID (config.LoadUserID, populated by auth); a col still
// on the legacy "1" format compares the cached login instead (config.
// LoadUser), exactly as before this migration — list never migrates a
// collection itself (see migrateIfNeeded's doc comment for why), so it has
// to keep working against both formats indefinitely. IsOwner/IsMember
// don't need to know which format they're being called with: as long as
// the caller passes the matching kind of value (ID for a CurrentVersion
// col, login for a "1" col), plain string equality is correct either way.
//
// Every local collection is listed. Rows used to be dropped whenever a
// role could not be determined, which meant list printed an empty table
// before "gitcollect auth" had ever run — right after pull-config or join
// fetched a team's collections, when a new joiner most needs to see them.
// The files are on the caller's own disk and show renders a public one
// without any identity at all, so hiding the row concealed nothing and
// only made the command look broken.
func roleFor(col *collection.Collection) string {
	identity := func() string {
		if col.Version == collection.CurrentVersion {
			id, err := config.LoadUserID(col.Host)
			if err != nil {
				return ""
			}
			return id
		}
		username, err := config.LoadUser(col.Host)
		if err != nil {
			return ""
		}
		return username
	}()

	if identity == "" {
		// No cached identity for this host yet.
		return "—"
	}
	switch {
	case col.IsOwner(identity):
		return "owner"
	case isListedMember(col, identity):
		return "member"
	case col.Visibility == collection.VisibilityPublic:
		return "public"
	default:
		return "—"
	}
}

// isListedMember reports whether identity appears in col.Members, without
// collection.IsMember's public-collection shortcut — list needs to tell a
// real member apart from someone merely admitted by public visibility.
func isListedMember(col *collection.Collection, identity string) bool {
	for _, m := range col.Members {
		if m == identity {
			return true
		}
	}
	return false
}
