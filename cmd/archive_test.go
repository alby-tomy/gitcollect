package cmd

import (
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// setupArchiveTest saves a collection owned by "owner" with "alice" as a
// plain member, and injects a mock client plus the caller identity the
// archive commands will see. Callers override cachedUser/cachedUserID to
// act as somebody other than the owner.
func setupArchiveTest(t *testing.T) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id"}
	col.Logins["alice-id"] = "alice"
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	cachedClient = newMultiAddMock()
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})
	return col
}

// actAs switches the cached identity for the rest of the test, so a
// command runs as somebody other than the collection owner.
func actAs(login, id string) {
	cachedUser = login
	cachedUserID = id
}

func reloadArchive(t *testing.T) *collection.Collection {
	t.Helper()
	col, err := collection.Load("acme")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return col
}

func TestArchive_OwnerCanArchive(t *testing.T) {
	setupArchiveTest(t)

	if err := runArchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("runArchive: %v", err)
	}
	if !reloadArchive(t).Archived {
		t.Error("expected collection to be archived on disk")
	}
}

// TestArchive_NonOwnerRejected is the regression test for the missing
// ownership check. archive's help promises "Only the collection owner can
// archive it", but both commands discarded the caller ID loadForOwner
// returns and never compared it, so any member could hide a collection
// from everyone's list/sync/status.
func TestArchive_NonOwnerRejected(t *testing.T) {
	setupArchiveTest(t)
	actAs("alice", "alice-id")

	err := runArchive(nil, []string{"acme"})
	if err == nil {
		t.Fatal("expected a non-owner to be refused")
	}
	if !strings.Contains(err.Error(), "owner") {
		t.Errorf("error should name the owner requirement, got: %v", err)
	}
	if reloadArchive(t).Archived {
		t.Error("collection must not be archived by a non-owner")
	}
}

func TestUnarchive_NonOwnerRejected(t *testing.T) {
	setupArchiveTest(t)
	if err := runArchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("runArchive: %v", err)
	}

	actAs("alice", "alice-id")
	if err := runUnarchive(nil, []string{"acme"}); err == nil {
		t.Fatal("expected a non-owner to be refused")
	}
	if !reloadArchive(t).Archived {
		t.Error("collection must stay archived when a non-owner tries to unarchive")
	}
}

func TestArchive_RoundTrip(t *testing.T) {
	setupArchiveTest(t)

	if err := runArchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !reloadArchive(t).Archived {
		t.Fatal("expected archived after archive")
	}
	if err := runUnarchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if reloadArchive(t).Archived {
		t.Error("expected not archived after unarchive")
	}
}

func TestArchive_IsIdempotent(t *testing.T) {
	setupArchiveTest(t)

	if err := runArchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("first archive: %v", err)
	}
	// A second archive is a no-op, not an error.
	if err := runArchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("second archive should be a no-op, got: %v", err)
	}
	if !reloadArchive(t).Archived {
		t.Error("expected still archived")
	}
}

func TestUnarchive_NotArchivedIsNoOp(t *testing.T) {
	setupArchiveTest(t)

	if err := runUnarchive(nil, []string{"acme"}); err != nil {
		t.Fatalf("unarchive on a live collection should be a no-op, got: %v", err)
	}
	if reloadArchive(t).Archived {
		t.Error("collection should remain unarchived")
	}
}

func TestArchive_UnknownCollection(t *testing.T) {
	setupArchiveTest(t)

	if err := runArchive(nil, []string{"nope"}); err == nil {
		t.Fatal("expected an error for an unknown collection")
	}
}
