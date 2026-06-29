package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/output"
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
	listPrivate bool
	listPublic  bool
	listJSON    bool
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
	rootCmd.AddCommand(listCmd)
}

type listRow struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Visibility string `json:"visibility"`
	Role       string `json:"role"`
	Members    int    `json:"members"`
	Repos      int    `json:"repos"`
	StaleDays  int    `json:"stale_days,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
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

		username, err := config.LoadUser(col.Host)
		if err != nil {
			output.Warn("skipping %q: %v", name, err)
			continue
		}

		var role string
		switch {
		case username != "" && username == col.Owner:
			role = "owner"
		case username != "" && col.IsMember(username):
			role = "member"
		default:
			continue // not yours
		}

		if filterVisibility {
			if listPrivate && col.Visibility != collection.VisibilityPrivate {
				continue
			}
			if listPublic && col.Visibility != collection.VisibilityPublic {
				continue
			}
		}

		rows = append(rows, listRow{
			Name:       col.Name,
			Host:       col.Host,
			Visibility: string(col.Visibility),
			Role:       role,
			Members:    len(col.Members),
			Repos:      len(col.Repos),
			StaleDays:  staleDays(col.UpdatedAt),
		})
	}

	if listJSON {
		return output.JSON(rows)
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{r.Name, r.Visibility, r.Role, fmt.Sprintf("%d", r.Members), fmt.Sprintf("%d", r.Repos)})
	}
	output.Table([]string{"NAME", "VISIBILITY", "ROLE", "MEMBERS", "REPOS"}, tableRows)

	for _, r := range rows {
		if r.StaleDays > 0 {
			output.StaleWarning(r.Name, r.StaleDays)
		}
	}
	return nil
}
