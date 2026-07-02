package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
)

// writeV1Collection writes a legacy-format YAML (version "1") into the
// collections directory so tests can exercise roleFor's legacy code path.
func writeV1Collection(t *testing.T, name, owner string, members []string) {
	t.Helper()
	collDir, err := config.CollectionsDir()
	if err != nil {
		t.Fatalf("CollectionsDir: %v", err)
	}
	if err := config.EnsureDir(collDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	memberYAML := ""
	for _, m := range members {
		memberYAML += "  - " + m + "\n"
	}
	if len(members) > 0 {
		memberYAML = "members:\n" + memberYAML
	}
	content := "name: " + name + "\nhost: github.com\nvisibility: private\nowner: " + owner + "\n" + memberYAML + "version: \"1\"\n"
	if err := os.WriteFile(filepath.Join(collDir, name+".yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// ── Version "2" (CurrentVersion) path — ID-based comparison ──────────────────

func TestRoleFor_V2_Owner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUserID("github.com", "owner-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}

	col, err := collection.New("proj", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	role, ok := roleFor(col)
	if !ok {
		t.Fatal("roleFor returned ok=false for the collection owner")
	}
	if role != "owner" {
		t.Errorf("role = %q, want owner", role)
	}
}

func TestRoleFor_V2_Member(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUserID("github.com", "alice-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}

	col, err := collection.New("proj", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = append(col.Members, "alice-id")
	col.Logins["alice-id"] = "alice"

	role, ok := roleFor(col)
	if !ok {
		t.Fatal("roleFor returned ok=false for a collection member")
	}
	if role != "member" {
		t.Errorf("role = %q, want member", role)
	}
}

func TestRoleFor_V2_NotMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUserID("github.com", "charlie-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}

	col, err := collection.New("proj", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = append(col.Members, "alice-id")
	col.Logins["alice-id"] = "alice"

	_, ok := roleFor(col)
	if ok {
		t.Error("roleFor returned ok=true for someone not in the collection")
	}
}

func TestRoleFor_V2_NoIDCached(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	// No SaveUserID call — nothing in config for this host.

	col, err := collection.New("proj", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	_, ok := roleFor(col)
	if ok {
		t.Error("roleFor returned ok=true when no user ID is cached — user has not authenticated")
	}
}

// ── Version "1" (legacy) path — login-string comparison ──────────────────────

func TestRoleFor_V1_Owner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUser("github.com", "alice"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	writeV1Collection(t, "legacy", "alice", []string{"bob"})

	col, err := collection.Load("legacy")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}

	role, ok := roleFor(col)
	if !ok {
		t.Fatal("roleFor returned ok=false for the v1 collection owner")
	}
	if role != "owner" {
		t.Errorf("role = %q, want owner", role)
	}
}

func TestRoleFor_V1_Member(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUser("github.com", "bob"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	writeV1Collection(t, "legacy", "alice", []string{"bob"})

	col, err := collection.Load("legacy")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}

	role, ok := roleFor(col)
	if !ok {
		t.Fatal("roleFor returned ok=false for a v1 collection member")
	}
	if role != "member" {
		t.Errorf("role = %q, want member", role)
	}
}

func TestRoleFor_V1_NotMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUser("github.com", "charlie"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	writeV1Collection(t, "legacy", "alice", []string{"bob"})

	col, err := collection.Load("legacy")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}

	_, ok := roleFor(col)
	if ok {
		t.Error("roleFor returned ok=true for someone not in a v1 collection")
	}
}

// TestRoleFor_V1_V2_SameHost verifies that a v1 and a v2 collection on the
// same host are each evaluated by the correct branch — the v1 collection uses
// the cached login (config.LoadUser) and the v2 collection uses the cached ID
// (config.LoadUserID), even when both are in the config simultaneously.
func TestRoleFor_V1_V2_SameHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUser("github.com", "alice"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if err := config.SaveUserID("github.com", "alice-new-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}

	// v1 collection: alice is the owner by login
	writeV1Collection(t, "v1col", "alice", nil)
	v1col, err := collection.Load("v1col")
	if err != nil {
		t.Fatalf("collection.Load(v1col): %v", err)
	}

	// v2 collection: alice-new-id is the owner by platform ID
	v2col, err := collection.New("v2col", "github.com", api.UserInfo{ID: "alice-new-id", Login: "alice"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	v1role, ok := roleFor(v1col)
	if !ok || v1role != "owner" {
		t.Errorf("v1 collection: role=%q ok=%v, want owner/true", v1role, ok)
	}

	v2role, ok := roleFor(v2col)
	if !ok || v2role != "owner" {
		t.Errorf("v2 collection: role=%q ok=%v, want owner/true", v2role, ok)
	}
}
