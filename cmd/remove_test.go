package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func setupRemoveTest(t *testing.T) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("remove-col", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id"}
	col.Logins["alice-id"] = "alice"
	col.Repos = []collection.RepoAccess{
		{Name: "old-service", Groups: []string{}, Users: []string{}},
		{Name: "keep-me", Groups: []string{}, Users: []string{}},
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	mock := newMultiAddMock()
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() { cachedClient = nil; cachedUser = ""; cachedUserID = "" })

	return col
}

// injectStdin replaces os.Stdin with a pipe that has the given text pre-written.
func injectStdin(t *testing.T, text string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	fmt.Fprintln(w, text)
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; r.Close() })
}

// TestRemove_RepoNotInCollection verifies an error is returned before confirmation
// when the target repo is not in the collection.
func TestRemove_RepoNotInCollection(t *testing.T) {
	setupRemoveTest(t)

	err := runRemove(removeCmd, []string{"remove-col", "no-such-repo"})
	if err == nil {
		t.Fatal("expected error for missing repo, got nil")
	}
	if !strings.Contains(err.Error(), "not in collection") {
		t.Errorf("expected 'not in collection' in error, got: %v", err)
	}
}

// TestRemove_CollectionNotFound verifies an error is returned for an unknown collection.
func TestRemove_CollectionNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	cachedClient = newMultiAddMock()
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() { cachedClient = nil; cachedUser = ""; cachedUserID = "" })

	err := runRemove(removeCmd, []string{"no-such-collection", "repo"})
	if err == nil {
		t.Fatal("expected error for missing collection, got nil")
	}
}

// TestRemove_Confirmed_RemovesRepo verifies that typing the repo name confirms the
// removal and the repo is deleted from the saved collection.
func TestRemove_Confirmed_RemovesRepo(t *testing.T) {
	setupRemoveTest(t)
	injectStdin(t, "old-service")

	if err := runRemove(removeCmd, []string{"remove-col", "old-service"}); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	col, err := collection.Load("remove-col")
	if err != nil {
		t.Fatalf("reload collection: %v", err)
	}
	for _, r := range col.Repos {
		if r.Name == "old-service" {
			t.Error("expected old-service to be removed, but it is still in the collection")
		}
	}
	found := false
	for _, r := range col.Repos {
		if r.Name == "keep-me" {
			found = true
		}
	}
	if !found {
		t.Error("keep-me should remain in the collection after removing old-service")
	}
}

// TestRemove_WrongWord_Aborts verifies that providing the wrong confirmation word
// aborts the removal and leaves the collection unchanged.
func TestRemove_WrongWord_Aborts(t *testing.T) {
	setupRemoveTest(t)
	injectStdin(t, "wrong-word")

	err := runRemove(removeCmd, []string{"remove-col", "old-service"})
	if err == nil {
		t.Fatal("expected error when confirmation aborted, got nil")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("expected 'aborted' in error, got: %v", err)
	}

	col, err := collection.Load("remove-col")
	if err != nil {
		t.Fatalf("reload collection: %v", err)
	}
	for _, r := range col.Repos {
		if r.Name == "old-service" {
			return // still present — correct
		}
	}
	t.Error("expected old-service to still be present after aborted removal")
}
