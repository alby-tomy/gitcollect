package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	findJSON   bool
	findMember string
)

type findMatch struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	Namespace  string `json:"namespace"`
}

type findResult struct {
	Repo    string      `json:"repo"`
	FoundIn []findMatch `json:"found_in"`
}

var findCmd = &cobra.Command{
	Use:   "find <repo>",
	Short: "Search all collections for a repo by name, or use --member to find by login",
	Args: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("member") {
			return cobra.NoArgs(cmd, args)
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	RunE: runFind,
}

type memberFindResult struct {
	Username  string      `json:"username"`
	FoundIn   []findMatch `json:"found_in"`
}

func runFind(cmd *cobra.Command, args []string) error {
	if findMember != "" {
		return runFindMember(findMember)
	}

	repoName := args[0]

	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}

	var matches []findMatch
	for _, name := range names {
		col, loadErr := collection.Load(name)
		if loadErr != nil {
			continue
		}
		for _, r := range col.Repos {
			if r.Name == repoName {
				matches = append(matches, findMatch{
					Name:       col.Name,
					Visibility: string(col.Visibility),
					Namespace:  col.RepoNamespace(),
				})
				break
			}
		}
	}

	if findJSON {
		fm := matches
		if fm == nil {
			fm = []findMatch{}
		}
		return output.JSON(findResult{Repo: repoName, FoundIn: fm})
	}

	if len(matches) == 0 {
		output.Info("%q not found in any collection", repoName)
		return fmt.Errorf("find: %q not found", repoName)
	}

	for _, m := range matches {
		output.Success("Found %q in %q (%s · %s)", repoName, m.Name, m.Namespace, m.Visibility)
	}
	return nil
}

// runFindMember searches every local collection for a member by login.
func runFindMember(login string) error {
	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}

	var matches []findMatch
	for _, name := range names {
		col, loadErr := collection.Load(name)
		if loadErr != nil {
			continue
		}
		// Check owner.
		if col.Logins[col.Owner] == login {
			matches = append(matches, findMatch{
				Name:       col.Name,
				Visibility: string(col.Visibility),
				Namespace:  col.RepoNamespace(),
			})
			continue
		}
		// Check members.
		for _, id := range col.Members {
			if col.Logins[id] == login {
				matches = append(matches, findMatch{
					Name:       col.Name,
					Visibility: string(col.Visibility),
					Namespace:  col.RepoNamespace(),
				})
				break
			}
		}
	}

	if findJSON {
		fm := matches
		if fm == nil {
			fm = []findMatch{}
		}
		return output.JSON(memberFindResult{Username: login, FoundIn: fm})
	}

	if len(matches) == 0 {
		output.Info("%q is not a member of any collection", login)
		return fmt.Errorf("find: %q not found in any collection", login)
	}
	for _, m := range matches {
		output.Success("Found member %q in %q (%s · %s)", login, m.Name, m.Namespace, m.Visibility)
	}
	return nil
}

func init() {
	findCmd.Flags().BoolVar(&findJSON, "json", false, "output result as JSON")
	findCmd.Flags().StringVar(&findMember, "member", "", "find all collections a given member login belongs to")
	rootCmd.AddCommand(findCmd)
}
