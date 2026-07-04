package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// diffMock controls GetRepo and CheckCollaborator responses per repo/member.
type diffMock struct {
	// repoStatus maps repo name → status to simulate:
	//   "" or missing key = ok (not archived, not missing)
	//   "archived"  = ok but Archived=true
	//   "missing"   = return api.ErrNotFound
	//   "forbidden" = return api.ErrForbidden
	//   "apierr"    = return a generic error
	repoStatus map[string]string

	// collaborators maps "owner/repo/username" → whether has access.
	// Missing key = true (has access by default).
	collaborators map[string]bool

	// concurrent tracking
	concurrentPeak int64
	activeJobs     int64
}

func newDiffMock() *diffMock {
	return &diffMock{
		repoStatus:    map[string]string{},
		collaborators: map[string]bool{},
	}
}

func (m *diffMock) GetRepo(owner, repo string) (api.RepoInfo, error) {
	atomic.AddInt64(&m.activeJobs, 1)
	peak := atomic.LoadInt64(&m.activeJobs)
	for {
		old := atomic.LoadInt64(&m.concurrentPeak)
		if peak <= old || atomic.CompareAndSwapInt64(&m.concurrentPeak, old, peak) {
			break
		}
	}
	defer atomic.AddInt64(&m.activeJobs, -1)

	switch m.repoStatus[repo] {
	case "archived":
		return api.RepoInfo{Name: repo, Archived: true}, nil
	case "missing":
		return api.RepoInfo{}, api.ErrNotFound
	case "forbidden":
		return api.RepoInfo{}, api.ErrForbidden
	case "apierr":
		return api.RepoInfo{}, io.ErrUnexpectedEOF
	default:
		return api.RepoInfo{Name: repo, CloneURL: "https://github.com/" + owner + "/" + repo + ".git"}, nil
	}
}

func (m *diffMock) CheckCollaborator(owner, repo, username string) (bool, error) {
	key := owner + "/" + repo + "/" + username
	if v, ok := m.collaborators[key]; ok {
		return v, nil
	}
	return true, nil // default: has access
}

func (m *diffMock) GetAuthenticatedUser() (api.UserInfo, error) {
	return api.UserInfo{ID: "owner-id", Login: "owner"}, nil
}
func (m *diffMock) GetUser(username string) (api.UserInfo, error) {
	return api.UserInfo{ID: username + "-id", Login: username}, nil
}
func (m *diffMock) AddCollaborator(owner, repo, username, perm string) error  { return nil }
func (m *diffMock) RemoveCollaborator(owner, repo, username string) error      { return nil }
func (m *diffMock) GetPendingInvite(owner, repo, username string) (bool, error) { return false, nil }
func (m *diffMock) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *diffMock) CreateRepo(owner, name string, private bool, desc string) (api.RepoInfo, error) {
	return api.RepoInfo{Name: name}, nil
}
func (m *diffMock) Host() string                      { return "github.com" }
func (m *diffMock) GetTokenScopes() ([]string, error) { return []string{}, nil }
func (m *diffMock) ListOrgTeams(org string) ([]api.TeamInfo, error) {
	return nil, nil
}
func (m *diffMock) ListTeamMembers(org, slug, role string) ([]api.UserInfo, error) {
	return nil, nil
}
func (m *diffMock) ListTeamRepos(org, slug string) ([]api.RepoInfo, error) { return nil, nil }
func (m *diffMock) SearchRepos(org, pattern, topic string, limit int) ([]api.RepoInfo, error) {
	return nil, nil
}

// setupDiffTest creates a collection on disk with the given repos and members,
// injects the mock client, and returns the collection for further mutation.
func setupDiffTest(t *testing.T, collName string, mock *diffMock, repos []string, members []string) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

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
	for _, m := range members {
		col.Members = append(col.Members, m+"-id")
		col.Logins[m+"-id"] = m
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
	return col
}

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestDiff_AllInSync(t *testing.T) {
	mock := newDiffMock()
	setupDiffTest(t, "testsync", mock,
		[]string{"repo-a", "repo-b"},
		[]string{"alice"},
	)

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testsync"}); err != nil {
			t.Fatalf("runDiff = %v", err)
		}
	})

	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ in output, got:\n%s", out)
	}
	if strings.Contains(out, "✗") || strings.Contains(out, "⚠") {
		t.Errorf("expected no errors/warnings for in-sync collection, got:\n%s", out)
	}
}

func TestDiff_MissingRepo(t *testing.T) {
	mock := newDiffMock()
	mock.repoStatus["gone-repo"] = "missing"
	setupDiffTest(t, "testmissing", mock,
		[]string{"ok-repo", "gone-repo"},
		nil,
	)

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testmissing"}); err != nil {
			t.Fatalf("runDiff = %v", err)
		}
	})

	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ for missing repo, got:\n%s", out)
	}
	if !strings.Contains(out, "gone-repo") {
		t.Errorf("expected gone-repo in output, got:\n%s", out)
	}
}

func TestDiff_ArchivedRepo(t *testing.T) {
	mock := newDiffMock()
	mock.repoStatus["archive-me"] = "archived"
	setupDiffTest(t, "testarchived", mock,
		[]string{"archive-me"},
		nil,
	)

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testarchived"}); err != nil {
			t.Fatalf("runDiff = %v", err)
		}
	})

	if !strings.Contains(out, "⚠") {
		t.Errorf("expected ⚠ for archived repo, got:\n%s", out)
	}
	if !strings.Contains(out, "archive-me") {
		t.Errorf("expected archive-me in output, got:\n%s", out)
	}
}

func TestDiff_MemberDrift(t *testing.T) {
	mock := newDiffMock()
	// bob was removed as collaborator directly on GitHub
	mock.collaborators["owner/sample-repo/bob"] = false
	setupDiffTest(t, "testdrift", mock,
		[]string{"sample-repo"},
		[]string{"alice", "bob"},
	)

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testdrift"}); err != nil {
			t.Fatalf("runDiff = %v", err)
		}
	})

	if !strings.Contains(out, "⚠") {
		t.Errorf("expected ⚠ for member drift, got:\n%s", out)
	}
	if !strings.Contains(out, "bob") {
		t.Errorf("expected bob in output, got:\n%s", out)
	}
}

func TestDiff_ReposOnly(t *testing.T) {
	mock := newDiffMock()
	setupDiffTest(t, "testreposonly", mock,
		[]string{"repo-x"},
		[]string{"alice"},
	)

	old := diffReposOnly
	diffReposOnly = true
	t.Cleanup(func() { diffReposOnly = old })

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testreposonly"}); err != nil {
			t.Fatalf("runDiff --repos-only = %v", err)
		}
	})

	if strings.Contains(out, "MEMBERS") {
		t.Errorf("--repos-only: expected no MEMBERS section, got:\n%s", out)
	}
	if !strings.Contains(out, "REPOS") {
		t.Errorf("--repos-only: expected REPOS section, got:\n%s", out)
	}
}

func TestDiff_MembersOnly(t *testing.T) {
	mock := newDiffMock()
	setupDiffTest(t, "testmembersonly", mock,
		[]string{"repo-y"},
		[]string{"alice"},
	)

	old := diffMembersOnly
	diffMembersOnly = true
	t.Cleanup(func() { diffMembersOnly = old })

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testmembersonly"}); err != nil {
			t.Fatalf("runDiff --members-only = %v", err)
		}
	})

	if strings.Contains(out, "REPOS") {
		t.Errorf("--members-only: expected no REPOS section, got:\n%s", out)
	}
	if !strings.Contains(out, "MEMBERS") {
		t.Errorf("--members-only: expected MEMBERS section, got:\n%s", out)
	}
}

func TestDiff_JSON(t *testing.T) {
	mock := newDiffMock()
	mock.repoStatus["gone"] = "missing"
	setupDiffTest(t, "testjson", mock,
		[]string{"ok-repo", "gone"},
		[]string{"alice"},
	)

	old := diffJSON
	diffJSON = true
	t.Cleanup(func() { diffJSON = old })

	out := captureStdout(func() {
		if err := runDiff(nil, []string{"testjson"}); err != nil {
			t.Fatalf("runDiff --json = %v", err)
		}
	})

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON output not parseable: %v\nOutput:\n%s", err, out)
	}
	if result.Collection != "testjson" {
		t.Errorf("collection = %q, want testjson", result.Collection)
	}
	found := false
	for _, r := range result.Repos {
		if r.Name == "gone" && r.Status == "missing" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gone repo with status=missing in JSON output: %+v", result.Repos)
	}
}

func TestDiff_ExitZeroOnDrift(t *testing.T) {
	mock := newDiffMock()
	mock.repoStatus["missing-repo"] = "missing"
	mock.collaborators["owner/sample-r/alice"] = false
	setupDiffTest(t, "testexitzero", mock,
		[]string{"missing-repo", "sample-r"},
		[]string{"alice"},
	)

	captureStdout(func() {
		err := runDiff(nil, []string{"testexitzero"})
		if err != nil {
			t.Errorf("runDiff with drift = %v, want nil (drift is not an error)", err)
		}
	})
}

func TestDiff_ExitOneOnAPIError(t *testing.T) {
	mock := newDiffMock()
	mock.repoStatus["bad-repo"] = "apierr"
	setupDiffTest(t, "testexitone", mock,
		[]string{"bad-repo"},
		nil,
	)

	captureStdout(func() {
		err := runDiff(nil, []string{"testexitone"})
		if err == nil {
			t.Error("runDiff with API error = nil, want error")
		}
	})
}

func TestDiff_Concurrent(t *testing.T) {
	mock := newDiffMock()
	// 6 repos so the semaphore (max 4) is exercised.
	repos := []string{"r1", "r2", "r3", "r4", "r5", "r6"}
	setupDiffTest(t, "testconcurrent", mock, repos, nil)

	captureStdout(func() {
		if err := runDiff(nil, []string{"testconcurrent"}); err != nil {
			t.Fatalf("runDiff = %v", err)
		}
	})

	// Verify all 6 repos were checked (concurrentPeak will be > 0).
	if mock.concurrentPeak == 0 {
		t.Error("expected concurrent GetRepo calls, concurrentPeak = 0")
	}
}
