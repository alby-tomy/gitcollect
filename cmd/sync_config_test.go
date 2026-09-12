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
