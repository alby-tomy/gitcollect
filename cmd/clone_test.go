package cmd

import (
	"os"
	"path/filepath"
	"reflect"
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
