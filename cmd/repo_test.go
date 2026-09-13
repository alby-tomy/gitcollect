package cmd

import (
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

func setupRepoTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("repo-col", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id", "bob-id"}
	col.Logins["alice-id"] = "alice"
	col.Logins["bob-id"] = "bob"
	col.Repos = []collection.RepoAccess{
		{Name: "api", Groups: []string{}, Users: []string{}},
		{Name: "frontend", Groups: []string{"eng"}, Users: []string{}},
	}
	col.Groups = map[string][]string{"eng": {"alice-id"}}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	mock := newMultiAddMock()
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() { cachedClient = nil; cachedUser = ""; cachedUserID = "" })
}

func resetRepoAccessFlags(t *testing.T) {
	t.Helper()
	oldGroups, oldUsers, oldOpen := repoAccessGroups, repoAccessUsers, repoAccessOpen
	t.Cleanup(func() {
		repoAccessGroups = oldGroups
		repoAccessUsers = oldUsers
		repoAccessOpen = oldOpen
		// Reset cobra's Changed state so the next test starts clean.
		for _, name := range []string{"groups", "users", "open"} {
			if f := repoAccessCmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
	repoAccessGroups = nil
	repoAccessUsers = nil
	repoAccessOpen = false
	for _, name := range []string{"groups", "users", "open"} {
		if f := repoAccessCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

// TestRepoAccess_Open sets a restricted repo to open and verifies the change persists.
func TestRepoAccess_Open(t *testing.T) {
	setupRepoTest(t)
	resetRepoAccessFlags(t)
	repoAccessOpen = true
	repoAccessCmd.Flags().Lookup("open").Changed = true

	if err := runRepoAccess(repoAccessCmd, []string{"repo-col", "frontend"}); err != nil {
		t.Fatalf("runRepoAccess --open: %v", err)
	}

	col, err := collection.Load("repo-col")
	if err != nil {
		t.Fatalf("reload collection: %v", err)
	}
	for _, r := range col.Repos {
		if r.Name == "frontend" {
			if len(r.Groups) != 0 || len(r.Users) != 0 {
				t.Errorf("expected frontend to be open (no groups/users), got groups=%v users=%v", r.Groups, r.Users)
			}
			return
		}
	}
	t.Error("frontend not found in saved collection")
}

// TestRepoAccess_NoFlag_Error verifies that omitting --groups, --users, and --open is a usage error.
func TestRepoAccess_NoFlag_Error(t *testing.T) {
	setupRepoTest(t)
	resetRepoAccessFlags(t)

	err := runRepoAccess(repoAccessCmd, []string{"repo-col", "api"})
	if err == nil {
		t.Fatal("expected UsageError when no flag is set, got nil")
	}
	var ue *UsageError
	if !isUsageError(err, &ue) {
		t.Errorf("expected UsageError, got %T: %v", err, err)
	}
}

// TestRepoAccess_OpenAndGroups_Error verifies --open combined with --groups is a usage error.
func TestRepoAccess_OpenAndGroups_Error(t *testing.T) {
	setupRepoTest(t)
	resetRepoAccessFlags(t)
	repoAccessOpen = true
	repoAccessGroups = []string{"eng"}
	repoAccessCmd.Flags().Lookup("open").Changed = true
	repoAccessCmd.Flags().Lookup("groups").Changed = true

	err := runRepoAccess(repoAccessCmd, []string{"repo-col", "api"})
	if err == nil {
		t.Fatal("expected UsageError for --open + --groups, got nil")
	}
	var ue *UsageError
	if !isUsageError(err, &ue) {
		t.Errorf("expected UsageError, got %T: %v", err, err)
	}
}

// TestRepoAccess_RepoNotFound verifies an error for a repo not in the collection.
func TestRepoAccess_RepoNotFound(t *testing.T) {
	setupRepoTest(t)
	resetRepoAccessFlags(t)
	repoAccessOpen = true
	repoAccessCmd.Flags().Lookup("open").Changed = true

	err := runRepoAccess(repoAccessCmd, []string{"repo-col", "no-such-repo"})
	if err == nil {
		t.Fatal("expected error for unknown repo, got nil")
	}
	if !strings.Contains(err.Error(), "not in collection") {
		t.Errorf("expected 'not in collection' in error, got: %v", err)
	}
}

// TestRepoShow_ListsMembers verifies repo show prints member access.
func TestRepoShow_ListsMembers(t *testing.T) {
	setupRepoTest(t)

	out := captureStdout(func() {
		if err := runRepoShow(repoShowCmd, []string{"repo-col", "api"}); err != nil {
			t.Fatalf("runRepoShow: %v", err)
		}
	})

	if !strings.Contains(out, "api") {
		t.Errorf("expected repo name in output, got: %q", out)
	}
}

// TestRepoShow_NotFound returns an error for an unknown repo.
func TestRepoShow_NotFound(t *testing.T) {
	setupRepoTest(t)

	err := runRepoShow(repoShowCmd, []string{"repo-col", "no-such-repo"})
	if err == nil {
		t.Fatal("expected error for unknown repo, got nil")
	}
}
