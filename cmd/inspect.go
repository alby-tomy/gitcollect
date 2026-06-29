package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	inspectUser string
	inspectRepo string
	inspectJSON bool
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <collection>",
	Short: "Show access decisions for a user, a repo, or the full collection matrix",
	Args:  cobra.ExactArgs(1),
	RunE:  runInspect,
}

func init() {
	inspectCmd.Flags().StringVar(&inspectUser, "user", "", "show the full access map for this user")
	inspectCmd.Flags().StringVar(&inspectRepo, "repo", "", "show who can access this repo and why")
	inspectCmd.Flags().BoolVar(&inspectJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	name := args[0]

	if inspectUser != "" && inspectRepo != "" {
		return NewUsageError(fmt.Errorf("inspect: --user and --repo cannot be combined"))
	}

	col, _, err := loadForRead(name)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}

	switch {
	case inspectUser != "":
		return inspectByUser(col, inspectUser)
	case inspectRepo != "":
		return inspectByRepo(col, inspectRepo)
	default:
		return inspectMatrix(col)
	}
}

type inspectUserOutput struct {
	User       string                    `json:"user"`
	Collection string                    `json:"collection"`
	Visibility string                    `json:"visibility"`
	Member     bool                      `json:"member"`
	Groups     []string                  `json:"groups"`
	Repos      []access.RepoAccessDetail `json:"repos"`
}

func inspectByUser(col *collection.Collection, username string) error {
	details := access.UserAccessMap(col, username)

	if inspectJSON {
		return output.JSON(inspectUserOutput{
			User:       username,
			Collection: col.Name,
			Visibility: string(col.Visibility),
			Member:     col.IsMember(username),
			Groups:     groupsForMember(col, username),
			Repos:      details,
		})
	}

	member := "no"
	if col.IsMember(username) {
		member = "yes"
	}
	groups := strings.Join(groupsForMember(col, username), ", ")
	if groups == "" {
		groups = "—"
	}

	fmt.Printf("User:        %s\n", username)
	fmt.Printf("Collection:  %s (%s)\n", col.Name, col.Visibility)
	fmt.Printf("Member:      %s\n", member)
	fmt.Printf("Groups:      %s\n", groups)
	fmt.Println()

	rows := make([][]string, 0, len(details))
	for _, d := range details {
		mark := "✗ no"
		if d.CanAccess {
			mark = "✓ yes"
		}
		rows = append(rows, []string{d.RepoName, mark, d.Reason})
	}
	output.Table([]string{"REPO", "ACCESS", "REASON"}, rows)
	return nil
}

type inspectRepoOutput struct {
	Repo       string                      `json:"repo"`
	Collection string                      `json:"collection"`
	Groups     []string                    `json:"groups"`
	Users      []string                    `json:"users"`
	Members    []access.MemberAccessDetail `json:"members"`
}

func inspectByRepo(col *collection.Collection, repoName string) error {
	repo, found := repoAccessOf(col, repoName)
	if !found {
		return fmt.Errorf("inspect: %q is not in collection %q", repoName, col.Name)
	}

	details := access.RepoAccessMap(col, repoName)

	if inspectJSON {
		return output.JSON(inspectRepoOutput{
			Repo:       repoName,
			Collection: col.Name,
			Groups:     repo.Groups,
			Users:      repo.Users,
			Members:    details,
		})
	}

	fmt.Printf("Repo:       %s\n", repoName)
	fmt.Printf("Access:     %s\n", describeAccess(repo))
	fmt.Println()

	rows := make([][]string, 0, len(details))
	for _, d := range details {
		mark := "✗ no"
		if d.CanAccess {
			mark = "✓ yes"
		}
		rows = append(rows, []string{d.Username, mark, d.Reason})
	}
	output.Table([]string{"MEMBER", "ACCESS", "REASON"}, rows)
	return nil
}

type inspectMatrixOutput struct {
	Collection string             `json:"collection"`
	Visibility string             `json:"visibility"`
	Matrix     access.AccessMatrix `json:"matrix"`
}

func inspectMatrix(col *collection.Collection) error {
	matrix := access.FullMatrix(col)

	if inspectJSON {
		return output.JSON(inspectMatrixOutput{
			Collection: col.Name,
			Visibility: string(col.Visibility),
			Matrix:     matrix,
		})
	}

	fmt.Printf("Collection:  %s\n", col.Name)
	fmt.Printf("Visibility:  %s\n", col.Visibility)
	fmt.Printf("Members:     %d\n", len(matrix.Members))
	fmt.Println()

	if len(matrix.Members) == 0 || len(matrix.Repos) == 0 {
		fmt.Println("No members or no repos to show in the access matrix.")
		return nil
	}

	headers := append([]string{"MEMBER"}, matrix.Repos...)
	rows := make([][]string, 0, len(matrix.Members))
	for i, member := range matrix.Members {
		row := make([]string, 0, len(matrix.Repos)+1)
		row = append(row, member)
		for j := range matrix.Repos {
			if matrix.Grid[i][j] {
				row = append(row, "✓")
			} else {
				row = append(row, "✗")
			}
		}
		rows = append(rows, row)
	}
	output.Table(headers, rows)
	return nil
}
