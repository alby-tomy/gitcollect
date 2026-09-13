package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/git"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	pullConfigRepo       string
	pullConfigCollection string
	pullConfigBranch     string
	pullConfigPath       string
	pullConfigOverwrite  bool
)

var pullConfigCmd = &cobra.Command{
	Use:   "pull-config",
	Short: "Fetch collection files from a shared git repository",
	Long: `Clone or shallow-fetch a shared config repository and copy its collection
YAML files to ~/.gitcollect/collections/.

Run this after a team admin has published collections with:
  gitcollect publish --repo <org/repo>`,
	RunE: runPullConfig,
}

func init() {
	pullConfigCmd.Flags().StringVar(&pullConfigRepo, "repo", "", "shared config repo to fetch from, e.g. acme-corp/gitcollect-config (required)")
	pullConfigCmd.Flags().StringVar(&pullConfigCollection, "collection", "", "fetch only this collection (default: all)")
	pullConfigCmd.Flags().StringVar(&pullConfigBranch, "branch", "main", "branch to fetch from")
	pullConfigCmd.Flags().StringVar(&pullConfigPath, "path", "collections/", "directory in the repo that contains collection files")
	pullConfigCmd.Flags().BoolVar(&pullConfigOverwrite, "overwrite", false, "overwrite existing local collections")
	rootCmd.AddCommand(pullConfigCmd)
}

func runPullConfig(_ *cobra.Command, _ []string) error {
	if pullConfigRepo == "" {
		return NewUsageError(fmt.Errorf("pull-config: --repo is required (e.g. acme-corp/gitcollect-config)"))
	}
	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("pull-config: %w", err)
	}

	host := config.DefaultHost
	cloneURL := repoCloneURL(host, pullConfigRepo)

	output.Info("Fetching collections from %s...", pullConfigRepo)

	// Clone to a temp directory.
	tmpDir, err := os.MkdirTemp("", "gitcollect-pull-config-*")
	if err != nil {
		return fmt.Errorf("pull-config: could not create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := git.ShallowClone(cloneURL, tmpDir); err != nil {
		return fmt.Errorf("pull-config: could not clone %s: %w", cloneURL, err)
	}

	if pullConfigBranch != "main" && pullConfigBranch != "" {
		if err := git.Checkout(tmpDir, pullConfigBranch); err != nil {
			return fmt.Errorf("pull-config: could not checkout branch %q: %w", pullConfigBranch, err)
		}
	}

	srcDir := filepath.Join(tmpDir, filepath.FromSlash(pullConfigPath))
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pull-config: directory %q not found in %s", pullConfigPath, pullConfigRepo)
		}
		return fmt.Errorf("pull-config: could not read %s: %w", pullConfigPath, err)
	}

	collDir, err := config.CollectionsDir()
	if err != nil {
		return fmt.Errorf("pull-config: %w", err)
	}
	if err := config.EnsureDir(collDir); err != nil {
		return fmt.Errorf("pull-config: %w", err)
	}

	var fetched, skipped int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")

		// Filter to a single collection if --collection is set.
		if pullConfigCollection != "" && name != pullConfigCollection {
			continue
		}

		dst := filepath.Join(collDir, e.Name())

		// Check existence before writing — determines new vs. updated label.
		_, statErr := os.Stat(dst)
		existedBefore := statErr == nil
		if existedBefore && !pullConfigOverwrite {
			output.Dim("  %-30s (exists, skipped — use --overwrite to replace)", name)
			skipped++
			continue
		}

		src := filepath.Join(srcDir, e.Name())
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("pull-config: %w", err)
		}

		if existedBefore {
			output.Success("%-30s (updated)", name)
		} else {
			output.Success("%-30s (new)", name)
		}
		fetched++
	}

	if pullConfigCollection != "" && fetched == 0 {
		return fmt.Errorf("pull-config: collection %q not found in %s/%s", pullConfigCollection, pullConfigRepo, pullConfigPath)
	}

	fmt.Println()
	output.Success("Fetched %d collection(s)", fetched)
	if skipped > 0 {
		output.Dim("  %d skipped (already exist locally)", skipped)
	}

	fmt.Printf("\nYour collections are ready. Clone your team's repos:\n")
	fmt.Printf("  gitcollect list\n")

	client, err := currentClient(host)
	if err == nil {
		caller, _ := currentUser(client)
		recordAudit(audit.AuditEntry{
			Collection: pullConfigRepo,
			Actor:      caller,
			Action:     "pull-config",
			Target:     pullConfigRepo,
			Detail:     fmt.Sprintf("Fetched %d collection(s) from %s", fetched, pullConfigRepo),
			Result:     "ok",
		})
	}

	return nil
}
