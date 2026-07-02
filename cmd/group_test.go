package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// newGroupTestCol builds a collection with two members (dev-id/dev, ops-id/ops),
// two groups (backend, frontend), and optional group admin wiring.
// It does NOT save — callers inject any extra state then call injectGroupIdent.
func newGroupTestCol(t *testing.T, orgEnabled bool) *collection.Collection {
	t.Helper()
	col, err := collection.New("acme", "github.com",
		api.UserInfo{ID: "cto-id", Login: "cto"},
		collection.VisibilityPrivate,
	)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"dev-id", "ops-id"}
	col.Logins["dev-id"] = "dev"
	col.Logins["ops-id"] = "ops"
	col.Groups = map[string][]string{"backend": {}, "frontend": {}}
	col.GroupAdminsEnabled = orgEnabled
	if orgEnabled {
		col.GroupAdmins = map[string][]string{"backend": {"dev-id"}}
	}
	return col
}

// injectGroupIdent saves col and installs the given identity into the package
// vars that loadForOwner reads. Also installs a transferMock seeded with
// dev/ops identities so group add/remove calls that hit GetUser succeed.
func injectGroupIdent(t *testing.T, col *collection.Collection, callerID, callerLogin string) {
	t.Helper()
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
	mock := &transferMock{
		users: map[string]api.UserInfo{
			"dev": {ID: "dev-id", Login: "dev"},
			"ops": {ID: "ops-id", Login: "ops"},
		},
	}
	cachedClient = mock
	cachedUser = callerLogin
	cachedUserID = callerID
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})
}

// ── runGroupAdd auth matrix ──────────────────────────────────────────────────

func TestGroupAdd_NonOwner_OrgDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, false)
	injectGroupIdent(t, col, "dev-id", "dev")

	err := runGroupAdd(nil, []string{"acme", "backend", "ops"})
	if err == nil {
		t.Fatal("expected error when non-owner calls group add with org disabled")
	}
}

func TestGroupAdd_GroupAdmin_CorrectGroup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// org enabled: dev-id is admin of "backend"
	col := newGroupTestCol(t, true)
	injectGroupIdent(t, col, "dev-id", "dev")

	if err := runGroupAdd(nil, []string{"acme", "backend", "ops"}); err != nil {
		t.Fatalf("runGroupAdd = %v, want nil (group admin of backend should be allowed)", err)
	}
}

func TestGroupAdd_GroupAdmin_WrongGroup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// dev-id is admin of "backend" but attempts to manage "frontend"
	col := newGroupTestCol(t, true)
	injectGroupIdent(t, col, "dev-id", "dev")

	err := runGroupAdd(nil, []string{"acme", "frontend", "ops"})
	if err == nil {
		t.Fatal("expected error when group admin of backend tries to manage frontend")
	}
}

func TestGroupAdd_RegularMember_OrgEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// ops-id is not an admin of anything
	col := newGroupTestCol(t, true)
	injectGroupIdent(t, col, "ops-id", "ops")

	err := runGroupAdd(nil, []string{"acme", "backend", "dev"})
	if err == nil {
		t.Fatal("expected error when regular member (non-admin) calls group add")
	}
}

// ── runGroupRemove auth matrix ───────────────────────────────────────────────

func TestGroupRemove_GroupAdmin_CorrectGroup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true)
	col.Groups["backend"] = []string{"ops-id"} // ops is already in backend
	injectGroupIdent(t, col, "dev-id", "dev")  // dev-id is admin of backend

	if err := runGroupRemove(nil, []string{"acme", "backend", "ops"}); err != nil {
		t.Fatalf("runGroupRemove = %v, want nil (group admin should be able to remove from own group)", err)
	}
}

func TestGroupRemove_GroupAdmin_WrongGroup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true)
	col.Groups["frontend"] = []string{"ops-id"}
	injectGroupIdent(t, col, "dev-id", "dev") // admin of backend, not frontend

	err := runGroupRemove(nil, []string{"acme", "frontend", "ops"})
	if err == nil {
		t.Fatal("expected error when group admin of backend tries to remove from frontend")
	}
}

// ── runGroupAdminAdd ─────────────────────────────────────────────────────────

func TestGroupAdminAdd_NonOwner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true)
	injectGroupIdent(t, col, "dev-id", "dev") // not the owner

	err := runGroupAdminAdd(nil, []string{"acme", "backend", "ops"})
	if err == nil {
		t.Fatal("expected error when non-owner calls group admin add")
	}
}

func TestGroupAdminAdd_OrgDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, false) // org NOT enabled
	injectGroupIdent(t, col, "cto-id", "cto")

	err := runGroupAdminAdd(nil, []string{"acme", "backend", "dev"})
	if err == nil {
		t.Fatal("expected error when group admin support is not enabled")
	}
}

func TestGroupAdminAdd_GroupNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true)
	injectGroupIdent(t, col, "cto-id", "cto")

	err := runGroupAdminAdd(nil, []string{"acme", "no-such-group", "dev"})
	if err == nil {
		t.Fatal("expected error when group does not exist")
	}
}

func TestGroupAdminAdd_UserNotMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true)
	// add an unknown user to the mock but NOT to col.Members
	injectGroupIdent(t, col, "cto-id", "cto")
	cachedClient.(*transferMock).users["stranger"] = api.UserInfo{ID: "stranger-id", Login: "stranger"}

	err := runGroupAdminAdd(nil, []string{"acme", "backend", "stranger"})
	if err == nil {
		t.Fatal("expected error when target user is not a member")
	}
}

func TestGroupAdminAdd_Success(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true) // dev-id already admin of backend
	injectGroupIdent(t, col, "cto-id", "cto")

	if err := runGroupAdminAdd(nil, []string{"acme", "backend", "ops"}); err != nil {
		t.Fatalf("runGroupAdminAdd = %v, want nil", err)
	}

	loaded, err := collection.Load("acme")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	found := false
	for _, id := range loaded.GroupAdmins["backend"] {
		if id == "ops-id" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ops-id in GroupAdmins[backend] after group admin add, got %v", loaded.GroupAdmins["backend"])
	}
}

// ── runGroupAdminRemove ──────────────────────────────────────────────────────

func TestGroupAdminRemove_OwnerCanRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true) // dev-id is admin of backend
	injectGroupIdent(t, col, "cto-id", "cto")

	if err := runGroupAdminRemove(nil, []string{"acme", "backend", "dev"}); err != nil {
		t.Fatalf("runGroupAdminRemove = %v, want nil (owner should be able to remove any admin)", err)
	}

	loaded, _ := collection.Load("acme")
	for _, id := range loaded.GroupAdmins["backend"] {
		if id == "dev-id" {
			t.Error("expected dev-id removed from GroupAdmins[backend]")
		}
	}
}

func TestGroupAdminRemove_SelfRemoval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true) // dev-id is admin of backend
	injectGroupIdent(t, col, "dev-id", "dev")

	if err := runGroupAdminRemove(nil, []string{"acme", "backend", "dev"}); err != nil {
		t.Fatalf("runGroupAdminRemove (self-removal) = %v, want nil", err)
	}
}

func TestGroupAdminRemove_NonOwnerCannotRemoveOther(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// ops-id is NOT a group admin; dev-id IS the admin of backend.
	// ops tries to remove dev's admin rights — should fail.
	col := newGroupTestCol(t, true)
	injectGroupIdent(t, col, "ops-id", "ops")

	err := runGroupAdminRemove(nil, []string{"acme", "backend", "dev"})
	if err == nil {
		t.Fatal("expected error when non-owner tries to remove another user's group admin rights")
	}
}

func TestGroupAdminRemove_NotAnAdmin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true) // dev-id is admin of backend; ops is NOT
	injectGroupIdent(t, col, "cto-id", "cto")

	err := runGroupAdminRemove(nil, []string{"acme", "backend", "ops"})
	if err == nil {
		t.Fatal("expected error when target is not a group admin")
	}
}

// ── runGroupAdminList ────────────────────────────────────────────────────────

func TestGroupAdminList_Disabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, false) // org NOT enabled
	injectGroupIdent(t, col, "cto-id", "cto")

	if err := runGroupAdminList(nil, []string{"acme"}); err != nil {
		t.Fatalf("runGroupAdminList(disabled) = %v, want nil", err)
	}
}

func TestGroupAdminList_Enabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col := newGroupTestCol(t, true) // org enabled, dev-id is admin of backend
	injectGroupIdent(t, col, "cto-id", "cto")

	if err := runGroupAdminList(nil, []string{"acme"}); err != nil {
		t.Fatalf("runGroupAdminList(enabled) = %v, want nil", err)
	}
}

// TestAddOneToGroup exercises the per-username helper behind group add's
// multi-value support: a valid member succeeds, a non-member surfaces
// ErrNotMember as an error, and a sync failure surfaces an error without
// leaving the username in the group — covering the same three outcomes
// runGroupAdd loops over for a batch of usernames.
func TestAddOneToGroup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice", "bob", "charlie"}
	for _, login := range col.Members {
		col.Logins[login] = login
	}
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
