package cmd

import (
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// setupSyncConfigTest creates a collection with the given namespace and
// injects a mock client. Returns the collection and a cleanup func.
func setupSyncConfigTest(t *testing.T, collName, namespace string, mock *importMock) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cachedClient = mock
	cachedUser = "caller"
	cachedUserID = "caller-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	col, err := collection.New(collName, "github.com",
		api.UserInfo{ID: "caller-id", Login: "caller"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Namespace = namespace
	col.Members = []string{"alice-id"}
	col.Logins["alice-id"] = "alice"
	col.Repos = []collection.RepoAccess{{Name: "r", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
	return col
}

func TestSyncConfig_NoChanges(t *testing.T) {
	mock := newImportMock()
	// Platform state matches local collection exactly.
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	setupSyncConfigTest(t, "acme-corp-payments-team", "acme-corp", mock)

	old := syncConfigOrg
	syncConfigOrg = ""
	defer func() { syncConfigOrg = old }()

	err := runSyncConfig(nil, []string{"acme-corp-payments-team"})
	if err != nil {
		t.Fatalf("runSyncConfig no-change = %v", err)
	}
	// Collection on disk must not have been modified (save would bump UpdatedAt).
	col, _ := collection.Load("acme-corp-payments-team")
	if len(col.Members) != 1 || col.Members[0] != "alice-id" {
		t.Errorf("Members changed unexpectedly: %v", col.Members)
	}
}

func TestSyncConfig_NewMember(t *testing.T) {
	mock := newImportMock()
	// Platform has a new member: bob.
	mock.membersBySlug["payments-team"] = []api.UserInfo{
		{ID: "alice-id", Login: "alice"},
		{ID: "bob-id", Login: "bob"},
	}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	setupSyncConfigTest(t, "acme-corp-payments-team", "acme-corp", mock)

	old := syncConfigOrg
	syncConfigOrg = ""
	defer func() { syncConfigOrg = old }()

	if err := runSyncConfig(nil, []string{"acme-corp-payments-team"}); err != nil {
		t.Fatalf("runSyncConfig = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	foundBob := false
	for _, m := range col.Members {
		if m == "bob-id" {
			foundBob = true
		}
	}
	if !foundBob {
		t.Error("expected bob to be added to collection after sync")
	}
}

func TestSyncConfig_RemovedMember(t *testing.T) {
	mock := newImportMock()
	// Platform no longer has alice.
	mock.membersBySlug["payments-team"] = []api.UserInfo{}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	setupSyncConfigTest(t, "acme-corp-payments-team", "acme-corp", mock)

	old := syncConfigOrg
	syncConfigOrg = ""
	defer func() { syncConfigOrg = old }()

	if err := runSyncConfig(nil, []string{"acme-corp-payments-team"}); err != nil {
		t.Fatalf("runSyncConfig = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range col.Members {
		if m == "alice-id" {
			t.Error("expected alice to be removed after sync")
		}
	}
}

func TestSyncConfig_NewRepo(t *testing.T) {
	mock := newImportMock()
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	// Platform now has a new repo: r2.
	mock.reposBySlug["payments-team"] = []api.RepoInfo{
		{Name: "r", CloneURL: "u"},
		{Name: "r2", CloneURL: "u"},
	}
	setupSyncConfigTest(t, "acme-corp-payments-team", "acme-corp", mock)

	old := syncConfigOrg
	syncConfigOrg = ""
	defer func() { syncConfigOrg = old }()

	if err := runSyncConfig(nil, []string{"acme-corp-payments-team"}); err != nil {
		t.Fatalf("runSyncConfig = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	foundR2 := false
	for _, r := range col.Repos {
		if r.Name == "r2" {
			foundR2 = true
		}
	}
	if !foundR2 {
		t.Error("expected r2 to be added after sync")
	}
}

func TestSyncConfig_DryRun(t *testing.T) {
	mock := newImportMock()
	// Platform has a new member: bob.
	mock.membersBySlug["payments-team"] = []api.UserInfo{
		{ID: "alice-id", Login: "alice"},
		{ID: "bob-id", Login: "bob"},
	}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	setupSyncConfigTest(t, "acme-corp-payments-team", "acme-corp", mock)

	old := syncConfigOrg
	oldDry := syncConfigDryRun
	syncConfigOrg = ""
	syncConfigDryRun = true
	defer func() {
		syncConfigOrg = old
		syncConfigDryRun = oldDry
	}()

	if err := runSyncConfig(nil, []string{"acme-corp-payments-team"}); err != nil {
		t.Fatalf("runSyncConfig dry-run = %v", err)
	}

	// File must NOT have been modified (dry-run writes nothing).
	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range col.Members {
		if m == "bob-id" {
			t.Error("dry-run must not write bob to local collection")
		}
	}
}

func TestSyncConfig_NoNamespace(t *testing.T) {
	mock := newImportMock()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	cachedClient = mock
	cachedUser = "caller"
	cachedUserID = "caller-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	// Collection with no namespace set.
	col, _ := collection.New("standalone", "github.com",
		api.UserInfo{ID: "caller-id", Login: "caller"}, collection.VisibilityPrivate)
	// Namespace is deliberately empty.
	col.Save() //nolint:errcheck

	old := syncConfigOrg
	syncConfigOrg = "" // no --org flag either
	defer func() { syncConfigOrg = old }()

	err := runSyncConfig(nil, []string{"standalone"})
	// Must produce an error about missing namespace, not a panic or generic error.
	if err == nil {
		t.Fatal("expected error when collection has no namespace and --org is not set")
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{2 * time.Minute, "2 minutes ago"},
		{1 * time.Minute, "1 minute ago"},
		{3 * time.Hour, "3 hours ago"},
		{1 * time.Hour, "1 hour ago"},
		{48 * time.Hour, "2 days ago"},
		{24 * time.Hour, "1 day ago"},
	}
	for _, tc := range cases {
		if got := humanDuration(tc.d); got != tc.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// --- regression: applySync must not destroy per-repo access rules ---

// buildSyncCol returns a collection with a group, a group-restricted repo,
// an individually-granted repo, and two members — the shape whose access
// rules applySync used to discard.
func buildSyncCol(t *testing.T) *collection.Collection {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	col, err := collection.New("acme", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id", "bob-id"}
	col.Logins["alice-id"] = "alice"
	col.Logins["bob-id"] = "bob"
	col.Groups = map[string][]string{"backend": {"alice-id", "bob-id"}}
	col.Repos = []collection.RepoAccess{
		{Name: "api", Groups: []string{"backend"}, Users: []string{}},
		{Name: "web", Groups: []string{}, Users: []string{"bob-id"}},
	}
	return col
}

func repoByName(t *testing.T, col *collection.Collection, name string) collection.RepoAccess {
	t.Helper()
	for _, r := range col.Repos {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("repo %q not found in collection", name)
	return collection.RepoAccess{}
}

// The platform knows which repos exist; it knows nothing about gitcollect's
// groups. applySync previously rebuilt every RepoAccess with empty Groups
// and Users, silently reopening every restricted repo to all members.
func TestApplySync_PreservesRepoAccessRules(t *testing.T) {
	col := buildSyncCol(t)

	applySync(col,
		[]api.UserInfo{{ID: "alice-id", Login: "alice"}, {ID: "bob-id", Login: "bob"}},
		[]api.RepoInfo{{Name: "api"}, {Name: "web"}},
	)

	if got := repoByName(t, col, "api").Groups; len(got) != 1 || got[0] != "backend" {
		t.Errorf("api must stay restricted to the backend group, got %v", got)
	}
	if got := repoByName(t, col, "web").Users; len(got) != 1 || got[0] != "bob-id" {
		t.Errorf("web must keep its individual grant, got %v", got)
	}
}

func TestApplySync_AddsNewAndDropsRemovedRepos(t *testing.T) {
	col := buildSyncCol(t)

	applySync(col,
		[]api.UserInfo{{ID: "alice-id", Login: "alice"}, {ID: "bob-id", Login: "bob"}},
		[]api.RepoInfo{{Name: "api"}, {Name: "db"}}, // web gone, db new
	)

	names := map[string]bool{}
	for _, r := range col.Repos {
		names[r.Name] = true
	}
	if !names["db"] {
		t.Error("a repo new on the platform should be added")
	}
	if names["web"] {
		t.Error("a repo no longer on the platform should be dropped")
	}
	if got := repoByName(t, col, "db"); len(got.Groups) != 0 || len(got.Users) != 0 {
		t.Errorf("a newly discovered repo should start open to all members, got %+v", got)
	}
}

// A departing member has to be removed from groups, group admins and
// per-repo grants as well as Members: Validate requires every such
// reference to name a current member, so a stale one makes the very next
// Save fail.
func TestApplySync_PrunesDepartedMemberReferences(t *testing.T) {
	col := buildSyncCol(t)
	col.GroupAdminsEnabled = true
	col.GroupAdmins = map[string][]string{"backend": {"bob-id"}}

	applySync(col,
		[]api.UserInfo{{ID: "alice-id", Login: "alice"}}, // bob has left the team
		[]api.RepoInfo{{Name: "api"}, {Name: "web"}},
	)

	if err := col.Validate(); err != nil {
		t.Fatalf("collection must stay valid after a member leaves: %v", err)
	}
	for _, id := range col.Groups["backend"] {
		if id == "bob-id" {
			t.Error("departed member still listed in a group")
		}
	}
	for _, id := range repoByName(t, col, "web").Users {
		if id == "bob-id" {
			t.Error("departed member still holds an individual repo grant")
		}
	}
	if _, ok := col.GroupAdmins["backend"]; ok {
		t.Error("a group whose only admin left should not keep an empty admin list")
	}
}

// Groups are a structure the owner created deliberately; emptying one out
// is not a reason to delete it.
func TestApplySync_KeepsEmptiedGroups(t *testing.T) {
	col := buildSyncCol(t)

	applySync(col, []api.UserInfo{}, []api.RepoInfo{{Name: "api"}})

	if _, ok := col.Groups["backend"]; !ok {
		t.Error("group should survive even when all its members leave")
	}
	if len(col.Groups["backend"]) != 0 {
		t.Errorf("group should be empty, got %v", col.Groups["backend"])
	}
}

func TestApplySync_SavesCleanlyAfterMemberDeparture(t *testing.T) {
	col := buildSyncCol(t)
	if err := col.Save(); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	applySync(col, []api.UserInfo{{ID: "alice-id", Login: "alice"}}, []api.RepoInfo{{Name: "api"}})

	if err := col.Save(); err != nil {
		t.Fatalf("save after sync-config must succeed: %v", err)
	}
}
