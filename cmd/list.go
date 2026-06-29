package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	listAll  bool
	listJSON bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List your collections (owned + member)",
	Long: `Lists collections you belong to, reading only local manifests under
~/.gitcollect/collections — no network calls are made.

Public collections and any collection you're explicitly listed as a member
of are shown by default. Pass --all to also include private collections
you own but haven't added yourself to as a member.`,
	Args: cobra.NoArgs,
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "include private collections you own even if you aren't a member")
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
}

func runList(cmd *cobra.Command, args []string) error {
	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

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

		if role == "owner" && col.Visibility == collection.VisibilityPrivate && !listAll {
			continue
		}

		rows = append(rows, listRow{
			Name:       col.Name,
			Host:       col.Host,
			Visibility: string(col.Visibility),
			Role:       role,
			Members:    len(col.Members),
			Repos:      len(col.Repos),
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
	return nil
}
