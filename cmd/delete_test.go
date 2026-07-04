package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// TestPrintDeletionImpact verifies that printDeletionImpact lists the member
// logins that will have their access revoked.
func TestPrintDeletionImpact(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id", "bob-id"}
	col.Logins["alice-id"] = "alice"
	col.Logins["bob-id"] = "bob"
	col.Repos = []collection.RepoAccess{{Name: "repo1"}, {Name: "repo2"}}

	var buf bytes.Buffer
	printDeletionImpact(&buf, col)

	out := buf.String()
	if !strings.Contains(out, "alice") {
		t.Errorf("expected alice in deletion impact, got: %q", out)
	}
	if !strings.Contains(out, "bob") {
		t.Errorf("expected bob in deletion impact, got: %q", out)
	}
}

// TestPrintDeletionImpact_Empty verifies that printDeletionImpact is silent
// when there are no members to revoke.
func TestPrintDeletionImpact_Empty(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	var buf bytes.Buffer
	printDeletionImpact(&buf, col)

	if buf.Len() != 0 {
		t.Errorf("expected no output for empty collection, got: %q", buf.String())
	}
}

// TestDeleteDryRun_ShowsImpactWithoutDeleting verifies that --dry-run does not
// delete the collection or call the platform API.
func TestDeleteDryRun_ShowsImpactWithoutDeleting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id"}
	col.Logins["alice-id"] = "alice"
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	mock := &transferMock{}
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() { cachedClient = nil; cachedUser = ""; cachedUserID = "" })

	old := deleteDryRun
	deleteDryRun = true
	defer func() { deleteDryRun = old }()

	if err := runDelete(nil, []string{"acme"}); err != nil {
		t.Fatalf("runDelete --dry-run: %v", err)
	}

	if _, err := collection.Load("acme"); err != nil {
		t.Errorf("expected collection to still exist after --dry-run: %v", err)
	}
}

// TestDeleteDryRun_EmptyCollection verifies that --dry-run on an empty
// collection exits cleanly without any confirmation prompt.
func TestDeleteDryRun_EmptyCollection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("empty-col", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	mock := &transferMock{}
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() { cachedClient = nil; cachedUser = ""; cachedUserID = "" })

	old := deleteDryRun
	deleteDryRun = true
	defer func() { deleteDryRun = old }()

	if err := runDelete(nil, []string{"empty-col"}); err != nil {
		t.Fatalf("runDelete --dry-run on empty collection: %v", err)
	}

	if _, err := collection.Load("empty-col"); err != nil {
		t.Errorf("expected collection to still exist: %v", err)
	}
}
