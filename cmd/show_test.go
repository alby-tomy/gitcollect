package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func TestBuildShowRepoRows(t *testing.T) {
	repos := []collection.RepoAccess{
		{Name: "open-repo", Groups: []string{}, Users: []string{}},
		{Name: "group-repo", Groups: []string{"red-team"}},
		{Name: "user-repo", Users: []string{"alice"}},
	}
	details := []access.RepoAccessDetail{
		{RepoName: "open-repo", CanAccess: true, Reason: "open to all members"},
		{RepoName: "group-repo", CanAccess: false, Reason: "no access — group red-team required"},
		{RepoName: "user-repo", CanAccess: true, Reason: "individually granted"},
	}

	rows, denied := buildShowRepoRows(repos, details)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if len(denied) != 1 || denied[0] != "group-repo" {
		t.Fatalf("expected only group-repo to be denied, got %v", denied)
	}

	if rows[0][2] != "✓ yes" {
		t.Errorf("expected open-repo's YOU column to be granted, got %q", rows[0][2])
	}
	if rows[1][2] != "✗ no — no access — group red-team required" {
		t.Errorf("expected group-repo's YOU column to show the denial reason, got %q", rows[1][2])
	}
	if rows[2][2] != "✓ yes" {
		t.Errorf("expected user-repo's YOU column to be granted, got %q", rows[2][2])
	}

	if rows[0][1] != "open to all members" {
		t.Errorf("expected open-repo's rule column, got %q", rows[0][1])
	}
	if rows[1][1] != "groups: [red-team]" {
		t.Errorf("expected group-repo's rule column, got %q", rows[1][1])
	}
	if rows[2][1] != "users: [alice]" {
		t.Errorf("expected user-repo's rule column, got %q", rows[2][1])
	}
}

func TestBuildShowRepoRows_Empty(t *testing.T) {
	rows, denied := buildShowRepoRows(nil, nil)
	if len(rows) != 0 || len(denied) != 0 {
		t.Fatalf("expected empty output for empty input, got rows=%v denied=%v", rows, denied)
	}
}

func TestToShowOutput_OwnerNotListedAsMember(t *testing.T) {
	col, err := collection.New("acme", "github.com", "owner", collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	// Deliberately leave the owner out of col.Members — this used to make
	// UserAccessMap report false for the owner on every repo (see
	// internal/access's decide() fix).
	col.Repos = []collection.RepoAccess{
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	out := toShowOutput(col, "owner")
	if len(out.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(out.Repos))
	}
	if !out.Repos[0].YouCanAccess {
		t.Errorf("expected the owner to access their own collection's repo even though not a listed member")
	}
	if out.Repos[0].YouReason != "owner" {
		t.Errorf("expected reason %q, got %q", "owner", out.Repos[0].YouReason)
	}
}
