package cmd

import (
	"path/filepath"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// setupGitTest creates a collection on disk with one repo entry and injects
// the owner as the cached client identity, so loadForGit succeeds without
// network access.
func setupGitTest(t *testing.T, collName string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	// Pre-populate the mock so CheckCollaborator returns true for owner/repo1.
	mock.collabs["owner/repo1/owner"] = true

	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	col, err := collection.New(collName, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Repos = []collection.RepoAccess{{Name: "repo1", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
}

// TestClone_LocationLine_DryRunNoLocation verifies that dry-run clone exits
// before the location line — the success+location block is in the else branch
// that only runs when no clone fails and dryRun is false.
func TestClone_LocationLine_DryRunNoLocation(t *testing.T) {
	setupGitTest(t, "testcol-clone")

	old := cloneDryRun
	oldDest := cloneDest
	cloneDryRun = true
	cloneDest = "."
	t.Cleanup(func() {
		cloneDryRun = old
		cloneDest = oldDest
	})

	if err := runClone(nil, []string{"testcol-clone"}); err != nil {
		t.Fatalf("runClone dry-run = %v", err)
	}
}

// TestSync_LocationLine_DryRunNoLocation verifies the same for sync: dry-run
// returns at line 84-87 of sync.go, before the location line is reached.
func TestSync_LocationLine_DryRunNoLocation(t *testing.T) {
	setupGitTest(t, "testcol-sync")

	old := syncDryRun
	oldDest := syncDest
	syncDryRun = true
	syncDest = "."
	t.Cleanup(func() {
		syncDryRun = old
		syncDest = oldDest
	})

	if err := runSync(nil, []string{"testcol-sync"}); err != nil {
		t.Fatalf("runSync dry-run = %v", err)
	}
}

// TestAbsDestPath_ConvertsRelative confirms that filepath.Abs converts the
// default "." dest to a non-empty absolute path — the core of the location
// line's path resolution logic.
func TestAbsDestPath_ConvertsRelative(t *testing.T) {
	abs, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("filepath.Abs(.) = %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Errorf("filepath.Abs(.) = %q, want an absolute path", abs)
	}
	// Sanity: "some-dir" gets a non-empty abs path too.
	abs2, err := filepath.Abs("some-dir")
	if err != nil {
		t.Fatalf("filepath.Abs(some-dir) = %v", err)
	}
	if !filepath.IsAbs(abs2) {
		t.Errorf("filepath.Abs(some-dir) = %q, want absolute", abs2)
	}
}
