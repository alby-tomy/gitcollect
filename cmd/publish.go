package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/git"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

// auditLogPublish is the audit log name used for publish and pull-config.
// Both act on a repository spec ("owner/name"), not a collection, and
// audit.Append builds its file path from the Collection field — so the
// raw spec would write to ~/.gitcollect/audit/owner/name.log, a directory
// that does not exist.
const auditLogPublish = "config-repo"

var (
	publishRepo       string
	publishCollection string
	publishBranch     string
	publishPath       string
	publishMessage    string
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Push collection files to a shared git repository",
	Long: `Copy gitcollect collection YAML files into a shared git repository so team
members can fetch them with: gitcollect pull-config --repo <org/repo>.

The target repository must already exist. It can be private — team members
only need read access to fetch from it.`,
	RunE: runPublish,
}

func init() {
	publishCmd.Flags().StringVar(&publishRepo, "repo", "", "GitHub/GitLab repo to publish to, e.g. acme-corp/gitcollect-config (required)")
	publishCmd.Flags().StringVar(&publishCollection, "collection", "", "publish only this collection (default: all)")
	publishCmd.Flags().StringVar(&publishBranch, "branch", "main", "branch to push to")
	publishCmd.Flags().StringVar(&publishPath, "path", "collections/", "directory in the repo to write collection files to")
	publishCmd.Flags().StringVar(&publishMessage, "message", "update gitcollect collections", "commit message")
	rootCmd.AddCommand(publishCmd)
}

// repoCloneURL builds an HTTPS clone URL from an "owner/repo" spec and host.
// host must be "github.com" or "gitlab.com".
func repoCloneURL(host, ownerSlashRepo string) string {
	return fmt.Sprintf("https://%s/%s.git", host, ownerSlashRepo)
}

func runPublish(_ *cobra.Command, _ []string) error {
	if publishRepo == "" {
		return NewUsageError(fmt.Errorf("publish: --repo is required (e.g. acme-corp/gitcollect-config)"))
	}
	if err := git.CheckInstalled(); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	// Determine which collections to publish.
	collDir, err := config.CollectionsDir()
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	var filesToPublish []string // absolute paths to .yaml files
	if publishCollection != "" {
		path := filepath.Join(collDir, publishCollection+".yaml")
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("publish: collection %q not found", publishCollection)
			}
			return fmt.Errorf("publish: %w", err)
		}
		filesToPublish = []string{path}
	} else {
		names, err := collection.List()
		if err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		if len(names) == 0 {
			return fmt.Errorf("publish: no collections found in %s", collDir)
		}
		for _, name := range names {
			filesToPublish = append(filesToPublish, filepath.Join(collDir, name+".yaml"))
		}
	}

	host, err := publishHost(filesToPublish)
	if err != nil {
		return err
	}
	cloneURL := repoCloneURL(host, publishRepo)
	token := gitTokenFor(host)

	output.Info("Publishing %d collection(s) to %s...", len(filesToPublish), publishRepo)

	// Clone to a temp directory.
	tmpDir, err := os.MkdirTemp("", "gitcollect-publish-*")
	if err != nil {
		return fmt.Errorf("publish: could not create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Select the branch during the clone. --depth=1 only fetches the
	// branch it clones, so the previous clone-then-checkout sequence could
	// never reach any branch but the remote's default — making --branch
	// fail on, for example, a repo whose default is master.
	if err := git.ShallowCloneBranch(cloneURL, tmpDir, publishBranch, token); err != nil {
		return fmt.Errorf("publish: could not clone %s (branch %q): %w", cloneURL, publishBranch, err)
	}

	// Ensure the target path inside the clone exists.
	destDir := filepath.Join(tmpDir, filepath.FromSlash(publishPath))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("publish: could not create %s in clone: %w", publishPath, err)
	}

	// Copy collection files into the clone.
	for _, src := range filesToPublish {
		name := filepath.Base(src)
		dst := filepath.Join(destDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		output.Success("%s", strings.TrimSuffix(name, ".yaml"))
	}

	// Stage, commit, push.
	if err := git.Add(tmpDir, filepath.FromSlash(publishPath)); err != nil {
		return fmt.Errorf("publish: git add: %w", err)
	}
	if err := git.Commit(tmpDir, publishMessage); err != nil {
		return fmt.Errorf("publish: git commit: %w", err)
	}
	if err := git.PushWithToken(tmpDir, token); err != nil {
		return fmt.Errorf("publish: git push: %w", err)
	}

	fmt.Println()
	output.Success("Published %d collection(s) to %s (%s)", len(filesToPublish), publishRepo, publishBranch)
	output.Dim("  Commit: %q", publishMessage)
	fmt.Printf("\nShare this with your team:\n")
	fmt.Printf("  gitcollect pull-config --repo %s\n", publishRepo)

	client, err := currentClient(host)
	if err == nil {
		caller, _ := currentUser(client)
		recordAudit(audit.AuditEntry{
			// Not publishRepo: it is "owner/name", and the audit log path
			// is built from this field, so a slash would send the write to
			// a directory that does not exist.
			Collection: auditLogPublish,
			Actor:      caller,
			Action:     "publish",
			Target:     publishRepo,
			Detail:     fmt.Sprintf("Published %d collection(s) to %s branch %s", len(filesToPublish), publishRepo, publishBranch),
			Result:     "ok",
		})
	}

	return nil
}

// copyFile copies src to dst, creating dst if it doesn't exist.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copyFile: open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("copyFile: create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copyFile: copy %s → %s: %w", src, dst, err)
	}
	return nil
}

// publishHost decides which platform the config repository lives on.
//
// It used to be hardcoded to github.com behind a comment claiming the
// value came from a GITCOLLECT_HOST environment variable that was never
// read, so GitLab users could not publish at all. The collections being
// published already record their host, so use that: unambiguous when they
// agree, and an explicit ask when they do not.
func publishHost(files []string) (string, error) {
	hosts := map[string]bool{}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		col, err := collection.Load(name)
		if err != nil {
			continue
		}
		if col.Host != "" {
			hosts[col.Host] = true
		}
	}

	switch len(hosts) {
	case 0:
		return config.DefaultHost, nil
	case 1:
		for h := range hosts {
			return h, nil
		}
	}

	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	return "", fmt.Errorf(
		"publish: collections span more than one host (%s)\n"+
			"  Publish them separately with --collection <name>",
		strings.Join(names, ", "))
}
