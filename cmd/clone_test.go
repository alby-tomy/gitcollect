package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func TestSplitPick(t *testing.T) {
	cases := []struct {
		name string
		raw  []string
		want []string
	}{
		{"nil", nil, nil},
		{"single value", []string{"repo1"}, []string{"repo1"}},
		{"space-separated within one value", []string{"repo1 repo2"}, []string{"repo1", "repo2"}},
		{"repeated flag", []string{"repo1", "repo2"}, []string{"repo1", "repo2"}},
		{"mixed", []string{"repo1 repo2", "repo3"}, []string{"repo1", "repo2", "repo3"}},
		{"extra whitespace collapses", []string{"  repo1   repo2  "}, []string{"repo1", "repo2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitPick(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitPick(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFirstPendingInvite(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	client := &pendingInviteMock{pending: map[string]bool{"repo2/alice": true}}

	if got := firstPendingInvite(col, "alice", []string{"repo1"}, client); got != "" {
		t.Errorf("expected no pending invite among repo1 alone, got %q", got)
	}
	if got := firstPendingInvite(col, "alice", []string{"repo1", "repo2"}, client); got != "repo2" {
		t.Errorf("expected repo2 to be reported as pending, got %q", got)
	}
	if got := firstPendingInvite(col, "alice", nil, client); got != "" {
		t.Errorf("expected no pending invite for an empty skipped list, got %q", got)
	}
}

func TestSelectCloneTargets_NoPickReturnsAllAccessible(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Repos = []collection.RepoAccess{
		{Name: "repo-a"},
		{Name: "repo-b"},
		{Name: "repo-c"},
	}
	accessible := []collection.RepoAccess{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}

	targets, skipped, err := selectCloneTargets(col, accessible, nil)
	if err != nil {
		t.Fatalf("selectCloneTargets: %v", err)
	}
	if !reflect.DeepEqual(targets, accessible) {
		t.Errorf("targets = %v, want %v", targets, accessible)
	}
	if len(skipped) != 1 || skipped[0] != "repo-c" {
		t.Errorf("skipped = %v, want [repo-c]", skipped)
	}
}

func TestSelectCloneTargets_PickFiltersToAccessible(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	accessible := []collection.RepoAccess{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}
	col.Repos = accessible

	targets, _, err := selectCloneTargets(col, accessible, []string{"repo-b"})
	if err != nil {
		t.Fatalf("selectCloneTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "repo-b" {
		t.Errorf("targets = %v, want [repo-b]", targets)
	}
}

func TestSelectCloneTargets_PickInaccessibleRepoErrors(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	accessible := []collection.RepoAccess{{Name: "repo-a"}}
	col.Repos = accessible

	_, _, err = selectCloneTargets(col, accessible, []string{"repo-z"})
	if err == nil {
		t.Fatal("expected error for picking an inaccessible repo, got nil")
	}
}

func TestCloneOne_DryRun_NoGitCall(t *testing.T) {
	dir := t.TempDir()
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner-login"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	client := newMultiAddMock()

	// dry-run=true: GetRepo is called to resolve the URL but git.Clone is not invoked.
	if err := cloneOne(col, client, "my-repo", dir, true); err != nil {
		t.Fatalf("cloneOne(dryRun=true): %v", err)
	}

	// Confirm no directory was created under dest (git.Clone would have created one).
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == "my-repo" {
			t.Errorf("git.Clone was called in dry-run mode: found %s directory", filepath.Join(dir, "my-repo"))
		}
	}
}

// ── A3 skip-existing tests ───────────��────────────────────────────────────────

func TestClone_SkipsExistingDirectory(t *testing.T) {
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "my-repo"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	col, err := collection.New("test", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	targets := []collection.RepoAccess{{Name: "my-repo"}}
	client := newMultiAddMock()

	var results []cloneResult
	captureStderr(func() {
		captureStdout(func() {
			// dryRun=false: the dir already exists so the skip fires before
			// cloneOne is called — no real git subprocess runs.
			results = cloneAll(col, client, targets, dest, 1, false)
		})
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].skipped {
		t.Error("expected result.skipped=true for pre-existing directory")
	}
	if results[0].err != nil {
		t.Errorf("expected no error for skipped repo, got %v", results[0].err)
	}
}

func TestClone_SkipCountInSummary(t *testing.T) {
	setupGitTest(t, "testcol-skipcount")

	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "repo1"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	oldDest, oldDryRun := cloneDest, cloneDryRun
	t.Cleanup(func() { cloneDest = oldDest; cloneDryRun = oldDryRun })
	cloneDest = dest
	cloneDryRun = false

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runClone(nil, []string{"testcol-skipcount"}); err != nil {
				t.Errorf("runClone = %v", err)
			}
		})
	})

	if !strings.Contains(out, "already present") {
		t.Errorf("expected 'already present' in summary stdout, got: %q", out)
	}
}

func TestClone_SuggestsSyncAfterSkip(t *testing.T) {
	setupGitTest(t, "testcol-skipsugg")

	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "repo1"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	oldDest, oldDryRun := cloneDest, cloneDryRun
	t.Cleanup(func() { cloneDest = oldDest; cloneDryRun = oldDryRun })
	cloneDest = dest
	cloneDryRun = false

	stderr := captureStderr(func() {
		captureStdout(func() {
			if err := runClone(nil, []string{"testcol-skipsugg"}); err != nil {
				t.Errorf("runClone = %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "sync") {
		t.Errorf("expected sync suggestion in stderr when repos skipped, got: %q", stderr)
	}
}

func TestClone_ClonesNewWhenSomeExist(t *testing.T) {
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "existing-repo"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	col, err := collection.New("test", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	targets := []collection.RepoAccess{
		{Name: "existing-repo"},
		{Name: "new-repo"},
	}
	client := newMultiAddMock()

	var results []cloneResult
	captureStderr(func() {
		captureStdout(func() {
			// dryRun=true so new-repo's cloneOne returns nil without git.
			results = cloneAll(col, client, targets, dest, 1, true)
		})
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	var existingResult, newResult *cloneResult
	for i := range results {
		switch results[i].name {
		case "existing-repo":
			existingResult = &results[i]
		case "new-repo":
			newResult = &results[i]
		}
	}
	if existingResult == nil || newResult == nil {
		t.Fatalf("unexpected result names: %+v", results)
	}
	if !existingResult.skipped {
		t.Error("existing-repo directory is present — expected result.skipped=true")
	}
	if newResult.skipped {
		t.Error("new-repo directory is absent — expected result.skipped=false")
	}
	if newResult.err != nil {
		t.Errorf("expected no error for new-repo in dryRun mode, got %v", newResult.err)
	}
}

// ── B3: sync suggestion after clone failure ──────────────────────────────────

// failingGetRepoMock wraps multiAddMock but returns an error for repos in failFor.
type failingGetRepoMock struct {
	*multiAddMock
	failFor map[string]bool
}

func (m *failingGetRepoMock) GetRepo(owner, repo string) (api.RepoInfo, error) {
	if m.failFor[repo] {
		return api.RepoInfo{}, fmt.Errorf("mock: GetRepo %s failed", repo)
	}
	return m.multiAddMock.GetRepo(owner, repo)
}

func setupCloneFailTest(t *testing.T, collName string, repos []string, failRepos map[string]bool) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	base := newMultiAddMock()
	mock := &failingGetRepoMock{multiAddMock: base, failFor: failRepos}
	for _, r := range repos {
		mock.collabs["owner/"+r+"/owner"] = true
	}

	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	col, err := collection.New(collName, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	for _, r := range repos {
		col.Repos = append(col.Repos, collection.RepoAccess{Name: r, Groups: []string{}, Users: []string{}})
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
}

// TestClone_PartialFailure_SuggestsSync verifies that the sync suggestion
// appears in stderr when at least one clone fails.
func TestClone_PartialFailure_SuggestsSync(t *testing.T) {
	// two repos: repo1 succeeds (pre-existing dir → skipped), repo2 fails (GetRepo error)
	setupCloneFailTest(t, "testcol-partfail",
		[]string{"repo1", "repo2"},
		map[string]bool{"repo2": true},
	)

	dest := t.TempDir()
	// repo1 already present → skipped (not a failure)
	if err := os.MkdirAll(filepath.Join(dest, "repo1"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	oldDest, oldDryRun := cloneDest, cloneDryRun
	t.Cleanup(func() { cloneDest = oldDest; cloneDryRun = oldDryRun })
	cloneDest = dest
	cloneDryRun = false

	stderr := captureStderr(func() {
		captureStdout(func() {
			_ = runClone(nil, []string{"testcol-partfail"})
		})
	})

	if !strings.Contains(stderr, "sync") {
		t.Errorf("expected sync suggestion in stderr after partial clone failure, got: %q", stderr)
	}
}

// TestClone_AllFail_SuggestsSync verifies sync suggestion appears when all clones fail.
func TestClone_AllFail_SuggestsSync(t *testing.T) {
	setupCloneFailTest(t, "testcol-allfail",
		[]string{"repo1", "repo2"},
		map[string]bool{"repo1": true, "repo2": true},
	)

	dest := t.TempDir()

	oldDest, oldDryRun := cloneDest, cloneDryRun
	t.Cleanup(func() { cloneDest = oldDest; cloneDryRun = oldDryRun })
	cloneDest = dest
	cloneDryRun = false

	stderr := captureStderr(func() {
		captureStdout(func() {
			_ = runClone(nil, []string{"testcol-allfail"})
		})
	})

	if !strings.Contains(stderr, "sync") {
		t.Errorf("expected sync suggestion in stderr when all clones fail, got: %q", stderr)
	}
}

// TestClone_AllSucceed_NoSuggestion verifies no failure-branch sync suggestion
// when nothing fails (dry-run exits before any failure check).
func TestClone_AllSucceed_NoSuggestion(t *testing.T) {
	setupGitTest(t, "testcol-nosugg")

	oldDest, oldDryRun := cloneDest, cloneDryRun
	t.Cleanup(func() { cloneDest = oldDest; cloneDryRun = oldDryRun })
	cloneDest = t.TempDir()
	cloneDryRun = true // dry-run: no real clone, no failures

	stderr := captureStderr(func() {
		captureStdout(func() {
			if err := runClone(nil, []string{"testcol-nosugg"}); err != nil {
				t.Errorf("runClone dry-run = %v", err)
			}
		})
	})

	// dry-run exits early before the failure branch — no failure-branch suggestion
	if strings.Contains(stderr, "gitcollect sync testcol-nosugg --dest") {
		t.Errorf("expected no failure-branch sync suggestion in dry-run, got: %q", stderr)
	}
}
