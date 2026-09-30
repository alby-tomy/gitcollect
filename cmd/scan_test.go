package cmd

import (
	"sort"
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

// --- token grouping ---

func scanRepos(names ...string) []api.RepoInfo {
	out := make([]api.RepoInfo, 0, len(names))
	for _, n := range names {
		out = append(out, api.RepoInfo{Name: n})
	}
	return out
}

func groupNames(gs []scanGroup) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.name)
	}
	sort.Strings(out)
	return out
}

// The case the old prefix rule got backwards: one team's pricing module,
// split by region. Grouping on the first segment produced "china", "eu" and
// "us" - scattering the very module the operator was trying to assemble.
func TestGroupByToken_SharedSuffixIsTheCategory(t *testing.T) {
	groups := groupByToken(scanRepos("china-pricing", "eu-pricing", "us-pricing"))

	if len(groups) != 1 || groups[0].name != "pricing" {
		t.Fatalf("expected one \"pricing\" group, got %v", groupNames(groups))
	}
	if len(groups[0].repos) != 3 {
		t.Errorf("expected all 3 repos in pricing, got %d", len(groups[0].repos))
	}
	// The old behaviour, pinned so the regression is visible if it returns.
	old := groupNames(groupByPrefix(scanRepos("china-pricing", "eu-pricing", "us-pricing")))
	if len(old) != 3 {
		t.Errorf("prefix grouping should still split these three ways, got %v", old)
	}
}

// A shared word must be found wherever it sits, not only as a suffix.
func TestGroupByToken_SharedPrefixAlsoWorks(t *testing.T) {
	groups := groupByToken(scanRepos("payments-gateway", "payments-checkout", "payments-ledger"))
	if len(groups) != 1 || groups[0].name != "payments" {
		t.Fatalf("expected one \"payments\" group, got %v", groupNames(groups))
	}
}

// Generic words describe what a repo is, not what it belongs to. Collapsing
// on them would be worse than not grouping at all.
func TestGroupByToken_IgnoresGenericWords(t *testing.T) {
	groups := groupByToken(scanRepos("cart-service", "search-service", "availability-service"))
	names := groupNames(groups)
	for _, n := range names {
		if n == "service" {
			t.Fatalf("generic word became a category: %v", names)
		}
	}
	if len(groups) != 3 {
		t.Errorf("expected 3 distinct groups, got %v", names)
	}
}

// A word unique to one repo must not invent a category for it.
func TestGroupByToken_UniqueWordIsNotACategory(t *testing.T) {
	groups := groupByToken(scanRepos("alpha-pricing", "beta-pricing", "lonely-experiment"))
	names := groupNames(groups)
	if len(names) != 2 {
		t.Fatalf("expected pricing + the singleton, got %v", names)
	}
	var pricing *scanGroup
	for i := range groups {
		if groups[i].name == "pricing" {
			pricing = &groups[i]
		}
	}
	if pricing == nil || len(pricing.repos) != 2 {
		t.Errorf("pricing should hold exactly the two pricing repos, got %v", names)
	}
}

// The user's full worked example, end to end.
func TestGroupByToken_EcommerceExample(t *testing.T) {
	groups := groupByToken(scanRepos(
		"china-pricing", "eu-pricing", "us-pricing",
		"search-indexer", "search-ranking",
		"availability-checker", "availability-sync",
		"crm-customers", "crm-tickets",
	))
	got := groupNames(groups)
	want := []string{"availability", "crm", "pricing", "search"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Grouping must not depend on map iteration order.
func TestGroupByToken_Deterministic(t *testing.T) {
	in := scanRepos("a-pricing", "b-pricing", "c-search", "d-search")
	first := groupNames(groupByToken(in))
	for i := 0; i < 20; i++ {
		if got := groupNames(groupByToken(in)); !equalStrings(got, first) {
			t.Fatalf("run %d gave %v, first run gave %v", i, got, first)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- interactive review ---

// Enter keeps the suggested category, which is what a piped or CI run sends.
func TestReviewGroups_EmptyAnswerKeepsSuggestion(t *testing.T) {
	withStdin(t, "\n\n")
	var got []scanGroup
	captureStdout(func() {
		got = reviewGroups([]scanGroup{{name: "pricing", repos: scanRepos("eu-pricing", "us-pricing")}})
	})
	if len(got) != 1 || got[0].name != "pricing" || len(got[0].repos) != 2 {
		t.Fatalf("expected both repos kept in pricing, got %v", groupNames(got))
	}
}

// "d" drops a repo that the name-based guess placed wrongly.
func TestReviewGroups_DropsRepo(t *testing.T) {
	withStdin(t, "\nd\n")
	var got []scanGroup
	captureStdout(func() {
		got = reviewGroups([]scanGroup{{name: "pricing", repos: scanRepos("eu-pricing", "unrelated-thing")}})
	})
	if len(got) != 1 || len(got[0].repos) != 1 || got[0].repos[0].Name != "eu-pricing" {
		t.Fatalf("expected only eu-pricing to survive, got %+v", got)
	}
}

// Typing a name files the repo under that category instead - the
// "does this really belong here?" correction.
func TestReviewGroups_ReassignsRepo(t *testing.T) {
	withStdin(t, "\ncrm\n")
	var got []scanGroup
	captureStdout(func() {
		got = reviewGroups([]scanGroup{{name: "pricing", repos: scanRepos("eu-pricing", "customer-tickets")}})
	})
	names := groupNames(got)
	if len(names) != 2 || names[0] != "crm" || names[1] != "pricing" {
		t.Fatalf("expected crm + pricing, got %v", names)
	}
	for _, g := range got {
		if g.name == "crm" && (len(g.repos) != 1 || g.repos[0].Name != "customer-tickets") {
			t.Errorf("crm should hold customer-tickets, got %+v", g.repos)
		}
	}
}

// A typo that sanitises to nothing must not silently discard the repo.
func TestReviewGroups_UnusableNameKeepsRepo(t *testing.T) {
	withStdin(t, "***\n")
	var got []scanGroup
	captureStdout(func() {
		got = reviewGroups([]scanGroup{{name: "pricing", repos: scanRepos("eu-pricing")}})
	})
	if len(got) != 1 || len(got[0].repos) != 1 {
		t.Fatalf("repo must survive an unusable category name, got %+v", got)
	}
}
