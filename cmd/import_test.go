package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
)

// importMock is a minimal api.Client for import tests. All three new list
// methods are controllable; everything else is a fixed stub.
type importMock struct {
	host        string
	teams       []api.TeamInfo
	membersBySlug  map[string][]api.UserInfo // slug → all members
	maintBySlug    map[string][]api.UserInfo // slug → maintainers
	reposBySlug    map[string][]api.RepoInfo // slug → repos
	scopeErr    error
	scopes      []string
	teamsErr    error
}

func newImportMock() *importMock {
	return &importMock{
		host:        "github.com",
		membersBySlug:  map[string][]api.UserInfo{},
		maintBySlug:    map[string][]api.UserInfo{},
		reposBySlug:    map[string][]api.RepoInfo{},
		scopes:      []string{"read:org", "repo"},
	}
}

func (m *importMock) Host() string { return m.host }

func (m *importMock) GetTokenScopes() ([]string, error) {
	if m.scopeErr != nil {
		return nil, m.scopeErr
	}
	return m.scopes, nil
}

func (m *importMock) ListOrgTeams(org string) ([]api.TeamInfo, error) {
	if m.teamsErr != nil {
		return nil, m.teamsErr
	}
	return m.teams, nil
}

func (m *importMock) ListTeamMembers(org, teamSlug, role string) ([]api.UserInfo, error) {
	if role == "maintainer" {
		return m.maintBySlug[teamSlug], nil
	}
	return m.membersBySlug[teamSlug], nil
}

func (m *importMock) ListTeamRepos(org, teamSlug string) ([]api.RepoInfo, error) {
	return m.reposBySlug[teamSlug], nil
}
func (m *importMock) SearchRepos(org, pattern, topic string, limit int) ([]api.RepoInfo, error) {
	return nil, nil
}
func (m *importMock) ListOrgRepos(org string) ([]api.RepoInfo, error) { return nil, nil }
func (m *importMock) ListOpenPRs(owner, repo string) ([]api.PRInfo, error) { return nil, nil }

// Stubs for the rest of the Client interface.
func (m *importMock) GetAuthenticatedUser() (api.UserInfo, error) {
	return api.UserInfo{ID: "caller-id", Login: "caller"}, nil
}
func (m *importMock) GetUser(username string) (api.UserInfo, error) {
	return api.UserInfo{ID: username + "-id", Login: username}, nil
}
func (m *importMock) GetRepo(owner, repo string) (api.RepoInfo, error) { return api.RepoInfo{}, nil }
func (m *importMock) AddCollaborator(owner, repo, username, permission string) error { return nil }
func (m *importMock) RemoveCollaborator(owner, repo, username string) error          { return nil }
func (m *importMock) CheckCollaborator(owner, repo, username string) (bool, error)   { return false, nil }
func (m *importMock) GetPendingInvite(owner, repo, username string) (bool, error)    { return false, nil }
func (m *importMock) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *importMock) CreateRepo(owner, name string, private bool, description string) (api.RepoInfo, error) {
	return api.RepoInfo{}, nil
}

// setupImportTest prepares the test environment: sets HOME/USERPROFILE to a
// temp dir, injects the mock client, and resets all import flag variables and
// the caller identity cache. The returned cleanup func must be deferred.
func setupImportTest(t *testing.T, mock *importMock) func() {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cachedClient = mock
	cachedUser = "caller"
	cachedUserID = "caller-id"

	// Reset flag variables to their defaults before each test.
	importFrom = "github"
	importOrg = "acme-corp"
	importTeam = ""
	importDryRun = false
	importFlatten = true
	importOwnerFromMaintainer = true
	importNamespace = ""
	importMerge = false
	importOverwrite = false
	importSkipExisting = false

	return func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
		importFrom = ""
		importOrg = ""
		importTeam = ""
		importDryRun = false
		importFlatten = true
		importOwnerFromMaintainer = true
		importNamespace = ""
		importMerge = false
		importOverwrite = false
		importSkipExisting = false
	}
}

func TestImport_DryRun_NoFilesWritten(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "payments-api", CloneURL: "https://github.com/acme-corp/payments-api.git"}}
	defer setupImportTest(t, mock)()

	importDryRun = true

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport dry-run = %v, want nil", err)
	}

	// No collection files must have been written.
	collDir, _ := config.CollectionsDir()
	entries, _ := os.ReadDir(collDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yaml") {
			t.Errorf("dry-run wrote file %s", e.Name())
		}
	}
}

func TestImport_CreatesCollectionFiles(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{
		{ID: 1, Slug: "payments-team", Name: "Payments"},
		{ID: 2, Slug: "mobile-team", Name: "Mobile"},
	}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	mock.membersBySlug["mobile-team"] = []api.UserInfo{{ID: "bob-id", Login: "bob"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "payments-api", CloneURL: "u"}}
	mock.reposBySlug["mobile-team"] = []api.RepoInfo{{Name: "mobile-app", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	collDir, _ := config.CollectionsDir()
	for _, name := range []string{"acme-corp-payments-team", "acme-corp-mobile-team"} {
		if _, err := os.Stat(filepath.Join(collDir, name+".yaml")); err != nil {
			t.Errorf("expected collection file %s.yaml to exist: %v", name, err)
		}
	}
}

func TestImport_SetsOwnerFromMaintainer(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	mock.membersBySlug["payments-team"] = []api.UserInfo{
		{ID: "alice-id", Login: "alice"},
		{ID: "bob-id", Login: "bob"},
	}
	mock.maintBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	importOwnerFromMaintainer = true

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if col.Owner != "alice-id" {
		t.Errorf("Owner = %q, want alice-id (first maintainer)", col.Owner)
	}
}

func TestImport_FallsBackToCallerOwner(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	// No maintainers.
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	importOwnerFromMaintainer = true

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// No maintainers → falls back to the authenticated caller.
	if col.Owner != "caller-id" {
		t.Errorf("Owner = %q, want caller-id (importer)", col.Owner)
	}
}

func TestImport_SingleTeam(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{
		{ID: 1, Slug: "payments-team", Name: "Payments"},
		{ID: 2, Slug: "mobile-team", Name: "Mobile"},
	}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "alice-id", Login: "alice"}}
	mock.membersBySlug["mobile-team"] = []api.UserInfo{{ID: "bob-id", Login: "bob"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r", CloneURL: "u"}}
	mock.reposBySlug["mobile-team"] = []api.RepoInfo{{Name: "r2", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	importTeam = "payments-team"

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	collDir, _ := config.CollectionsDir()

	// payments-team should exist.
	if _, err := os.Stat(filepath.Join(collDir, "acme-corp-payments-team.yaml")); err != nil {
		t.Errorf("expected payments-team collection to exist: %v", err)
	}
	// mobile-team must NOT have been written.
	if _, err := os.Stat(filepath.Join(collDir, "acme-corp-mobile-team.yaml")); err == nil {
		t.Error("expected mobile-team collection NOT to exist when --team=payments-team")
	}
}

func TestImport_MissingScope_ReturnsError(t *testing.T) {
	mock := newImportMock()
	mock.scopes = []string{"public_repo"} // missing read:org
	defer setupImportTest(t, mock)()

	importFrom = "github"

	err := runImport(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing read:org scope, got nil")
	}
	if !strings.Contains(err.Error(), "read:org") {
		t.Errorf("expected error to mention 'read:org', got: %v", err)
	}
}

func TestImport_ConflictMerge(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	// Import has a new member (bob) and a new repo (r2).
	mock.membersBySlug["payments-team"] = []api.UserInfo{
		{ID: "alice-id", Login: "alice"},
		{ID: "bob-id", Login: "bob"},
	}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{
		{Name: "r", CloneURL: "u"},
		{Name: "r2", CloneURL: "u"},
	}
	defer setupImportTest(t, mock)()

	// Pre-create an existing collection with only alice and repo r.
	existing, _ := collection.New("acme-corp-payments-team", "github.com",
		api.UserInfo{ID: "caller-id", Login: "caller"}, collection.VisibilityPrivate)
	existing.Members = []string{"alice-id"}
	existing.Logins["alice-id"] = "alice"
	existing.Repos = []collection.RepoAccess{{Name: "r", Groups: []string{}, Users: []string{}}}
	if err := existing.Save(); err != nil {
		t.Fatalf("pre-create existing collection: %v", err)
	}

	importMerge = true

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load after merge: %v", err)
	}
	// alice must still be present (kept from existing).
	foundAlice := false
	for _, m := range col.Members {
		if m == "alice-id" {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Error("expected alice to remain after merge")
	}
	// bob must have been added.
	foundBob := false
	for _, m := range col.Members {
		if m == "bob-id" {
			foundBob = true
		}
	}
	if !foundBob {
		t.Error("expected bob to be added by merge")
	}
	// r2 must have been added.
	foundR2 := false
	for _, r := range col.Repos {
		if r.Name == "r2" {
			foundR2 = true
		}
	}
	if !foundR2 {
		t.Error("expected r2 to be added by merge")
	}
}

func TestImport_ConflictOverwrite(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "bob-id", Login: "bob"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r2", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	// Pre-create an existing collection with different contents.
	existing, _ := collection.New("acme-corp-payments-team", "github.com",
		api.UserInfo{ID: "caller-id", Login: "caller"}, collection.VisibilityPrivate)
	existing.Members = []string{"alice-id"}
	existing.Logins["alice-id"] = "alice"
	existing.Repos = []collection.RepoAccess{{Name: "old-repo", Groups: []string{}, Users: []string{}}}
	if err := existing.Save(); err != nil {
		t.Fatalf("pre-create: %v", err)
	}

	importOverwrite = true

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load after overwrite: %v", err)
	}
	// Old member alice must be gone.
	for _, m := range col.Members {
		if m == "alice-id" {
			t.Error("alice should have been replaced by overwrite")
		}
	}
	// bob must be present (from import).
	foundBob := false
	for _, m := range col.Members {
		if m == "bob-id" {
			foundBob = true
		}
	}
	if !foundBob {
		t.Error("expected bob from import after overwrite")
	}
	// old-repo must be gone.
	for _, r := range col.Repos {
		if r.Name == "old-repo" {
			t.Error("old-repo should have been replaced by overwrite")
		}
	}
}

func TestImport_ConflictSkip(t *testing.T) {
	mock := newImportMock()
	mock.teams = []api.TeamInfo{{ID: 1, Slug: "payments-team", Name: "Payments"}}
	mock.membersBySlug["payments-team"] = []api.UserInfo{{ID: "bob-id", Login: "bob"}}
	mock.reposBySlug["payments-team"] = []api.RepoInfo{{Name: "r2", CloneURL: "u"}}
	defer setupImportTest(t, mock)()

	// Pre-create an existing collection.
	existing, _ := collection.New("acme-corp-payments-team", "github.com",
		api.UserInfo{ID: "caller-id", Login: "caller"}, collection.VisibilityPrivate)
	existing.Members = []string{"alice-id"}
	existing.Logins["alice-id"] = "alice"
	existing.Repos = []collection.RepoAccess{{Name: "old-repo", Groups: []string{}, Users: []string{}}}
	if err := existing.Save(); err != nil {
		t.Fatalf("pre-create: %v", err)
	}

	importSkipExisting = true

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport = %v", err)
	}

	col, err := collection.Load("acme-corp-payments-team")
	if err != nil {
		t.Fatalf("Load after skip: %v", err)
	}
	// Local collection must be unchanged: alice present, old-repo present, bob absent.
	foundAlice := false
	for _, m := range col.Members {
		if m == "alice-id" {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Error("expected alice to remain (skip left local unchanged)")
	}
	for _, m := range col.Members {
		if m == "bob-id" {
			t.Error("bob must not be present (skip did not apply import)")
		}
	}
	foundOldRepo := false
	for _, r := range col.Repos {
		if r.Name == "old-repo" {
			foundOldRepo = true
		}
	}
	if !foundOldRepo {
		t.Error("expected old-repo to remain (skip left local unchanged)")
	}
}

func TestImport_FlattenNestedTeams(t *testing.T) {
	parentTeam := api.TeamInfo{ID: 1, Slug: "backend", Name: "Backend", ParentSlug: ""}
	childTeam := api.TeamInfo{ID: 2, Slug: "payments", Name: "Payments", ParentSlug: "backend"}

	// flatten=true → child name is just "payments".
	if got := collectionNameForTeam(childTeam, true); got != "payments" {
		t.Errorf("flatten=true: collectionNameForTeam = %q, want payments", got)
	}
	// flatten=false → child name is "backend-payments".
	if got := collectionNameForTeam(childTeam, false); got != "backend-payments" {
		t.Errorf("flatten=false: collectionNameForTeam = %q, want backend-payments", got)
	}
	// Top-level team: same regardless of flatten.
	if got := collectionNameForTeam(parentTeam, false); got != "backend" {
		t.Errorf("top-level team flatten=false: collectionNameForTeam = %q, want backend", got)
	}
}

func TestImport_BuildCollectionFromTeam_Valid(t *testing.T) {
	team := api.TeamInfo{ID: 1, Slug: "payments-team", Name: "Payments", Description: "handles payments"}
	members := []api.UserInfo{{ID: "alice-id", Login: "alice"}, {ID: "bob-id", Login: "bob"}}
	repos := []api.RepoInfo{{Name: "payments-api", CloneURL: "u"}, {Name: "payments-db", CloneURL: "u"}}

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := buildCollectionFromTeam(
		team, members, nil, repos,
		"acme-corp", "payments-team", "acme-corp", "github.com",
		false, "caller-id", "caller",
	)
	if err != nil {
		t.Fatalf("buildCollectionFromTeam = %v", err)
	}

	if col.Name != "payments-team" {
		t.Errorf("Name = %q, want payments-team", col.Name)
	}
	if col.Host != "github.com" {
		t.Errorf("Host = %q, want github.com", col.Host)
	}
	if col.Namespace != "acme-corp" {
		t.Errorf("Namespace = %q, want acme-corp", col.Namespace)
	}
	if col.Description != "handles payments" {
		t.Errorf("Description = %q, want 'handles payments'", col.Description)
	}
	if col.Version != collection.CurrentVersion {
		t.Errorf("Version = %q, want %q", col.Version, collection.CurrentVersion)
	}
	if len(col.Repos) != 2 {
		t.Errorf("len(Repos) = %d, want 2", len(col.Repos))
	}
}

func TestImport_BuildCollectionFromTeam_OwnerNotInMembers(t *testing.T) {
	team := api.TeamInfo{ID: 1, Slug: "payments-team", Name: "Payments"}
	members := []api.UserInfo{
		{ID: "alice-id", Login: "alice"},
		{ID: "bob-id", Login: "bob"},
	}
	maintainers := []api.UserInfo{{ID: "alice-id", Login: "alice"}}

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := buildCollectionFromTeam(
		team, members, maintainers, nil,
		"acme-corp", "payments-team", "acme-corp", "github.com",
		true, "caller-id", "caller",
	)
	if err != nil {
		t.Fatalf("buildCollectionFromTeam = %v", err)
	}

	// alice is maintainer → becomes owner; she must NOT appear in Members.
	if col.Owner != "alice-id" {
		t.Errorf("Owner = %q, want alice-id", col.Owner)
	}
	for _, m := range col.Members {
		if m == "alice-id" {
			t.Error("owner (alice) must not appear in Members")
		}
	}
	// bob is not the owner and must appear in Members.
	foundBob := false
	for _, m := range col.Members {
		if m == "bob-id" {
			foundBob = true
		}
	}
	if !foundBob {
		t.Error("expected bob to be in Members")
	}
}

func TestImport_LoginsMapComplete(t *testing.T) {
	team := api.TeamInfo{ID: 1, Slug: "payments-team", Name: "Payments"}
	members := []api.UserInfo{
		{ID: "alice-id", Login: "alice"},
		{ID: "bob-id", Login: "bob"},
		{ID: "charlie-id", Login: "charlie"},
	}

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := buildCollectionFromTeam(
		team, members, nil, nil,
		"acme-corp", "payments-team", "acme-corp", "github.com",
		false, "caller-id", "caller",
	)
	if err != nil {
		t.Fatalf("buildCollectionFromTeam = %v", err)
	}

	// caller (owner) + all 3 members must be in Logins.
	expected := map[string]string{
		"caller-id":  "caller",
		"alice-id":   "alice",
		"bob-id":     "bob",
		"charlie-id": "charlie",
	}
	for id, wantLogin := range expected {
		if got := col.Logins[id]; got != wantLogin {
			t.Errorf("Logins[%q] = %q, want %q", id, got, wantLogin)
		}
	}
}
