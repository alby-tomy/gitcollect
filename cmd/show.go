package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

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
	Visibility  string              `json:"visibility"`
	Members     []string            `json:"members"`
	Groups      map[string][]string `json:"groups"`
	Repos       []showRepo          `json:"repos"`
}

type showRepo struct {
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
	Users  []string `json:"users"`
}

func runShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	col, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("show: %w", err)
	}

	if showJSON {
		return output.JSON(toShowOutput(col))
	}

	fmt.Printf("Collection:  %s\n", col.Name)
	if col.Description != "" {
		fmt.Printf("Description: %s\n", col.Description)
	}
	fmt.Printf("Host:        %s\n", col.Host)
	fmt.Printf("Owner:       %s\n", col.Owner)
	fmt.Printf("Visibility:  %s\n", col.Visibility)
	fmt.Printf("Members:     %d\n", len(col.Members))
	fmt.Printf("Groups:      %d\n", len(col.Groups))
	fmt.Printf("Repos:       %d\n", len(col.Repos))

	if len(col.Members) > 0 {
		rows := make([][]string, 0, len(col.Members))
		for _, m := range col.Members {
			rows = append(rows, []string{m})
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

	if len(col.Repos) > 0 {
		rows := make([][]string, 0, len(col.Repos))
		for _, r := range col.Repos {
			access := "open to all members"
			switch {
			case len(r.Groups) > 0 && len(r.Users) > 0:
				access = fmt.Sprintf("groups: %v, users: %v", r.Groups, r.Users)
			case len(r.Groups) > 0:
				access = fmt.Sprintf("groups: %v", r.Groups)
			case len(r.Users) > 0:
				access = fmt.Sprintf("users: %v", r.Users)
			}
			rows = append(rows, []string{r.Name, access})
		}
		fmt.Println()
		output.Table([]string{"REPO", "ACCESS"}, rows)
	}

	return nil
}

func toShowOutput(col *collection.Collection) showOutput {
	repos := make([]showRepo, 0, len(col.Repos))
	for _, r := range col.Repos {
		repos = append(repos, showRepo{Name: r.Name, Groups: r.Groups, Users: r.Users})
	}
	return showOutput{
		Name:        col.Name,
		Description: col.Description,
		Host:        col.Host,
		Owner:       col.Owner,
		Visibility:  string(col.Visibility),
		Members:     col.Members,
		Groups:      col.Groups,
		Repos:       repos,
	}
}
