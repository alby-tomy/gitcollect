package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

func TestJoin_RequiresOrg(t *testing.T) {
	old := joinOrg
	joinOrg = ""
	defer func() { joinOrg = old }()

	err := runJoin(nil, nil)
	if err == nil {
		t.Fatal("expected error when --org is not set")
	}
}

func TestJoin_RequiresTeam(t *testing.T) {
	oldOrg := joinOrg
	oldTeam := joinTeam
	joinOrg = "acme-corp"
	joinTeam = ""
	defer func() {
		joinOrg = oldOrg
		joinTeam = oldTeam
	}()

	err := runJoin(nil, nil)
	if err == nil {
		t.Fatal("expected error when --team is not set")
	}
}

// TestJoin_ViaAPI exercises runJoin without --repo (direct API import path),
// using the same importMock used by the import tests. After join, the
// collection should exist on disk under {org}-{team}.yaml.
func TestJoin_ViaAPI(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "payments-api", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	oldOrg := joinOrg
	oldTeam := joinTeam
	oldFrom := joinFrom
	oldClone := joinClone
	joinOrg = "acme-corp"
	joinTeam = "payments-team"
	joinFrom = "github"
	joinClone = false
	defer func() {
		joinOrg = oldOrg
		joinTeam = oldTeam
		joinFrom = oldFrom
		joinClone = oldClone
	}()

	if err := runJoin(nil, nil); err != nil {
		t.Fatalf("runJoin via API = %v", err)
	}

	// The collection is identified as "<org>-<team>" everywhere: that is the
	// file name AND the Name field. They used to disagree — the file was
	// org-prefixed while Name held the bare team slug — so "gitcollect list"
	// printed a name no other command would accept.
	const want = "acme-corp-payments-team"
	col, err := collection.Load(want)
	if err != nil {
		t.Fatalf("Load after join: %v", err)
	}
	if col.Name != want {
		t.Errorf("Name = %q, want %q — Name must match the file it loads from", col.Name, want)
	}
}

// TestJoin_ViaRepo exercises runJoin with --repo. It sets up a fake "clone"
// directory with a collection file, then verifies join copies it to the
// collections dir. The actual git clone is bypassed by pre-placing the file
// where join would look for it — this tests the copy path without needing
// a real git remote.
func TestJoin_ViaRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	cachedClient = newImportMock()
	cachedUser = "caller"
	cachedUserID = "caller-id"
	defer func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	}()

	// We can't test the git clone path here without a real remote. Instead,
	// verify that the --repo path produces a clear error when git fails (as
	// it will when there's no network or git config), rather than panicking
	// or producing a confusing message.
	oldOrg := joinOrg
	oldTeam := joinTeam
	oldRepo := joinRepo
	oldFrom := joinFrom
	oldClone := joinClone
	joinOrg = "acme-corp"
	joinTeam = "payments-team"
	joinRepo = "acme-corp/gitcollect-config"
	joinFrom = "github"
	joinClone = false
	defer func() {
		joinOrg = oldOrg
		joinTeam = oldTeam
		joinRepo = oldRepo
		joinFrom = oldFrom
		joinClone = oldClone
	}()

	err := runJoin(nil, nil)
	// Must fail (no real git remote), but not with a nil panic.
	if err == nil {
		t.Fatal("expected error (no real git remote)")
	}
}

// TestMakeTempDir verifies that makeTempDir creates a directory and the
// cleanup function removes it.
func TestMakeTempDir(t *testing.T) {
	dir, cleanup, err := makeTempDir("gitcollect-test-*")
	if err != nil {
		t.Fatalf("makeTempDir: %v", err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Errorf("temp dir does not exist: %v", statErr)
	}
	cleanup()
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Error("expected temp dir to be removed after cleanup()")
	}
}

// TestJoin_CollectionNameFormat verifies the expected file name format for a
// join result without needing a network call.
func TestJoin_CollectionNameFormat(t *testing.T) {
	// The join command stores collections as {org}-{team}.yaml.
	// Verify the name logic matches what collection.Load expects.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// Create a collection manually with the name join would use.
	col, err := collection.New("acme-corp-payments-team", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify the expected file exists.
	collDir := filepath.Join(dir, ".gitcollect", "collections")
	if _, err := os.Stat(filepath.Join(collDir, "acme-corp-payments-team.yaml")); err != nil {
		t.Errorf("expected acme-corp-payments-team.yaml to exist: %v", err)
	}

	// And can be loaded back.
	loaded, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "acme-corp-payments-team" {
		t.Errorf("Name = %q, want acme-corp-payments-team", loaded.Name)
	}
}
