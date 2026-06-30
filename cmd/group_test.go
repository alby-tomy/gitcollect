package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/collection"
)

// TestAddOneToGroup exercises the per-username helper behind group add's
// multi-value support: a valid member succeeds, a non-member surfaces
// ErrNotMember as an error, and a sync failure surfaces an error without
// leaving the username in the group — covering the same three outcomes
// runGroupAdd loops over for a batch of usernames.
func TestAddOneToGroup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", "owner", collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice", "bob", "charlie"}
	if err := col.CreateGroup("red-team"); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	client := newMultiAddMock()

	if err := addOneToGroup(col, "acme", "red-team", "owner", "alice", client); err != nil {
		t.Fatalf("addOneToGroup(alice) = %v, want nil", err)
	}
	if !col.IsInGroup("alice", "red-team") {
		t.Error("expected alice to be in red-team after addOneToGroup")
	}

	if err := addOneToGroup(col, "acme", "red-team", "owner", "diana", client); err == nil {
		t.Fatal("addOneToGroup(diana) = nil, want an error (diana is not a member)")
	}

	client.failAddFor["bob"] = true
	col.Repos = []collection.RepoAccess{{Name: "r", Groups: []string{"red-team"}}}
	if err := addOneToGroup(col, "acme", "red-team", "owner", "bob", client); err == nil {
		t.Fatal("addOneToGroup(bob) = nil, want an error from the failing sync")
	}
	if col.IsInGroup("bob", "red-team") {
		t.Error("expected bob NOT to be in red-team after a failed sync (AddToGroup rolls back)")
	}
}
