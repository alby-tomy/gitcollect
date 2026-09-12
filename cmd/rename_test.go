package cmd

import (
	"os"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

func setupRenameTest(t *testing.T, collName string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
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
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
}

func TestRename_RenamesFile(t *testing.T) {
	setupRenameTest(t, "old-name")

	if err := runRename(nil, []string{"old-name", "new-name"}); err != nil {
		t.Fatalf("runRename = %v", err)
	}

	if _, err := collection.Load("new-name"); err != nil {
		t.Errorf("new-name should exist after rename, got: %v", err)
	}
	if _, err := collection.Load("old-name"); err == nil {
		t.Error("old-name should be gone after rename")
	}
}

func TestRename_RequiresOwner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	cachedClient = mock
	cachedUser = "other"
	cachedUserID = "other-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	col, err := collection.New("mycollab", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	err = runRename(nil, []string{"mycollab", "renamed"})
	if err == nil {
		t.Fatal("expected error for non-owner rename, got nil")
	}
}

func TestRename_NameCollision(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	owner := api.UserInfo{ID: "owner-id", Login: "owner"}
	col1, _ := collection.New("alpha", "github.com", owner, collection.VisibilityPrivate)
	col1.Save() //nolint:errcheck
	col2, _ := collection.New("beta", "github.com", owner, collection.VisibilityPrivate)
	col2.Save() //nolint:errcheck

	err := runRename(nil, []string{"alpha", "beta"})
	if err == nil {
		t.Fatal("expected error renaming to an existing collection name, got nil")
	}

	// original file must still exist
	if _, statErr := os.Stat(col1.Path()); statErr != nil {
		t.Errorf("alpha.yaml should still exist after failed rename: %v", statErr)
	}
}
