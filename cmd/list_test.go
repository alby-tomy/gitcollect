package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
)

// captureListStdout redirects os.Stdout for the duration of fn and returns
// everything written to it.
func captureListStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	fn()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

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

	if got := roleFor(col); got != "owner" {
		t.Errorf("role = %q, want owner", got)
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

	if got := roleFor(col); got != "member" {
		t.Errorf("role = %q, want member", got)
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

	// A private collection the caller is not part of has no role for
	// them, but the row is still listed — the file is on their disk.
	if got := roleFor(col); got != "—" {
		t.Errorf("for someone not in the collection: role = %q, want \u2014", got)
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

	// A private collection the caller is not part of has no role for
	// them, but the row is still listed — the file is on their disk.
	if got := roleFor(col); got != "—" {
		t.Errorf("when no user ID is cached — user has not authenticated: role = %q, want \u2014", got)
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

	if got := roleFor(col); got != "owner" {
		t.Errorf("role = %q, want owner", got)
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

	if got := roleFor(col); got != "member" {
		t.Errorf("role = %q, want member", got)
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

	// A private collection the caller is not part of has no role for
	// them, but the row is still listed — the file is on their disk.
	if got := roleFor(col); got != "—" {
		t.Errorf("for someone not in a v1 collection: role = %q, want \u2014", got)
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

	if got := roleFor(v1col); got != "owner" {
		t.Errorf("v1 collection: role=%q, want owner", got)
	}
	if got := roleFor(v2col); got != "owner" {
		t.Errorf("v2 collection: role=%q, want owner", got)
	}
}

func TestList_ShowsDescription(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Cleanup(func() { listJSON = false; listPrivate = false; listPublic = false })

	if err := config.SaveUserID("github.com", "owner-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}
	col, err := collection.New("mycol", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Description = "My collection description"
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	out := captureListStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Errorf("runList: %v", err)
		}
	})

	if !strings.Contains(out, `"My collection description"`) {
		t.Errorf("output = %q, want quoted description", out)
	}
}

func TestList_ShowsNoDescriptionFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Cleanup(func() { listJSON = false; listPrivate = false; listPublic = false })

	if err := config.SaveUserID("github.com", "owner-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}
	col, err := collection.New("nodesc", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	out := captureListStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Errorf("runList: %v", err)
		}
	})

	if !strings.Contains(out, "(no description)") {
		t.Errorf("output = %q, want (no description) fallback", out)
	}
}

func TestList_JSON_IncludesDescription(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Cleanup(func() { listJSON = false; listPrivate = false; listPublic = false })

	if err := config.SaveUserID("github.com", "owner-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}
	col, err := collection.New("jsoncol", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Description = "json desc"
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	listJSON = true
	out := captureListStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Errorf("runList: %v", err)
		}
	})

	if !strings.Contains(out, `"description"`) || !strings.Contains(out, `"json desc"`) {
		t.Errorf("JSON output = %q, want description field with value", out)
	}
}

// --- regression: list must never hide a collection that is on disk ---

func writeListCol(t *testing.T, name string, vis collection.Visibility, members []string) {
	t.Helper()
	col, err := collection.New(name, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, vis)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	for _, m := range members {
		col.Members = append(col.Members, m)
		col.Logins[m] = m
	}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// Rows used to be dropped whenever no role could be determined, so list
// printed an empty table before "gitcollect auth" had ever run — exactly
// the state a new joiner is in right after pull-config or join.
func TestList_ShowsCollectionsWithoutAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Cleanup(func() { listJSON = false; listPrivate = false; listPublic = false })

	writeListCol(t, "payments", collection.VisibilityPrivate, nil)

	out := captureListStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Errorf("runList: %v", err)
		}
	})
	if !strings.Contains(out, "payments") {
		t.Errorf("a collection on disk must be listed even with no cached identity, got:\n%s", out)
	}
}

func TestRoleFor_NoIdentityIsUnknownNotHidden(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("c", "github.com",
		api.UserInfo{ID: "someone-else", Login: "someone"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := roleFor(col); got != "—" {
		t.Errorf("roleFor with no cached identity = %q, want %q", got, "—")
	}
}

// A public collection the caller is not listed in is still theirs to see;
// show renders it without any identity at all.
func TestRoleFor_PublicNonMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUserID("github.com", "stranger"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}

	col, err := collection.New("c", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPublic)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := roleFor(col); got != "public" {
		t.Errorf("roleFor(public, non-member) = %q, want %q", got, "public")
	}
}

func TestRoleFor_MemberBeatsPublic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := config.SaveUserID("github.com", "bob-id"); err != nil {
		t.Fatalf("SaveUserID: %v", err)
	}

	col, err := collection.New("c", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPublic)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	col.Members = []string{"bob-id"}
	col.Logins["bob-id"] = "bob"

	if got := roleFor(col); got != "member" {
		t.Errorf("a listed member of a public collection should read as member, got %q", got)
	}
}
