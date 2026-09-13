package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// setupPruneCollection creates a collection with namespace as the owner login,
// pre-populated with currentRepos. Returns the collection and a fresh temp destDir.
func setupPruneCollection(t *testing.T, namespace string, currentRepos ...string) (*collection.Collection, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	owner := api.UserInfo{ID: "owner-id", Login: namespace}
	col, err := collection.New("prune-test", "github.com", owner, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	for _, r := range currentRepos {
		col.Repos = append(col.Repos, collection.RepoAccess{Name: r, Groups: []string{}, Users: []string{}})
	}

	return col, t.TempDir()
}

// makeGitClone creates a fake cloned directory under destDir with a .git/config
// file pointing remoteURL as the origin.
func makeGitClone(t *testing.T, destDir, repoName, remoteURL string) string {
	t.Helper()
	repoDir := filepath.Join(destDir, repoName)
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .git: %v", err)
	}
	cfg := fmt.Sprintf(
		"[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = %s\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n",
		remoteURL,
	)
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile .git/config: %v", err)
	}
	return repoDir
}

// resetPruneFns saves all three prune injectable vars and restores them on cleanup.
func resetPruneFns(t *testing.T) {
	t.Helper()
	origHUC := pruneHasUncommittedChangesFn
	origIT := pruneIsTerminalFn
	origCF := pruneConfirmFn
	t.Cleanup(func() {
		pruneHasUncommittedChangesFn = origHUC
		pruneIsTerminalFn = origIT
		pruneConfirmFn = origCF
	})
}

// TestPullPrune_IdentifiesStaleClones verifies that a directory present on disk
// but absent from col.Repos (with a matching namespace origin URL) is deleted.
func TestPullPrune_IdentifiesStaleClones(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme", "active-repo")
	makeGitClone(t, destDir, "old-repo", "https://github.com/acme/old-repo.git")
	makeGitClone(t, destDir, "active-repo", "https://github.com/acme/active-repo.git")

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return false, nil }
	pruneIsTerminalFn = func() bool { return true }
	pruneConfirmFn = func(msg string) bool { return true }

	if err := runPrune(col, destDir, false); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "old-repo")); !os.IsNotExist(err) {
		t.Error("expected old-repo (stale) to be deleted")
	}
	if _, err := os.Stat(filepath.Join(destDir, "active-repo")); err != nil {
		t.Errorf("expected active-repo to still exist: %v", err)
	}
}

// TestPullPrune_VerifiesRemoteURL verifies that a directory whose origin URL
// belongs to a different namespace is not pruned even when absent from col.Repos.
func TestPullPrune_VerifiesRemoteURL(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme") // no repos in collection
	makeGitClone(t, destDir, "payments-api", "https://github.com/other-org/payments-api.git")
	makeGitClone(t, destDir, "stale-api", "https://github.com/acme/stale-api.git")

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return false, nil }
	pruneIsTerminalFn = func() bool { return true }
	pruneConfirmFn = func(msg string) bool { return true }

	if err := runPrune(col, destDir, false); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "payments-api")); err != nil {
		t.Errorf("expected payments-api (other-org clone) to be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "stale-api")); !os.IsNotExist(err) {
		t.Error("expected stale-api (acme clone, absent from collection) to be deleted")
	}
}

// TestPullPrune_SkipsUncommittedChanges verifies that a stale clone with dirty
// working tree is not deleted.
func TestPullPrune_SkipsUncommittedChanges(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme") // no repos — clone is stale
	makeGitClone(t, destDir, "dirty-repo", "https://github.com/acme/dirty-repo.git")

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return true, nil }
	pruneIsTerminalFn = func() bool { return true }
	pruneConfirmFn = func(msg string) bool { return true } // would delete if clean

	if err := runPrune(col, destDir, false); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "dirty-repo")); err != nil {
		t.Errorf("expected dirty-repo to be preserved (has uncommitted changes): %v", err)
	}
}

// TestPullPrune_SkipsNonGitDirectories verifies that a plain directory with no
// .git subdirectory is never treated as a prune candidate.
func TestPullPrune_SkipsNonGitDirectories(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme") // no repos
	plainDir := filepath.Join(destDir, "plain-dir")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return false, nil }
	pruneIsTerminalFn = func() bool { return true }
	var confirmCalled bool
	pruneConfirmFn = func(msg string) bool { confirmCalled = true; return true }

	if err := runPrune(col, destDir, false); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if confirmCalled {
		t.Error("expected non-git directory to be skipped without a deletion prompt")
	}
	if _, err := os.Stat(plainDir); err != nil {
		t.Errorf("expected plain-dir to still exist: %v", err)
	}
}

// TestPullPrune_DryRun verifies that --dry-run prints the stale clone list
// without deleting anything.
func TestPullPrune_DryRun(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme") // no repos — clone is stale
	makeGitClone(t, destDir, "old-repo", "https://github.com/acme/old-repo.git")

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return false, nil }

	out := captureStdout(func() {
		if err := runPrune(col, destDir, true); err != nil {
			t.Fatalf("runPrune dry-run: %v", err)
		}
	})

	if _, err := os.Stat(filepath.Join(destDir, "old-repo")); err != nil {
		t.Errorf("expected old-repo to still exist after dry-run: %v", err)
	}
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %s", out)
	}
	if !strings.Contains(out, "old-repo") {
		t.Errorf("expected old-repo listed in dry-run output, got: %s", out)
	}
}

// TestPullPrune_PromptsIndividually verifies that pruneConfirmFn is invoked once
// per stale clone, not once for the entire batch.
func TestPullPrune_PromptsIndividually(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme") // no repos — both stale
	makeGitClone(t, destDir, "old-repo-1", "https://github.com/acme/old-repo-1.git")
	makeGitClone(t, destDir, "old-repo-2", "https://github.com/acme/old-repo-2.git")

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return false, nil }
	pruneIsTerminalFn = func() bool { return true }
	var prompts []string
	pruneConfirmFn = func(msg string) bool {
		prompts = append(prompts, msg)
		return false // decline — just verifying the prompt count
	}

	if err := runPrune(col, destDir, false); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if len(prompts) != 2 {
		t.Errorf("expected one prompt per stale clone (2 total), got %d: %v", len(prompts), prompts)
	}
}

// TestPullPrune_SkipsUnrelatedRepos verifies that directories cloned from a
// completely different organisation are never touched.
func TestPullPrune_SkipsUnrelatedRepos(t *testing.T) {
	col, destDir := setupPruneCollection(t, "acme", "my-repo")
	makeGitClone(t, destDir, "shared-lib", "https://github.com/third-party-org/shared-lib.git")
	makeGitClone(t, destDir, "my-repo", "https://github.com/acme/my-repo.git")

	resetPruneFns(t)
	pruneHasUncommittedChangesFn = func(dir string) (bool, error) { return false, nil }
	pruneIsTerminalFn = func() bool { return true }
	var confirmMsgs []string
	pruneConfirmFn = func(msg string) bool {
		confirmMsgs = append(confirmMsgs, msg)
		return false
	}

	if err := runPrune(col, destDir, false); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	for _, msg := range confirmMsgs {
		if strings.Contains(msg, "shared-lib") {
			t.Errorf("should never prompt to delete shared-lib (belongs to third-party-org), got prompt: %q", msg)
		}
	}
	if _, err := os.Stat(filepath.Join(destDir, "shared-lib")); err != nil {
		t.Errorf("expected shared-lib to still exist: %v", err)
	}
}

// ── B4: --all flag tests ─────────────────────────────────────────────────────

func setupPullAllTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	mock.collabs["owner/repo1/owner"] = true
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})
}

func makePullCollection(t *testing.T, name string, repos []string) {
	t.Helper()
	col, err := collection.New(name, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New(%s): %v", name, err)
	}
	for _, r := range repos {
		col.Repos = append(col.Repos, collection.RepoAccess{Name: r, Groups: []string{}, Users: []string{}})
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save(%s): %v", name, err)
	}
}

func resetPullFlags() func() {
	old := pullAll
	pullAll = false
	return func() { pullAll = old }
}

func TestPullAll_RunsOnAllCollections(t *testing.T) {
	defer resetPullFlags()()
	setupPullAllTest(t)
	makePullCollection(t, "pull-a", []string{"repo1"})
	makePullCollection(t, "pull-b", []string{"repo1"})

	dest := t.TempDir()
	// No repo dirs cloned — missing repos are skipped, no git.Pull calls.
	stderr := captureStderr(func() {
		captureStdout(func() {
			if err := runPullAll(dest); err != nil {
				t.Fatalf("runPullAll: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "pull-a") || !strings.Contains(stderr, "pull-b") {
		t.Errorf("expected both collection names in output, got: %q", stderr)
	}
}

func TestPullAll_SkipsInaccessible(t *testing.T) {
	defer resetPullFlags()()
	setupPullAllTest(t)
	makePullCollection(t, "accessible-col", []string{"repo1"})

	// Create a collection for a different host that has no token in the test env.
	inaccessible, err := collection.New("inaccessible-col", "gitlab.com",
		api.UserInfo{ID: "other-id", Login: "other"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New inaccessible: %v", err)
	}
	if err := inaccessible.Save(); err != nil {
		t.Fatalf("inaccessible.Save: %v", err)
	}

	dest := t.TempDir()
	var runErr error
	captureStderr(func() {
		captureStdout(func() {
			runErr = runPullAll(dest)
		})
	})

	if runErr != nil {
		t.Errorf("runPullAll should succeed even when one collection is inaccessible, got: %v", runErr)
	}
}

func TestPullAll_ErrorOnBothArgAndAll(t *testing.T) {
	defer resetPullFlags()()
	pullAll = true

	var runErr error
	captureStderr(func() {
		captureStdout(func() {
			runErr = runPull(nil, []string{"some-collection"})
		})
	})

	if runErr == nil {
		t.Error("expected error when both collection arg and --all are given, got nil")
	}
}

func TestPullAll_EmptyCollectionsDir(t *testing.T) {
	defer resetPullFlags()()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	dest := t.TempDir()
	var runErr error
	captureStderr(func() {
		captureStdout(func() {
			runErr = runPullAll(dest)
		})
	})

	if runErr != nil {
		t.Errorf("expected no error for empty collections dir, got: %v", runErr)
	}
}
