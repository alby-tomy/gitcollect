package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

func setupCopyTest(t *testing.T, collName string) {
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

func TestCopy_CreatesNewFile(t *testing.T) {
	setupCopyTest(t, "source")

	if err := runCopy(nil, []string{"source", "destination"}); err != nil {
		t.Fatalf("runCopy = %v", err)
	}

	if _, err := collection.Load("destination"); err != nil {
		t.Errorf("destination should exist after copy, got: %v", err)
	}
	if _, err := collection.Load("source"); err != nil {
		t.Errorf("source should still exist after copy, got: %v", err)
	}
}

func TestCopy_RequiresOwner(t *testing.T) {
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

	if err := runCopy(nil, []string{"mycollab", "copy-attempt"}); err == nil {
		t.Fatal("expected error for non-owner copy, got nil")
	}
}

func TestCopy_NameCollision(t *testing.T) {
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
	col1, _ := collection.New("src", "github.com", owner, collection.VisibilityPrivate)
	col1.Save() //nolint:errcheck
	col2, _ := collection.New("dst", "github.com", owner, collection.VisibilityPrivate)
	col2.Save() //nolint:errcheck

	if err := runCopy(nil, []string{"src", "dst"}); err == nil {
		t.Fatal("expected error copying to an existing collection name, got nil")
	}
}
