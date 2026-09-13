package cmd

import (
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// scanMock implements api.Client for scan tests.
type scanMock struct {
	multiAddMock // embed for all methods we don't override
	orgRepos     []api.RepoInfo
	orgReposErr  error
}

func (m *scanMock) ListOrgRepos(_ string) ([]api.RepoInfo, error) {
	return m.orgRepos, m.orgReposErr
}
func (m *scanMock) ListOpenPRs(owner, repo string) ([]api.PRInfo, error) { return nil, nil }
func (m *scanMock) GetTokenScopes() ([]string, error)                    { return []string{"repo"}, nil }

func newScanMock(repos []api.RepoInfo) *scanMock {
	return &scanMock{multiAddMock: *newMultiAddMock(), orgRepos: repos}
}

func setupScanTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() { cachedClient = nil; cachedUser = ""; cachedUserID = "" })
}

// TestRepoPrefix checks the prefix-extraction helper.
func TestRepoPrefix(t *testing.T) {
	cases := []struct{ name, want string }{
		{"payments-checkout", "payments"},
		{"payments_gateway", "payments"},
		{"standalone", "standalone"},
		{"a-b-c", "a"},
		{"", ""},
	}
	for _, tc := range cases {
		got := repoPrefix(tc.name)
		if got != tc.want {
			t.Errorf("repoPrefix(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestGroupByPrefix verifies repos are bucketed by their name prefix.
func TestGroupByPrefix(t *testing.T) {
	repos := []api.RepoInfo{
		{Name: "payments-checkout"},
		{Name: "payments-gateway"},
		{Name: "auth-service"},
		{Name: "standalone"},
	}
	groups := groupByPrefix(repos)

	// Expect 3 groups: auth, payments, standalone.
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}
	byName := make(map[string][]api.RepoInfo)
	for _, g := range groups {
		byName[g.name] = g.repos
	}
	if len(byName["payments"]) != 2 {
		t.Errorf("expected 2 payments repos, got %d", len(byName["payments"]))
	}
	if len(byName["auth"]) != 1 {
		t.Errorf("expected 1 auth repo, got %d", len(byName["auth"]))
	}
	if len(byName["standalone"]) != 1 {
		t.Errorf("expected 1 standalone repo, got %d", len(byName["standalone"]))
	}
}

// TestSanitizeCollectionName ensures illegal chars are replaced with dashes.
func TestSanitizeCollectionName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"my-org", "my-org"},
		{"my_org", "my-org"},
		{"my org", "my-org"},
		{"my--org", "my-org"},
		{"-leading", "leading"},
		{"trailing-", "trailing"},
	}
	for _, tc := range cases {
		got := sanitizeCollectionName(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeCollectionName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestScan_DryRun verifies --dry-run prints groups without writing files.
func TestScan_DryRun(t *testing.T) {
	setupScanTest(t)

	mock := newScanMock([]api.RepoInfo{
		{Name: "payments-checkout", CloneURL: "https://github.com/acme/payments-checkout.git"},
		{Name: "payments-gateway", CloneURL: "https://github.com/acme/payments-gateway.git"},
		{Name: "auth-service", CloneURL: "https://github.com/acme/auth-service.git"},
	})
	cachedClient = mock

	scanOrg = "acme"
	scanFrom = ""
	scanGroupBy = "prefix"
	scanDryRun = true
	scanApply = false
	scanVerify = false
	scanNoArch = false
	t.Cleanup(func() {
		scanOrg = ""
		scanDryRun = false
	})

	out := captureStdout(func() {
		if err := runScan(scanCmd, nil); err != nil {
			t.Fatalf("runScan: %v", err)
		}
	})

	if !strings.Contains(out, "payments-checkout") {
		t.Errorf("expected payments-checkout in dry-run output, got:\n%s", out)
	}
	if !strings.Contains(out, "auth-service") {
		t.Errorf("expected auth-service in dry-run output, got:\n%s", out)
	}
	// No collection files should have been written.
	cols, _ := collection.List()
	if len(cols) != 0 {
		t.Errorf("dry-run should not write collections, got %v", cols)
	}
}

// TestScan_Apply writes collection files for each prefix group.
func TestScan_Apply(t *testing.T) {
	setupScanTest(t)

	mock := newScanMock([]api.RepoInfo{
		{Name: "payments-checkout"},
		{Name: "payments-gateway"},
		{Name: "auth-service"},
	})
	cachedClient = mock

	scanOrg = "acme"
	scanFrom = ""
	scanGroupBy = "prefix"
	scanDryRun = false
	scanApply = true
	scanVerify = false
	scanNoArch = false
	t.Cleanup(func() {
		scanOrg = ""
		scanApply = false
	})

	captureStdout(func() {
		if err := runScan(scanCmd, nil); err != nil {
			t.Fatalf("runScan --apply: %v", err)
		}
	})

	cols, err := collection.List()
	if err != nil {
		t.Fatalf("collection.List: %v", err)
	}
	if len(cols) != 2 {
		t.Fatalf("expected 2 collections (auth, payments), got %d: %v", len(cols), cols)
	}

	// Verify payments collection has 2 repos.
	var paymentsName string
	for _, c := range cols {
		if strings.HasPrefix(c, "acme-payments") {
			paymentsName = c
		}
	}
	if paymentsName == "" {
		t.Fatalf("expected an acme-payments collection, got: %v", cols)
	}
	col, err := collection.Load(paymentsName)
	if err != nil {
		t.Fatalf("collection.Load(%q): %v", paymentsName, err)
	}
	if len(col.Repos) != 2 {
		t.Errorf("expected 2 repos in %q, got %d", paymentsName, len(col.Repos))
	}
	if col.Namespace != "acme" {
		t.Errorf("expected Namespace=acme, got %q", col.Namespace)
	}
}

// TestScan_NoRepos handles an org with zero repos gracefully.
func TestScan_NoRepos(t *testing.T) {
	setupScanTest(t)

	mock := newScanMock([]api.RepoInfo{})
	cachedClient = mock

	scanOrg = "acme"
	scanFrom = ""
	scanGroupBy = "prefix"
	scanDryRun = false
	scanApply = false
	scanVerify = false
	scanNoArch = false
	t.Cleanup(func() { scanOrg = "" })

	if err := runScan(scanCmd, nil); err != nil {
		t.Fatalf("runScan on empty org: %v", err)
	}
}

// TestScan_NoArchived verifies --no-archived excludes archived repos.
func TestScan_NoArchived(t *testing.T) {
	setupScanTest(t)

	mock := newScanMock([]api.RepoInfo{
		{Name: "payments-checkout", Archived: false},
		{Name: "payments-legacy", Archived: true},
	})
	cachedClient = mock

	scanOrg = "acme"
	scanFrom = ""
	scanGroupBy = "prefix"
	scanDryRun = true
	scanApply = false
	scanVerify = false
	scanNoArch = true
	t.Cleanup(func() {
		scanOrg = ""
		scanDryRun = false
		scanNoArch = false
	})

	out := captureStdout(func() {
		if err := runScan(scanCmd, nil); err != nil {
			t.Fatalf("runScan --no-archived: %v", err)
		}
	})

	if strings.Contains(out, "payments-legacy") {
		t.Errorf("archived repo should be excluded, but found in output:\n%s", out)
	}
	if !strings.Contains(out, "payments-checkout") {
		t.Errorf("active repo should appear in output, got:\n%s", out)
	}
}

// TestScan_FlatGrouping puts all repos in a single collection.
func TestScan_FlatGrouping(t *testing.T) {
	setupScanTest(t)

	mock := newScanMock([]api.RepoInfo{
		{Name: "payments-checkout"},
		{Name: "auth-service"},
		{Name: "docs"},
	})
	cachedClient = mock

	scanOrg = "acme"
	scanFrom = ""
	scanGroupBy = "flat"
	scanDryRun = false
	scanApply = true
	scanVerify = false
	scanNoArch = false
	t.Cleanup(func() {
		scanOrg = ""
		scanGroupBy = "prefix"
		scanApply = false
	})

	captureStdout(func() {
		if err := runScan(scanCmd, nil); err != nil {
			t.Fatalf("runScan --group-by flat: %v", err)
		}
	})

	cols, err := collection.List()
	if err != nil {
		t.Fatalf("collection.List: %v", err)
	}
	if len(cols) != 1 {
		t.Fatalf("flat mode should create 1 collection, got %d: %v", len(cols), cols)
	}
	col, err := collection.Load(cols[0])
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if len(col.Repos) != 3 {
		t.Errorf("expected 3 repos in flat collection, got %d", len(col.Repos))
	}
}

// --- regression: scan --apply must never overwrite an existing collection ---

// setScanFlags sets the package-level scan flags for one test and restores
// them afterwards, so tests do not leak state into each other.
func setScanFlags(t *testing.T, org string, apply, dryRun bool) {
	t.Helper()
	prevOrg, prevApply, prevDry, prevGroup := scanOrg, scanApply, scanDryRun, scanGroupBy
	scanOrg, scanApply, scanDryRun, scanGroupBy = org, apply, dryRun, "prefix"
	t.Cleanup(func() {
		scanOrg, scanApply, scanDryRun, scanGroupBy = prevOrg, prevApply, prevDry, prevGroup
	})
}

// collection.New validates a name but does NOT check whether the file
// already exists, so scan's "already exists — skip" branch never fired and
// Save clobbered the collection instead: members, groups and per-repo
// access rules all lost. Running scan --apply over an established
// collection must leave it exactly as it was.
func TestScanApply_DoesNotOverwriteExistingCollection(t *testing.T) {
	setupScanTest(t)
	cachedClient = newScanMock([]api.RepoInfo{
		{Name: "payments-api"}, {Name: "payments-db"},
	})
	setScanFlags(t, "acme", true, false)

	// An established collection under the name scan will derive.
	existing, err := collection.New("acme-payments", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	existing.Members = []string{"alice-id"}
	existing.Logins["alice-id"] = "alice"
	existing.Groups = map[string][]string{"backend": {"alice-id"}}
	existing.Repos = []collection.RepoAccess{
		{Name: "payments-api", Groups: []string{"backend"}, Users: []string{}},
	}
	if err := existing.Save(); err != nil {
		t.Fatalf("save existing: %v", err)
	}

	if err := runScan(nil, nil); err != nil {
		t.Fatalf("runScan: %v", err)
	}

	after, err := collection.Load("acme-payments")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(after.Members) != 1 || after.Members[0] != "alice-id" {
		t.Errorf("existing members were clobbered: %v", after.Members)
	}
	if len(after.Groups["backend"]) != 1 {
		t.Errorf("existing group was clobbered: %v", after.Groups)
	}
	if len(after.Repos) != 1 || len(after.Repos[0].Groups) != 1 {
		t.Errorf("existing per-repo access rule was clobbered: %+v", after.Repos)
	}
}

func TestScanApply_CreatesCollectionWhenAbsent(t *testing.T) {
	setupScanTest(t)
	cachedClient = newScanMock([]api.RepoInfo{
		{Name: "payments-api"}, {Name: "payments-db"},
	})
	setScanFlags(t, "acme", true, false)

	if err := runScan(nil, nil); err != nil {
		t.Fatalf("runScan: %v", err)
	}

	col, err := collection.Load("acme-payments")
	if err != nil {
		t.Fatalf("expected scan to create acme-payments: %v", err)
	}
	if len(col.Repos) != 2 {
		t.Errorf("expected both payments repos, got %d", len(col.Repos))
	}
	if col.Namespace != "acme" {
		t.Errorf("expected namespace acme, got %q", col.Namespace)
	}
}

func TestScanDryRun_WritesNothing(t *testing.T) {
	setupScanTest(t)
	cachedClient = newScanMock([]api.RepoInfo{{Name: "payments-api"}})
	setScanFlags(t, "acme", false, true)

	if err := runScan(nil, nil); err != nil {
		t.Fatalf("runScan: %v", err)
	}
	if _, err := collection.Load("acme-payments"); err == nil {
		t.Error("--dry-run must not create a collection file")
	}
}
