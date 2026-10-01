package collection

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
)

// useTempHome points ~/.gitcollect at a fresh t.TempDir() for the duration
// of the test, on both Windows (USERPROFILE) and Unix (HOME), so tests
// never touch a real home directory.
func useTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// mockClient is an in-memory api.Client for exercising mutation.go without
// any network access. Collaborator state is tracked per "owner/repo/user".
type mockClient struct {
	host          string
	user          api.UserInfo
	mu            sync.Mutex
	collaborators map[string]bool
	failAdd       bool
	failRemove    bool
	failCheck     bool
}

func newMockClient() *mockClient {
	return &mockClient{host: "github.com", user: api.UserInfo{ID: "owner", Login: "owner"}, collaborators: map[string]bool{}}
}

func key(owner, repo, username string) string { return owner + "/" + repo + "/" + username }

func (m *mockClient) GetRepo(owner, repo string) (api.RepoInfo, error) {
	return api.RepoInfo{Name: repo, CloneURL: "https://example.com/" + owner + "/" + repo + ".git"}, nil
}
func (m *mockClient) GetAuthenticatedUser() (api.UserInfo, error) { return m.user, nil }
func (m *mockClient) GetUser(username string) (api.UserInfo, error) {
	return api.UserInfo{ID: username, Login: username}, nil
}
func (m *mockClient) AddCollaborator(owner, repo, username, permission string) error {
	if m.failAdd {
		return errors.New("mock add failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collaborators[key(owner, repo, username)] = true
	return nil
}
func (m *mockClient) RemoveCollaborator(owner, repo, username string) error {
	if m.failRemove {
		return errors.New("mock remove failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.collaborators, key(owner, repo, username))
	return nil
}
func (m *mockClient) CheckCollaborator(owner, repo, username string) (bool, error) {
	if m.failCheck {
		return false, errors.New("mock check failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.collaborators[key(owner, repo, username)], nil
}
func (m *mockClient) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *mockClient) GetPendingInvite(owner, repo, username string) (bool, error) {
	return false, nil
}
func (m *mockClient) CreateRepo(owner, name string, private bool, description string) (api.RepoInfo, error) {
	return api.RepoInfo{Name: name, CloneURL: "https://example.com/" + owner + "/" + name + ".git", Private: private}, nil
}
func (m *mockClient) Host() string                      { return m.host }
func (m *mockClient) GetTokenScopes() ([]string, error) { return []string{}, nil }
func (m *mockClient) ListOrgTeams(org string) ([]api.TeamInfo, error) {
	return nil, nil
}
func (m *mockClient) ListTeamMembers(org, teamSlug, role string) ([]api.UserInfo, error) {
	return nil, nil
}
func (m *mockClient) ListTeamRepos(org, teamSlug string) ([]api.RepoInfo, error) {
	return nil, nil
}
func (m *mockClient) SearchRepos(org, pattern, topic string, limit int) ([]api.RepoInfo, error) {
	return nil, nil
}
func (m *mockClient) ListOrgRepos(org string) ([]api.RepoInfo, error)   { return nil, nil }
func (m *mockClient) ListUserRepos(user string) ([]api.RepoInfo, error) { return nil, nil }
func (m *mockClient) GetAccountKind(login string) (api.AccountKind, error) {
	return api.AccountUser, nil
}
func (m *mockClient) ListOpenPRs(owner, repo string) ([]api.PRInfo, error) { return nil, nil }

func newTestCollection(t *testing.T, visibility Visibility) *Collection {
	t.Helper()
	col, err := New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, visibility)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, login := range []string{"alice", "bob", "charlie", "ghost", "not-a-member", "nobody"} {
		col.Logins[login] = login
	}
	return col
}

func TestValidateCollectionName(t *testing.T) {
	cases := map[string]bool{
		"acme":        true,
		"acme-1":      true,
		"a":           true,
		"":            false,
		"-acme":       false,
		"has space":   false,
		"has/slash":   false,
		"has\\slash":  false,
		"../traverse": false,
	}
	for name, want := range cases {
		err := ValidateCollectionName(name)
		if got := err == nil; got != want {
			t.Errorf("ValidateCollectionName(%q): got valid=%v, want %v (err=%v)", name, got, want, err)
		}
	}
}

func TestValidateRepoUserGroupNames(t *testing.T) {
	if err := ValidateRepoName("../etc/passwd"); err == nil {
		t.Error("expected path traversal repo name to be rejected")
	}
	if err := ValidateRepoName("valid-repo.go"); err != nil {
		t.Errorf("expected valid repo name to pass: %v", err)
	}
	if err := ValidateUsername("a"); err != nil {
		t.Errorf("expected single-char username to pass: %v", err)
	}
	if err := ValidateUsername("bad..name"); err == nil {
		t.Error("expected invalid username to be rejected")
	}
	if err := ValidateGroupName("red-team"); err != nil {
		t.Errorf("expected valid group name to pass: %v", err)
	}
	if err := ValidateGroupName(""); err == nil {
		t.Error("expected empty group name to be rejected")
	}
}

func TestNewSaveLoadExistsDelete(t *testing.T) {
	useTempHome(t)

	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}

	if exists, err := Exists("acme"); err != nil || exists {
		t.Fatalf("expected collection not to exist yet, exists=%v err=%v", exists, err)
	}

	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if exists, err := Exists("acme"); err != nil || !exists {
		t.Fatalf("expected collection to exist, exists=%v err=%v", exists, err)
	}

	loaded, err := Load("acme")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "acme" || loaded.Owner != "owner" || len(loaded.Members) != 1 || loaded.Members[0] != "alice" {
		t.Fatalf("loaded collection mismatch: %+v", loaded)
	}

	if err := loaded.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Load("acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestList(t *testing.T) {
	useTempHome(t)

	for _, name := range []string{"alpha", "beta"} {
		col, err := New(name, "github.com", api.UserInfo{ID: "owner", Login: "owner"}, VisibilityPublic)
		if err != nil {
			t.Fatalf("New(%s): %v", name, err)
		}
		if err := col.Save(); err != nil {
			t.Fatalf("Save(%s): %v", name, err)
		}
	}

	names, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["alpha"] || !found["beta"] {
		t.Fatalf("List() = %v, want both alpha and beta", names)
	}
}

func TestValidateStructuralIntegrity(t *testing.T) {
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Groups = map[string][]string{"red-team": {"bob"}} // bob is not a member

	if err := col.Validate(); err == nil {
		t.Error("expected validation error for group referencing non-member")
	}

	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []RepoAccess{{Name: "repo1", Groups: []string{"unknown-group"}}}
	if err := col.Validate(); err == nil {
		t.Error("expected validation error for repo referencing unknown group")
	}

	col.Repos = []RepoAccess{{Name: "repo1", Users: []string{"not-a-member"}}}
	if err := col.Validate(); err == nil {
		t.Error("expected validation error for repo referencing non-member user")
	}

	col.Repos = []RepoAccess{{Name: "repo1", Groups: []string{"red-team"}}}
	if err := col.Validate(); err != nil {
		t.Errorf("expected valid manifest to pass: %v", err)
	}
}

func TestAddMember(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Repos = []RepoAccess{{Name: "open-repo", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	if err := col.AddMember("alice", client); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if !col.IsMember("alice") {
		t.Error("expected alice to be a member after AddMember")
	}
	if has := client.collaborators[key("owner", "open-repo", "alice")]; !has {
		t.Error("expected AddMember to grant collaborator access to the open repo")
	}

	// Idempotent: adding again is a no-op, not an error.
	if err := col.AddMember("alice", client); err != nil {
		t.Fatalf("AddMember (idempotent): %v", err)
	}
	count := 0
	for _, m := range col.Members {
		if m == "alice" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected alice to appear exactly once in Members, got %d", count)
	}
}

func TestAddMember_SyncFailureRollsBack(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Repos = []RepoAccess{{Name: "open-repo", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	client.failAdd = true

	before := append([]string{}, col.Members...)
	if err := col.AddMember("alice", client); err == nil {
		t.Fatal("expected AddMember to fail when sync fails")
	}
	if len(col.Members) != len(before) {
		t.Fatalf("expected Members to be rolled back on sync failure, got %v", col.Members)
	}
}

func TestRemoveMember(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice", "bob"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []RepoAccess{{Name: "repo1", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	client.collaborators[key("owner", "repo1", "alice")] = true

	if err := col.RemoveMember("alice", client); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if col.IsMember("alice") {
		t.Error("expected alice to no longer be a member")
	}
	if col.IsInGroup("alice", "red-team") {
		t.Error("expected alice to be removed from all groups")
	}
	if client.collaborators[key("owner", "repo1", "alice")] {
		t.Error("expected RemoveMember to revoke platform collaborator access")
	}

	// No-op for a non-member.
	if err := col.RemoveMember("charlie", client); err != nil {
		t.Fatalf("RemoveMember (non-member, no-op): %v", err)
	}
}

func TestAddToGroupAndRemoveFromGroup(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Groups = map[string][]string{"red-team": {}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()

	if err := col.AddToGroup("nobody", "red-team", client); !errors.Is(err, ErrNotMember) {
		t.Fatalf("expected ErrNotMember, got %v", err)
	}
	if err := col.AddToGroup("alice", "no-such-group", client); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}

	if err := col.AddToGroup("alice", "red-team", client); err != nil {
		t.Fatalf("AddToGroup: %v", err)
	}
	if !col.IsInGroup("alice", "red-team") {
		t.Error("expected alice to be in red-team after AddToGroup")
	}

	if err := col.RemoveFromGroup("alice", "red-team", client); err != nil {
		t.Fatalf("RemoveFromGroup: %v", err)
	}
	if col.IsInGroup("alice", "red-team") {
		t.Error("expected alice to be removed from red-team")
	}
}

func TestAddToGroup_Idempotent(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	if err := col.AddToGroup("alice", "red-team", client); err != nil {
		t.Fatalf("AddToGroup (already in group, no-op): %v", err)
	}
	if len(col.Groups["red-team"]) != 1 {
		t.Fatalf("expected alice to appear exactly once, got %v", col.Groups["red-team"])
	}
}

func TestAddToGroup_SyncFailureRollsBack(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Groups = map[string][]string{"red-team": {}}
	col.Repos = []RepoAccess{{Name: "r", Groups: []string{"red-team"}}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	client.failAdd = true

	if err := col.AddToGroup("alice", "red-team", client); err == nil {
		t.Fatal("expected AddToGroup to fail when sync fails")
	}
	if col.IsInGroup("alice", "red-team") {
		t.Fatal("expected group membership to be rolled back on sync failure")
	}
}

func TestRemoveFromGroup_NotFoundOrNoop(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Groups = map[string][]string{"red-team": {}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	if err := col.RemoveFromGroup("alice", "no-such-group", client); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
	// alice isn't in red-team: no-op, not an error.
	if err := col.RemoveFromGroup("alice", "red-team", client); err != nil {
		t.Fatalf("RemoveFromGroup (no-op): %v", err)
	}
}

func TestRemoveFromGroup_SyncFailureRollsBack(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	client.failRemove = true
	client.collaborators[key("owner", "r", "alice")] = true
	col.Repos = []RepoAccess{{Name: "r", Groups: []string{"red-team"}}}

	if err := col.RemoveFromGroup("alice", "red-team", client); err == nil {
		t.Fatal("expected RemoveFromGroup to fail when sync fails")
	}
	if !col.IsInGroup("alice", "red-team") {
		t.Fatal("expected group membership to be rolled back on sync failure")
	}
}

func TestSetRepoAccess_RejectsInvalidUser(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Repos = []RepoAccess{{Name: "repo1"}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	if err := col.SetRepoAccess("repo1", nil, []string{"not-a-member"}, client); !errors.Is(err, ErrNotMember) {
		t.Fatalf("expected ErrNotMember, got %v", err)
	}
}

func TestSetRepoAccess_SyncFailureRollsBack(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Repos = []RepoAccess{{Name: "repo1", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()
	client.failRemove = true
	client.collaborators[key("owner", "repo1", "alice")] = true

	before := col.Repos[0]
	if err := col.SetRepoAccess("repo1", []string{}, []string{}, client); err != nil {
		t.Fatalf("no-op restriction shouldn't need any platform change: %v", err)
	}
	col.Groups = map[string][]string{"red-team": {}}
	if err := col.SetRepoAccess("repo1", []string{"red-team"}, nil, client); err == nil {
		t.Fatal("expected SetRepoAccess to fail when revoking alice's now-unauthorized access fails")
	}
	if col.Repos[0].Groups != nil && len(col.Repos[0].Groups) != len(before.Groups) {
		t.Fatalf("expected repo access to be rolled back on sync failure, got %+v", col.Repos[0])
	}
}

func TestSyncCollaborators_CheckAndRemoveFailures(t *testing.T) {
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	col.Repos = []RepoAccess{{Name: "r", Groups: []string{}, Users: []string{}}}

	checkFail := newMockClient()
	checkFail.failCheck = true
	if _, _, err := col.SyncCollaborators(checkFail, nil); err == nil {
		t.Fatal("expected SyncCollaborators to surface CheckCollaborator failures")
	}

	removeFail := newMockClient()
	removeFail.failRemove = true
	removeFail.collaborators[key("owner", "r", "alice")] = true
	col2 := newTestCollection(t, VisibilityPrivate)
	col2.Members = []string{"alice"}
	col2.Groups = map[string][]string{"red-team": {}} // alice is a member but not in red-team
	col2.Repos = []RepoAccess{{Name: "r", Groups: []string{"red-team"}}}
	if _, _, err := col2.SyncCollaborators(removeFail, nil); err == nil {
		t.Fatal("expected SyncCollaborators to surface RemoveCollaborator failures")
	}
}

func TestCanAccessRepo_UnknownRepo(t *testing.T) {
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	if col.CanAccessRepo("alice", "no-such-repo") {
		t.Error("expected access to an unknown repo to be denied")
	}
}

func TestSetRepoAccess(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice", "bob"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []RepoAccess{{Name: "repo1", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	client := newMockClient()

	if err := col.SetRepoAccess("no-such-repo", nil, nil, client); !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("expected ErrRepoNotFound, got %v", err)
	}
	if err := col.SetRepoAccess("repo1", []string{"no-such-group"}, nil, client); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}

	if err := col.SetRepoAccess("repo1", []string{"red-team"}, nil, client); err != nil {
		t.Fatalf("SetRepoAccess: %v", err)
	}
	if !col.CanAccessRepo("alice", "repo1") {
		t.Error("expected alice (in red-team) to access repo1")
	}
	if col.CanAccessRepo("bob", "repo1") {
		t.Error("expected bob (not in red-team) to be denied repo1")
	}
}

func TestAccessibleRepos(t *testing.T) {
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice", "bob"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []RepoAccess{
		{Name: "open", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	got := col.AccessibleRepos("bob")
	if len(got) != 1 || got[0].Name != "open" {
		t.Fatalf("expected bob to access only 'open', got %v", got)
	}

	got = col.AccessibleRepos("alice")
	if len(got) != 2 {
		t.Fatalf("expected alice (red-team) to access both repos, got %v", got)
	}
}

func TestWhyCanAccess(t *testing.T) {
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice", "bob", "charlie"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []RepoAccess{
		{Name: "open", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}, Users: []string{"bob"}},
	}

	cases := []struct {
		username, repo, want string
	}{
		{"owner", "open", "owner — full access"},
		{"alice", "open", "open to all members"},
		{"alice", "restricted", "member of group red-team"},
		{"bob", "restricted", "individually granted"},
		{"charlie", "restricted", "no access — group red-team or individual grant required"},
		{"stranger", "open", "no access — not a member"},
		{"alice", "no-such-repo", "no access — repo not in collection"},
	}
	for _, tc := range cases {
		if got := col.WhyCanAccess(tc.username, tc.repo); got != tc.want {
			t.Errorf("WhyCanAccess(%q, %q) = %q, want %q", tc.username, tc.repo, got, tc.want)
		}
	}

	// Visibility decides who the collection admits; it never disables a
	// repo's own restrictions. This block previously asserted the opposite
	// — that a public collection granted access to "anything", a repo not
	// even in it — which is how per-repo rules came to be silently inert on
	// every public collection.
	pub := newTestCollection(t, VisibilityPublic)
	pub.Members = []string{"alice", "bob", "charlie"}
	pub.Groups = map[string][]string{"red-team": {"alice"}}
	pub.Repos = []RepoAccess{
		{Name: "open", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}, Users: []string{"bob"}},
	}
	pubCases := []struct{ username, repo, want string }{
		{"anyone", "anything", "no access — repo not in collection"},
		{"anyone", "open", "open to everyone — public collection"},
		{"charlie", "restricted", "no access — group red-team or individual grant required"},
		{"alice", "restricted", "member of group red-team"},
		{"bob", "restricted", "individually granted"},
	}
	for _, tc := range pubCases {
		if got := pub.WhyCanAccess(tc.username, tc.repo); got != tc.want {
			t.Errorf("public WhyCanAccess(%q, %q) = %q, want %q", tc.username, tc.repo, got, tc.want)
		}
	}

	onlyUsers := newTestCollection(t, VisibilityPrivate)
	onlyUsers.Members = []string{"alice"}
	onlyUsers.Repos = []RepoAccess{{Name: "r", Users: []string{"bob"}}}
	if got := onlyUsers.WhyCanAccess("alice", "r"); got != "no access — individual grant required" {
		t.Errorf("got %q, want %q", got, "no access — individual grant required")
	}
}

func TestNew_InvalidName(t *testing.T) {
	if _, err := New("", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, VisibilityPrivate); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName for an empty collection name, got %v", err)
	}
}

func TestSave_RejectsInvalidManifest(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Groups = map[string][]string{"red-team": {"ghost"}} // ghost is not a member

	if err := col.Save(); err == nil {
		t.Fatal("expected Save to reject a structurally invalid manifest")
	}
}

func TestDelete_UnresolvedPath(t *testing.T) {
	useTempHome(t)
	// A Collection built directly (not via New) has an empty path, exercising
	// Delete's own manifestPath resolution. Removing a manifest that was
	// never saved is not an error.
	col := &Collection{Name: "ghost-collection"}
	if err := col.Delete(); err != nil {
		t.Fatalf("Delete on a never-saved collection: %v", err)
	}
}

func TestCreateAndDeleteGroup(t *testing.T) {
	useTempHome(t)
	col := newTestCollection(t, VisibilityPrivate)
	col.Members = []string{"alice"}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := col.CreateGroup("red-team"); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := col.CreateGroup("red-team"); !errors.Is(err, ErrGroupExists) {
		t.Fatalf("expected ErrGroupExists, got %v", err)
	}

	col.Repos = []RepoAccess{{Name: "repo1", Groups: []string{"red-team"}, Users: []string{}}}
	if err := col.DeleteGroup("red-team"); !errors.Is(err, ErrGroupInUse) {
		t.Fatalf("expected ErrGroupInUse while a repo still references the group, got %v", err)
	}

	col.Repos = nil
	if err := col.DeleteGroup("red-team"); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if _, ok := col.Groups["red-team"]; ok {
		t.Error("expected red-team to be removed")
	}
}

func TestRepoNamespace_DefaultsToOwnerLogin(t *testing.T) {
	col := &Collection{
		Owner:  "owner-id",
		Logins: map[string]string{"owner-id": "alice"},
	}
	if got := col.RepoNamespace(); got != "alice" {
		t.Errorf("RepoNamespace() = %q, want alice (owner login fallback)", got)
	}
}

func TestRepoNamespace_UsesExplicitNamespace(t *testing.T) {
	col := &Collection{
		Owner:     "owner-id",
		Logins:    map[string]string{"owner-id": "alice"},
		Namespace: "acme-corp",
	}
	if got := col.RepoNamespace(); got != "acme-corp" {
		t.Errorf("RepoNamespace() = %q, want acme-corp (explicit namespace)", got)
	}
}

// A fix command needs a login to be typeable. Reading a public collection
// without authenticating has no identity, and every suggestion was being
// emitted with its username argument missing.
func TestFixCmd_NoLoginNoSuggestion(t *testing.T) {
	col := newTestCollection(t, VisibilityPublic)
	col.Repos = []RepoAccess{{Name: "r", Groups: []string{"g"}, Users: []string{}}}
	col.Groups = map[string][]string{"g": {}}

	if got := col.FixCmd("", "", "r"); got != "" {
		t.Errorf("FixCmd with no login = %q, want empty", got)
	}
	if got := col.FixCmd("someone", "someone", "r"); got == "" {
		t.Error("FixCmd with a login should still suggest a command")
	}
}

// --- access strategy ---

// Every manifest written before team support has no access_strategy field.
// Those collections must keep granting exactly as they did.
func TestStrategy_DefaultsToCollaborator(t *testing.T) {
	c := &Collection{}
	if got := c.Strategy(); got != StrategyCollaborator {
		t.Errorf("an absent strategy must mean %q, got %q", StrategyCollaborator, got)
	}
	c.AccessStrategy = StrategyCollaborator
	if got := c.Strategy(); got != StrategyCollaborator {
		t.Errorf("got %q", got)
	}
	c.AccessStrategy = StrategyTeam
	if got := c.Strategy(); got != StrategyTeam {
		t.Errorf("got %q", got)
	}
	// An unrecognised value is not treated as team access. Granting through
	// a mechanism nobody asked for is worse than ignoring a typo.
	c.AccessStrategy = "teams-ish"
	if got := c.Strategy(); got != StrategyCollaborator {
		t.Errorf("an unknown strategy must not enable teams, got %q", got)
	}
}

// teamKindMock answers GetAccountKind with whatever the test needs, and can
// be made to implement api.TeamGranter or not.
type teamKindMock struct {
	mockClient
	kind api.AccountKind
}

func (m *teamKindMock) GetAccountKind(string) (api.AccountKind, error) { return m.kind, nil }

type grantingMock struct {
	teamKindMock
}

func (m *grantingMock) CreateTeam(string, string, string) (api.TeamInfo, error) {
	return api.TeamInfo{}, nil
}
func (m *grantingMock) SetTeamRepoPermission(_, _, _, _, _ string) error { return nil }
func (m *grantingMock) RemoveTeamRepo(_, _, _, _ string) error           { return nil }
func (m *grantingMock) ListRepoTeams(_, _ string) ([]api.TeamAccess, error) {
	return nil, nil
}
func (m *grantingMock) SetTeamMembership(_, _, _, _ string) (api.TeamMembership, error) {
	return api.TeamMembership{}, nil
}
func (m *grantingMock) RemoveTeamMembership(_, _, _ string) error { return nil }
func (m *grantingMock) ListTeamMemberships(_, _ string) ([]api.TeamMembership, error) {
	return nil, nil
}

func TestCheckTeamSupport(t *testing.T) {
	// A collaborator collection never consults the platform at all.
	t.Run("collaborator strategy needs no support", func(t *testing.T) {
		c := &Collection{Host: "github.com"}
		if err := c.CheckTeamSupport(&teamKindMock{kind: api.AccountUser}); err != nil {
			t.Errorf("collaborator access must not require teams: %v", err)
		}
	})

	t.Run("organisation with a team-capable client is allowed", func(t *testing.T) {
		c := &Collection{Host: "github.com", AccessStrategy: StrategyTeam, Namespace: "acme"}
		if err := c.CheckTeamSupport(&grantingMock{teamKindMock{kind: api.AccountOrg}}); err != nil {
			t.Errorf("an org on a team-capable client should be allowed: %v", err)
		}
	})

	// Teams do not exist on a personal account, so this must refuse rather
	// than quietly grant one collaborator call per member per repo.
	t.Run("personal namespace is refused, not downgraded", func(t *testing.T) {
		c := &Collection{Host: "github.com", AccessStrategy: StrategyTeam, Namespace: "jsmith"}
		err := c.CheckTeamSupport(&grantingMock{teamKindMock{kind: api.AccountUser}})
		if !errors.Is(err, ErrTeamsUnsupported) {
			t.Fatalf("expected ErrTeamsUnsupported, got %v", err)
		}
		if !strings.Contains(err.Error(), StrategyCollaborator) {
			t.Errorf("the error should say how to fix it, got: %v", err)
		}
	})

	// A platform whose client cannot grant teams is refused before the
	// account kind is even considered.
	t.Run("client without team support is refused", func(t *testing.T) {
		c := &Collection{Host: "gitlab.com", AccessStrategy: StrategyTeam, Namespace: "acme"}
		err := c.CheckTeamSupport(&teamKindMock{kind: api.AccountOrg})
		if !errors.Is(err, ErrTeamsUnsupported) {
			t.Fatalf("expected ErrTeamsUnsupported, got %v", err)
		}
	})
}

func TestTeamBinding(t *testing.T) {
	c := &Collection{}
	if _, ok := c.TeamSlugFor("pricing"); ok {
		t.Error("an unbound group must report no binding")
	}

	c.BindTeam("pricing", "acme-pricing")
	slug, ok := c.TeamSlugFor("pricing")
	if !ok || slug != "acme-pricing" {
		t.Errorf("got %q/%v, want acme-pricing/true", slug, ok)
	}

	// An empty binding is no binding: it would otherwise be used as a team
	// slug and address the wrong URL.
	c.BindTeam("search", "")
	if _, ok := c.TeamSlugFor("search"); ok {
		t.Error("an empty slug must not count as bound")
	}
}

// --- team sync ---

// recordingGranter records every team call so a test can assert on what was
// actually asked of the platform, not just the outcome.
type recordingGranter struct {
	teamKindMock

	created   []string
	granted   []string // "slug:repo"
	revoked   []string
	addedMem  []string // "slug:login"
	removeMem []string

	repoTeams map[string][]api.TeamAccess
	members   map[string][]api.TeamMembership
	slugFor   func(name string) string
}

func newRecordingGranter() *recordingGranter {
	return &recordingGranter{
		teamKindMock: teamKindMock{kind: api.AccountOrg},
		repoTeams:    map[string][]api.TeamAccess{},
		members:      map[string][]api.TeamMembership{},
	}
}

func (m *recordingGranter) CreateTeam(_, name, _ string) (api.TeamInfo, error) {
	m.created = append(m.created, name)
	slug := name
	if m.slugFor != nil {
		slug = m.slugFor(name)
	}
	return api.TeamInfo{Name: name, Slug: slug}, nil
}
func (m *recordingGranter) SetTeamRepoPermission(_, slug, _, repo, _ string) error {
	m.granted = append(m.granted, slug+":"+repo)
	return nil
}
func (m *recordingGranter) RemoveTeamRepo(_, slug, _, repo string) error {
	m.revoked = append(m.revoked, slug+":"+repo)
	return nil
}
func (m *recordingGranter) ListRepoTeams(_, repo string) ([]api.TeamAccess, error) {
	return m.repoTeams[repo], nil
}
func (m *recordingGranter) SetTeamMembership(_, slug, login, _ string) (api.TeamMembership, error) {
	m.addedMem = append(m.addedMem, slug+":"+login)
	return api.TeamMembership{Login: login, Role: "member", State: "active"}, nil
}
func (m *recordingGranter) RemoveTeamMembership(_, slug, login string) error {
	m.removeMem = append(m.removeMem, slug+":"+login)
	return nil
}
func (m *recordingGranter) ListTeamMemberships(_, slug string) ([]api.TeamMembership, error) {
	return m.members[slug], nil
}

func teamCollection() *Collection {
	return &Collection{
		Name: "platform", Host: "github.com", Namespace: "acme",
		AccessStrategy: StrategyTeam,
		Members:        []string{"alice-id", "bob-id"},
		Logins:         map[string]string{"alice-id": "alice", "bob-id": "bob"},
		Groups:         map[string][]string{"pricing": {"alice-id"}},
		Repos: []RepoAccess{
			{Name: "eu-pricing", Groups: []string{"pricing"}},
			{Name: "us-pricing", Groups: []string{"pricing"}},
			{Name: "unrelated", Groups: []string{}},
		},
	}
}

// The whole point: a group's repos are granted once to the team, and the
// member is added once — not once per repo.
func TestSyncViaTeams_GrantsTeamOncePerRepoNotPerMember(t *testing.T) {
	c := teamCollection()
	c.Groups["pricing"] = []string{"alice-id", "bob-id"}
	g := newRecordingGranter()

	if _, _, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	}

	if len(g.granted) != 2 {
		t.Errorf("expected one grant per repo in the group, got %v", g.granted)
	}
	// Two members across two repos would be four collaborator calls; here
	// membership costs one call each regardless of repo count.
	if len(g.addedMem) != 2 {
		t.Errorf("expected one membership call per member, got %v", g.addedMem)
	}
}

// The slug is the platform's to decide, so it must be read back and stored,
// not derived from the group name.
func TestSyncViaTeams_StoresThePlatformSlug(t *testing.T) {
	c := teamCollection()
	g := newRecordingGranter()
	g.slugFor = func(name string) string { return "acme-" + name + "-7" }

	if _, _, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	}

	slug, ok := c.TeamSlugFor("pricing")
	if !ok || slug != "acme-pricing-7" {
		t.Fatalf("slug = %q/%v, want the platform's acme-pricing-7", slug, ok)
	}
	for _, g := range g.granted {
		if !strings.HasPrefix(g, "acme-pricing-7:") {
			t.Errorf("grants must use the platform slug, got %q", g)
		}
	}
}

// A second sync must not create the team again.
func TestSyncViaTeams_ReusesAnExistingBinding(t *testing.T) {
	c := teamCollection()
	c.BindTeam("pricing", "already-there")
	g := newRecordingGranter()

	if _, _, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	}
	if len(g.created) != 0 {
		t.Errorf("a bound group must not be recreated, created %v", g.created)
	}
	if len(g.granted) == 0 || !strings.HasPrefix(g.granted[0], "already-there:") {
		t.Errorf("should grant through the bound slug, got %v", g.granted)
	}
}

// Access the team holds but the manifest no longer grants is revoked.
func TestSyncViaTeams_RevokesAccessTheManifestDropped(t *testing.T) {
	c := teamCollection()
	c.BindTeam("pricing", "pricing")
	g := newRecordingGranter()
	// The team still holds "unrelated", which no group grants any more.
	g.repoTeams["unrelated"] = []api.TeamAccess{{Slug: "pricing", Direct: true}}

	if _, removed, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	} else if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if len(g.revoked) != 1 || g.revoked[0] != "pricing:unrelated" {
		t.Errorf("revoked = %v, want pricing:unrelated", g.revoked)
	}
}

// Inherited access is reported by the platform but does not live on this
// repo. Revoking it would not remove it, and would change what the parent
// team reaches.
func TestSyncViaTeams_NeverRevokesInheritedAccess(t *testing.T) {
	c := teamCollection()
	c.BindTeam("pricing", "pricing")
	g := newRecordingGranter()
	g.repoTeams["unrelated"] = []api.TeamAccess{
		{Slug: "pricing", Direct: false, ParentSlug: "platform"},
	}

	if _, removed, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	} else if removed != 0 {
		t.Errorf("removed = %d, want 0 — inherited access is not ours", removed)
	}
	if len(g.revoked) != 0 {
		t.Errorf("must not revoke inherited access, revoked %v", g.revoked)
	}
}

// Same rule for people: a member held through a parent team cannot be
// removed from the child.
func TestSyncViaTeams_NeverRemovesInheritedMembers(t *testing.T) {
	c := teamCollection()
	c.BindTeam("pricing", "pricing")
	g := newRecordingGranter()
	g.members["pricing"] = []api.TeamMembership{
		{Login: "alice", Inherited: false}, // wanted, stays
		{Login: "carol", Inherited: true},  // not wanted, but not ours
		{Login: "dave", Inherited: false},  // not wanted, and ours to remove
	}

	if _, _, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	}
	if len(g.removeMem) != 1 || g.removeMem[0] != "pricing:dave" {
		t.Errorf("removeMem = %v, want only pricing:dave", g.removeMem)
	}
}

// A repo outside the collection is none of gitcollect's business, even if
// the team reaches it.
func TestSyncViaTeams_IgnoresReposOutsideTheCollection(t *testing.T) {
	c := teamCollection()
	c.BindTeam("pricing", "pricing")
	g := newRecordingGranter()
	g.repoTeams["someone-elses-repo"] = []api.TeamAccess{{Slug: "pricing", Direct: true}}

	if _, _, err := c.SyncViaTeams(g, nil); err != nil {
		t.Fatalf("SyncViaTeams: %v", err)
	}
	for _, r := range g.revoked {
		if strings.Contains(r, "someone-elses-repo") {
			t.Error("must not touch repos the collection does not list")
		}
	}
}

func TestSyncViaTeams_RefusedWithoutTeamSupport(t *testing.T) {
	c := teamCollection()
	_, _, err := c.SyncViaTeams(&teamKindMock{kind: api.AccountOrg}, nil)
	if !errors.Is(err, ErrTeamsUnsupported) {
		t.Errorf("expected ErrTeamsUnsupported, got %v", err)
	}
}
