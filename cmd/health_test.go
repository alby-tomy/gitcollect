package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// healthMock answers CheckCollaborator affirmatively for every repo it is
// seeded with, and serves a canned open-PR count per repo.
type healthMock struct {
	multiAddMock
	prs map[string][]api.PRInfo
}

func (m *healthMock) ListOpenPRs(_, repo string) ([]api.PRInfo, error) {
	return m.prs[repo], nil
}

// setupHealthTest builds a collection owned by the caller containing every
// name in repos, grants the owner platform access to each (so
// FilterAccessible keeps them), and points --dest at a scratch directory.
// Repos named in cloned get a directory created there; the rest look
// un-cloned. Returns that directory.
func setupHealthTest(t *testing.T, repos, cloned []string, prs map[string][]api.PRInfo) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	col, err := collection.New("acme", "github.com",
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

	mock := &healthMock{multiAddMock: *newMultiAddMock(), prs: prs}
	// RepoNamespace falls back to the owner's login when no namespace is
	// set, so that is the owner the collaborator check will be made under.
	for _, r := range repos {
		if err := mock.AddCollaborator("owner", r, "owner", api.PermissionPull); err != nil {
			t.Fatalf("seed collaborator: %v", err)
		}
	}

	dest := t.TempDir()
	for _, r := range cloned {
		if err := os.MkdirAll(filepath.Join(dest, r), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", r, err)
		}
	}

	prevDest := healthDest
	healthDest = dest
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		healthDest = prevDest
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})
	return dest
}

func TestHealth_CountsClonedVersusAccessible(t *testing.T) {
	setupHealthTest(t, []string{"api", "web", "docs"}, []string{"api", "web"}, nil)

	out := captureStdout(func() {
		if err := runHealth(nil, []string{"acme"}); err != nil {
			t.Fatalf("runHealth: %v", err)
		}
	})

	if !strings.Contains(out, "Cloned locally   : 2 / 3") {
		t.Errorf("expected 2 of 3 repos reported as cloned, got:\n%s", out)
	}
	if !strings.Contains(out, "Repos accessible : 3 / 3") {
		t.Errorf("expected all 3 repos accessible, got:\n%s", out)
	}
}

func TestHealth_TotalsOpenPRsAcrossRepos(t *testing.T) {
	setupHealthTest(t, []string{"api", "web"}, []string{"api"}, map[string][]api.PRInfo{
		"api": {{Number: 1}, {Number: 2}},
		"web": {{Number: 9}},
	})

	out := captureStdout(func() {
		if err := runHealth(nil, []string{"acme"}); err != nil {
			t.Fatalf("runHealth: %v", err)
		}
	})

	if !strings.Contains(out, "Open PRs/MRs     : 3") {
		t.Errorf("expected 3 open PRs in total, got:\n%s", out)
	}
}

func TestHealth_ListsEveryRepoInTheTable(t *testing.T) {
	setupHealthTest(t, []string{"api", "web"}, []string{"api"}, nil)

	out := captureStdout(func() {
		if err := runHealth(nil, []string{"acme"}); err != nil {
			t.Fatalf("runHealth: %v", err)
		}
	})

	for _, want := range []string{"REPO", "CLONED", "OPEN PRS", "api", "web"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in health output, got:\n%s", want, out)
		}
	}
}

func TestHealth_EmptyCollection(t *testing.T) {
	setupHealthTest(t, nil, nil, nil)

	out := captureStdout(func() {
		if err := runHealth(nil, []string{"acme"}); err != nil {
			t.Fatalf("runHealth on an empty collection: %v", err)
		}
	})

	if !strings.Contains(out, "Repos accessible : 0 / 0") {
		t.Errorf("expected a zero summary, got:\n%s", out)
	}
}

func TestHealth_UnknownCollection(t *testing.T) {
	setupHealthTest(t, []string{"api"}, nil, nil)

	if err := runHealth(nil, []string{"nope"}); err == nil {
		t.Fatal("expected an error for an unknown collection")
	}
}
