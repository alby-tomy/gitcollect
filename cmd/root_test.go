package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
)

func TestStaleDays(t *testing.T) {
	if got := staleDays(time.Now()); got != 0 {
		t.Errorf("staleDays(now) = %d, want 0", got)
	}
	if got := staleDays(time.Now().Add(-29 * 24 * time.Hour)); got != 0 {
		t.Errorf("staleDays(29 days ago) = %d, want 0 (under the 30-day threshold)", got)
	}
	if got := staleDays(time.Now().Add(-45 * 24 * time.Hour)); got != 45 {
		t.Errorf("staleDays(45 days ago) = %d, want 45", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"cybersecurity", "cybersecurity", 0},
		{"cybersecurty", "cybersecurity", 1},   // one deletion
		{"cybersecurityy", "cybersecurity", 1}, // one insertion
		{"cybersecurety", "cybersecurity", 1},  // one substitution
		{"kitten", "sitting", 3},
	}
	for _, tc := range cases {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSuggestCollectionName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	collDir, err := config.CollectionsDir()
	if err != nil {
		t.Fatalf("CollectionsDir: %v", err)
	}
	if err := config.EnsureDir(collDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	for _, name := range []string{"cybersecurity", "machine-learning"} {
		if err := os.WriteFile(filepath.Join(collDir, name+".yaml"), []byte("name: "+name+"\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	if got := suggestCollectionName("cybersecurty"); got != "cybersecurity" {
		t.Errorf("suggestCollectionName(typo) = %q, want %q", got, "cybersecurity")
	}
	if got := suggestCollectionName("totally-unrelated-name"); got != "" {
		t.Errorf("suggestCollectionName(unrelated) = %q, want empty", got)
	}
	if got := suggestCollectionName("cybersecurity"); got != "" {
		t.Errorf("suggestCollectionName(exact match) = %q, want empty (nothing to suggest)", got)
	}
}

// rootMock is a minimal api.Client stub for testing root.go helpers directly.
type rootMock struct {
	getAuthCalls int
	userInfo     api.UserInfo
	getAuthErr   error
}

func (m *rootMock) GetAuthenticatedUser() (api.UserInfo, error) {
	m.getAuthCalls++
	return m.userInfo, m.getAuthErr
}
func (m *rootMock) GetUser(username string) (api.UserInfo, error) {
	return api.UserInfo{ID: username + "-id", Login: username}, nil
}
func (m *rootMock) GetRepo(owner, repo string) (api.RepoInfo, error) { return api.RepoInfo{}, nil }
func (m *rootMock) AddCollaborator(owner, repo, username, permission string) error { return nil }
func (m *rootMock) RemoveCollaborator(owner, repo, username string) error          { return nil }
func (m *rootMock) CheckCollaborator(owner, repo, username string) (bool, error)   { return false, nil }
func (m *rootMock) GetPendingInvite(owner, repo, username string) (bool, error)    { return false, nil }
func (m *rootMock) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *rootMock) Host() string { return "github.com" }

// resetCallerCache clears the package-level identity cache before a test and
// restores it to empty afterward, so each test starts from a clean slate.
func resetCallerCache(t *testing.T) {
	t.Helper()
	cachedUser, cachedUserID = "", ""
	t.Cleanup(func() { cachedUser, cachedUserID = "", "" })
}

func TestLoginsFor(t *testing.T) {
	col, err := collection.New("test-col", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Logins["alice-id"] = "alice"
	col.Logins["bob-id"] = "bob"

	got := loginsFor(col, []string{"alice-id", "bob-id"})
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Errorf("loginsFor = %v, want [alice bob]", got)
	}

	got2 := loginsFor(col, []string{"bob-id", "alice-id"})
	if got2[0] != "bob" || got2[1] != "alice" {
		t.Errorf("loginsFor(reversed) = %v, want [bob alice]", got2)
	}

	// Unknown ID must fall back to the raw ID rather than an empty string.
	got3 := loginsFor(col, []string{"unknown-id"})
	if len(got3) != 1 || got3[0] != "unknown-id" {
		t.Errorf("loginsFor(unknown) = %v, want [unknown-id]", got3)
	}

	if got4 := loginsFor(col, nil); len(got4) != 0 {
		t.Errorf("loginsFor(nil) = %v, want []", got4)
	}
}

func TestCurrentUserInfo_CachesResult(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetCallerCache(t)

	mock := &rootMock{userInfo: api.UserInfo{ID: "alice-id", Login: "alice"}}

	u1, err := currentUserInfo(mock)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	u2, err := currentUserInfo(mock)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if mock.getAuthCalls != 1 {
		t.Errorf("GetAuthenticatedUser called %d times, want 1 (result should be cached)", mock.getAuthCalls)
	}
	if u1.Login != "alice" || u2.Login != "alice" {
		t.Errorf("login = %q and %q, want alice for both", u1.Login, u2.Login)
	}
	if u1.ID != "alice-id" || u2.ID != "alice-id" {
		t.Errorf("ID = %q and %q, want alice-id for both", u1.ID, u2.ID)
	}
}

func TestCurrentUserInfo_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetCallerCache(t)

	sentinel := errors.New("token rejected")
	mock := &rootMock{getAuthErr: sentinel}
	_, err := currentUserInfo(mock)
	if !errors.Is(err, sentinel) {
		t.Errorf("currentUserInfo error = %v, want to wrap sentinel", err)
	}
}

func TestCurrentUser_ReturnsLogin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetCallerCache(t)

	mock := &rootMock{userInfo: api.UserInfo{ID: "bob-id", Login: "bob"}}
	login, err := currentUser(mock)
	if err != nil {
		t.Fatalf("currentUser: %v", err)
	}
	if login != "bob" {
		t.Errorf("currentUser = %q, want bob", login)
	}
}

func TestCurrentUserID_ReturnsID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetCallerCache(t)

	mock := &rootMock{userInfo: api.UserInfo{ID: "carol-id", Login: "carol"}}
	id, err := currentUserID(mock)
	if err != nil {
		t.Fatalf("currentUserID: %v", err)
	}
	if id != "carol-id" {
		t.Errorf("currentUserID = %q, want carol-id", id)
	}
}

func TestMigrateIfNeeded_NoOp_CurrentVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("already-v2", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	mock := &rootMock{}
	if err := migrateIfNeeded(col, mock); err != nil {
		t.Fatalf("migrateIfNeeded: %v", err)
	}
	if mock.getAuthCalls != 0 {
		t.Errorf("GetAuthenticatedUser called %d times on a current-version collection, want 0", mock.getAuthCalls)
	}
	if col.Version != collection.CurrentVersion {
		t.Errorf("version = %q after no-op migrate, want %q", col.Version, collection.CurrentVersion)
	}
}

func TestMigrateIfNeeded_UpgradesV1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	collDir, err := config.CollectionsDir()
	if err != nil {
		t.Fatalf("CollectionsDir: %v", err)
	}
	if err := config.EnsureDir(collDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	v1yaml := "name: legacy\nhost: github.com\nvisibility: private\nowner: alice\nmembers:\n  - bob\nversion: \"1\"\n"
	if err := os.WriteFile(filepath.Join(collDir, "legacy.yaml"), []byte(v1yaml), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	col, err := collection.Load("legacy")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if col.Version != "1" {
		t.Fatalf("expected version 1 before migration, got %q", col.Version)
	}

	// GetUser returns {ID: login+"-id", Login: login} so alice→alice-id, bob→bob-id.
	mock := &rootMock{}
	if err := migrateIfNeeded(col, mock); err != nil {
		t.Fatalf("migrateIfNeeded: %v", err)
	}

	if col.Version != collection.CurrentVersion {
		t.Errorf("version after migration = %q, want %q", col.Version, collection.CurrentVersion)
	}
	if col.Owner != "alice-id" {
		t.Errorf("Owner after migration = %q, want alice-id", col.Owner)
	}
	if len(col.Members) != 1 || col.Members[0] != "bob-id" {
		t.Errorf("Members after migration = %v, want [bob-id]", col.Members)
	}
	if col.Logins["alice-id"] != "alice" {
		t.Errorf("Logins[alice-id] = %q, want alice", col.Logins["alice-id"])
	}
	if col.Logins["bob-id"] != "bob" {
		t.Errorf("Logins[bob-id] = %q, want bob", col.Logins["bob-id"])
	}
}

func TestLoadForRead_PublicCollection_NoAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("open-proj", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPublic)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, caller, callerID, err := loadForRead("open-proj")
	if err != nil {
		t.Fatalf("loadForRead: %v", err)
	}
	if got.Name != "open-proj" {
		t.Errorf("Name = %q, want open-proj", got.Name)
	}
	if caller != "" || callerID != "" {
		t.Errorf("caller=%q callerID=%q, want both empty for a public collection", caller, callerID)
	}
}

func TestLoadForRead_MissingCollection_ReturnsForbidden(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	_, _, _, err := loadForRead("does-not-exist")
	if !errors.Is(err, access.ErrForbidden) {
		t.Errorf("loadForRead(missing) = %v, want access.ErrForbidden", err)
	}
}

func TestLoadForOwner_CollectionNotFound_VerbPrefixed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	_, _, _, _, err := loadForOwner("delete", "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing collection, got nil")
	}
	if !strings.HasPrefix(err.Error(), "delete:") {
		t.Errorf("expected error prefixed with verb 'delete:', got: %v", err)
	}
}
