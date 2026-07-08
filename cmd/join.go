package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/git"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	joinOrg   string
	joinTeam  string
	joinRepo  string
	joinClone bool
	joinDest  string
	joinFrom  string
)

var joinCmd = &cobra.Command{
	Use:   "join",
	Short: "New-hire onboarding: fetch your team's config and clone repos in one step",
	Long: `Fetch a team's gitcollect collection from a shared config repo (or directly
from GitHub), then optionally clone every repo you have access to.

This is the one-command setup for someone joining an organisation:

  gitcollect join --org acme-corp --team payments-team --clone`,
	RunE: runJoin,
}

func init() {
	joinCmd.Flags().StringVar(&joinOrg, "org", "", "GitHub org or GitLab group (required)")
	joinCmd.Flags().StringVar(&joinTeam, "team", "", "your team slug (required)")
	joinCmd.Flags().StringVar(&joinRepo, "repo", "", "shared config repo to fetch from (e.g. acme-corp/gitcollect-config); if not set, imports directly from the platform API")
	joinCmd.Flags().BoolVar(&joinClone, "clone", false, "clone repos immediately after joining")
	joinCmd.Flags().StringVar(&joinDest, "dest", ".", "clone destination directory (used with --clone)")
	joinCmd.Flags().StringVar(&joinFrom, "from", "github", "platform to import from when --repo is not set: github or gitlab")
	rootCmd.AddCommand(joinCmd)
}

func runJoin(_ *cobra.Command, _ []string) error {
	if joinOrg == "" {
		return NewUsageError(fmt.Errorf("join: --org is required"))
	}
	if joinTeam == "" {
		return NewUsageError(fmt.Errorf("join: --team is required"))
	}

	host := config.DefaultHost
	if joinFrom == "gitlab" {
		host = "gitlab.com"
	}

	client, err := currentClient(host)
	if err != nil {
		return fmt.Errorf("join: %w", err)
	}
	caller, err := currentUserInfo(client)
	if err != nil {
		return fmt.Errorf("join: %w", err)
	}

	output.Info("Setting up gitcollect for %s/%s...\n", joinOrg, joinTeam)
	output.Success("Authenticated as %s (%s)", caller.Login, host)

	collectionName := joinOrg + "-" + joinTeam

	output.Info("\nFetching %s configuration...", joinTeam)

	if joinRepo != "" {
		// Fetch from shared config repo via pull-config flow.
		if err := git.CheckInstalled(); err != nil {
			return fmt.Errorf("join: %w", err)
		}
		cloneURL := repoCloneURL(host, joinRepo)
		tmpDir, removeTemp, err := makeTempDir("gitcollect-join-*")
		if err != nil {
			return fmt.Errorf("join: %w", err)
		}
		defer removeTemp()

		if err := git.ShallowClone(cloneURL, tmpDir); err != nil {
			return fmt.Errorf("join: could not clone %s: %w", joinRepo, err)
		}

		srcFile := filepath.Join(tmpDir, "collections", collectionName+".yaml")
		collDir, err := config.CollectionsDir()
		if err != nil {
			return fmt.Errorf("join: %w", err)
		}
		if err := config.EnsureDir(collDir); err != nil {
			return fmt.Errorf("join: %w", err)
		}
		dst := filepath.Join(collDir, collectionName+".yaml")
		if err := copyFile(srcFile, dst); err != nil {
			return fmt.Errorf("join: collection %q not found in %s/collections: %w", collectionName, joinRepo, err)
		}
	} else {
		// Import directly from the platform API.
		importFrom = joinFrom
		importOrg = joinOrg
		importTeam = joinTeam
		importDryRun = false
		importFlatten = true
		importOwnerFromMaintainer = true
		importNamespace = joinOrg
		importMerge = false
		importOverwrite = false
		importSkipExisting = false

		if err := runImport(nil, nil); err != nil {
			return fmt.Errorf("join: %w", err)
		}
	}

	col, err := collection.Load(collectionName)
	if err != nil {
		return fmt.Errorf("join: could not load collection %q: %w", collectionName, err)
	}

	output.Success("%s fetched (%d repos · %d members)", joinTeam, len(col.Repos), len(col.Members))
	collDir, _ := config.CollectionsDir()
	output.Dim("  Written to %s/%s.yaml", collDir, collectionName)

	recordAudit(audit.AuditEntry{
		Collection: collectionName,
		Actor:      caller.Login,
		Action:     "join",
		Target:     joinOrg + "/" + joinTeam,
		Detail:     fmt.Sprintf("Joined %s/%s via %s", joinOrg, joinTeam, func() string {
			if joinRepo != "" {
				return joinRepo
			}
			return host + " API"
		}()),
		Result: "ok",
	})

	if !joinClone {
		fmt.Printf("\nWelcome to the team. Next steps:\n")
		fmt.Printf("  gitcollect show %s    see your full repo access\n", collectionName)
		fmt.Printf("  gitcollect clone %s   clone your repos\n", collectionName)
		return nil
	}

	// Clone repos.
	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("join: %w", err)
	}
	accessible, err := access.FilterAccessible(col, caller.ID, client)
	if err != nil {
		return fmt.Errorf("join: %w", err)
	}

	fmt.Printf("\nCloning repos...\n")
	ns := col.RepoNamespace()
	cloned := 0
	for i, r := range accessible {
		info, err := client.GetRepo(ns, r.Name)
		if err != nil {
			output.Warn("  could not get %s: %v", r.Name, err)
			continue
		}
		destDir := filepath.Join(joinDest, r.Name)
		output.Progress(i+1, len(accessible), "Cloning "+r.Name+"...")
		if err := git.Clone(info.CloneURL, destDir); err != nil {
			output.Warn("  could not clone %s: %v", r.Name, err)
			continue
		}
		cloned++
	}

	fmt.Printf("\n")
	output.Success("Joined %s/%s", joinOrg, joinTeam)
	fmt.Printf("  %d repos cloned to %s\n", cloned, joinDest)

	fmt.Printf("\nWelcome to the team. Next steps:\n")
	fmt.Printf("  gitcollect show %s    see your full repo access\n", collectionName)
	fmt.Printf("  gitcollect pull %s    pull updates any time\n", collectionName)

	return nil
}

// makeTempDir creates a temporary directory and returns its path along with a
// cleanup function that removes it. Callers must defer the cleanup.
func makeTempDir(pattern string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", pattern)
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}
