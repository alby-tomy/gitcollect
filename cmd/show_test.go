package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// TestToShowOutput_OwnerVsMemberAccess verifies that the owner's callerID
// gives full access to every repo while a non-owner member's access is
// evaluated against the collection's rules — confirming the view-selection
// split driven by col.IsOwner(callerID).
func TestToShowOutput_OwnerVsMemberAccess(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id"}
	col.Logins["alice-id"] = "alice"
	col.Repos = []collection.RepoAccess{
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	ownerOut := toShowOutput(col, "owner", "owner-id")
	if !ownerOut.Repos[0].YouCanAccess {
		t.Errorf("owner should always have access; YouCanAccess = false")
	}

	aliceOut := toShowOutput(col, "alice", "alice-id")
	if aliceOut.Repos[0].YouCanAccess {
		t.Errorf("alice should not access a red-team-restricted repo she has no group for; YouCanAccess = true")
	}
	if aliceOut.Repos[0].YouReason == "" {
		t.Errorf("YouReason should explain why alice is denied, got empty string")
	}
}

func TestBuildShowRepoRows(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Logins["alice"] = "alice"
	repos := []collection.RepoAccess{
		{Name: "open-repo", Groups: []string{}, Users: []string{}},
		{Name: "group-repo", Groups: []string{"red-team"}},
		{Name: "user-repo", Users: []string{"alice"}},
	}
	details := []access.RepoAccessDetail{
		{RepoName: "open-repo", CanAccess: true, Reason: "open to all members"},
		{RepoName: "group-repo", CanAccess: false, Reason: "no access — group red-team required", FixCmd: "gitcollect group add acme red-team alice"},
		{RepoName: "user-repo", CanAccess: true, Reason: "individually granted"},
	}

	rows, denied := buildShowRepoRows(col, repos, details)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if len(denied) != 1 || denied[0].repo != "group-repo" {
		t.Fatalf("expected only group-repo to be denied, got %v", denied)
	}
	if denied[0].fixCmd != "gitcollect group add acme red-team alice" {
		t.Errorf("expected denied entry to carry the exact fix command, got %q", denied[0].fixCmd)
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
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	rows, denied := buildShowRepoRows(col, nil, nil)
	if len(rows) != 0 || len(denied) != 0 {
		t.Fatalf("expected empty output for empty input, got rows=%v denied=%v", rows, denied)
	}
}

func TestToShowOutput_OwnerNotListedAsMember(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	// Deliberately leave the owner out of col.Members — this used to make
	// UserAccessMap report false for the owner on every repo (see
	// internal/access's decide() fix).
	col.Repos = []collection.RepoAccess{
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	out := toShowOutput(col, "owner", "owner")
	if len(out.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(out.Repos))
	}
	if !out.Repos[0].YouCanAccess {
		t.Errorf("expected the owner to access their own collection's repo even though not a listed member")
	}
	if out.Repos[0].YouReason != "owner — full access" {
		t.Errorf("expected reason %q, got %q", "owner — full access", out.Repos[0].YouReason)
	}
	if out.Repos[0].YouFixCmd != "" {
		t.Errorf("expected no fix command for someone who already has access, got %q", out.Repos[0].YouFixCmd)
	}
}

func TestBuildOwnerShowRepoRows(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice", "bob", "charlie"}
	for _, login := range col.Members {
		col.Logins[login] = login
	}
	col.Groups = map[string][]string{"red-team": {"alice", "bob"}}
	col.Repos = []collection.RepoAccess{
		{Name: "open-repo", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}},
		{Name: "nobody-yet", Groups: []string{"empty-group"}},
	}
	col.Groups["empty-group"] = []string{}

	rows := buildOwnerShowRepoRows(col)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	if rows[0][2] != "alice, bob, charlie (3)" {
		t.Errorf("expected open-repo to list all 3 members, got %q", rows[0][2])
	}
	if rows[1][2] != "alice, bob (2)" {
		t.Errorf("expected restricted to list red-team's 2 members, got %q", rows[1][2])
	}
	if rows[2][2] != "—" {
		t.Errorf("expected nobody-yet to show the empty placeholder, got %q", rows[2][2])
	}
}
