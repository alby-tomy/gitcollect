package cmd

import (
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// multiAddMock is a concurrency-safe, in-memory api.Client used to exercise
// addOneMember/addOneToGroup/addOneRepo (the per-item helpers behind the
// member add / group add / add commands' multi-value support) without any
// network access. Unlike pendingInviteMock above, it actually tracks
// collaborator state so SyncCollaborators' add/check logic has something
// real to operate on, and lets a test force one specific username's
// AddCollaborator call to fail — needed to verify that a batch continues
// past one bad item instead of aborting the whole run.
type multiAddMock struct {
	mu         sync.Mutex
	collabs    map[string]bool // "owner/repo/username" -> has access
	failAddFor map[string]bool // username -> AddCollaborator fails for them
}

func newMultiAddMock() *multiAddMock {
	return &multiAddMock{collabs: map[string]bool{}, failAddFor: map[string]bool{}}
}

func (m *multiAddMock) key(owner, repo, username string) string {
	return owner + "/" + repo + "/" + username
}

func (m *multiAddMock) GetRepo(owner, repo string) (api.RepoInfo, error) {
	return api.RepoInfo{Name: repo, CloneURL: "https://example.com/" + owner + "/" + repo + ".git"}, nil
}
func (m *multiAddMock) GetAuthenticatedUser() (api.UserInfo, error) {
	return api.UserInfo{ID: "owner", Login: "owner"}, nil
}
func (m *multiAddMock) GetUser(username string) (api.UserInfo, error) {
	return api.UserInfo{ID: username, Login: username}, nil
}
func (m *multiAddMock) AddCollaborator(owner, repo, username, permission string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAddFor[username] {
		return errors.New("mock add failure")
	}
	m.collabs[m.key(owner, repo, username)] = true
	return nil
}
func (m *multiAddMock) RemoveCollaborator(owner, repo, username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.collabs, m.key(owner, repo, username))
	return nil
}
func (m *multiAddMock) CheckCollaborator(owner, repo, username string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.collabs[m.key(owner, repo, username)], nil
}
func (m *multiAddMock) GetPendingInvite(owner, repo, username string) (bool, error) {
	return false, nil
}
func (m *multiAddMock) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *multiAddMock) Host() string { return "github.com" }

// pendingInviteMock is a minimal api.Client stub for exercising
// hasPendingInvite without any network access — every method except
// GetPendingInvite is a fixed, unused stub.
type pendingInviteMock struct {
	pending map[string]bool // "repo/username" -> has a pending invite
}

func (m *pendingInviteMock) GetRepo(owner, repo string) (api.RepoInfo, error) {
	return api.RepoInfo{}, nil
}
func (m *pendingInviteMock) GetAuthenticatedUser() (api.UserInfo, error) {
	return api.UserInfo{}, nil
}
func (m *pendingInviteMock) GetUser(username string) (api.UserInfo, error) {
	return api.UserInfo{ID: username, Login: username}, nil
}
func (m *pendingInviteMock) AddCollaborator(owner, repo, username, permission string) error {
	return nil
}
func (m *pendingInviteMock) RemoveCollaborator(owner, repo, username string) error { return nil }
func (m *pendingInviteMock) CheckCollaborator(owner, repo, username string) (bool, error) {
	return false, nil
}
func (m *pendingInviteMock) GetPendingInvite(owner, repo, username string) (bool, error) {
	return m.pending[repo+"/"+username], nil
}
func (m *pendingInviteMock) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *pendingInviteMock) Host() string { return "github.com" }

func TestHasPendingInvite(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	client := &pendingInviteMock{pending: map[string]bool{"repo2/bob": true}}

	if hasPendingInvite(col, "bob", []string{"repo1"}, client) {
		t.Error("expected no pending invite when none of the granted repos have one")
	}
	if !hasPendingInvite(col, "bob", []string{"repo1", "repo2"}, client) {
		t.Error("expected a pending invite to be found in the granted list")
	}
	if hasPendingInvite(col, "bob", nil, client) {
		t.Error("expected no pending invite for an empty granted list")
	}
}

// TestAddOneMember exercises the per-username helper behind member add's
// multi-value support: a fresh add succeeds, re-adding an existing member is
// a no-op (not an error), and a sync failure surfaces as an error without
// leaving the username added — covering the same three outcomes runMemberAdd
// loops over for a batch of usernames.
func TestAddOneMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	client := newMultiAddMock()

	if err := addOneMember(col, "acme", "owner", "alice", client); err != nil {
		t.Fatalf("addOneMember(alice) = %v, want nil", err)
	}
	if !col.IsMember("alice") {
		t.Error("expected alice to be a member after addOneMember")
	}

	if err := addOneMember(col, "acme", "owner", "alice", client); err != nil {
		t.Errorf("addOneMember(alice again) = %v, want nil (already a member is a no-op)", err)
	}

	client.failAddFor["bob"] = true
	col.Repos = []collection.RepoAccess{{Name: "r", Groups: []string{}, Users: []string{}}}
	if err := addOneMember(col, "acme", "owner", "bob", client); err == nil {
		t.Fatal("addOneMember(bob) = nil, want an error from the failing sync")
	}
	if col.IsMember("bob") {
		t.Error("expected bob NOT to be added to Members after a failed sync (AddMember rolls back)")
	}
}

// TestGroupsForMember exercises the groupsForMember helper (used by member
// list and printAccessSummary) with a member in multiple groups, one group,
// and no groups at all.
func TestGroupsForMember(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id", "bob-id"}
	col.Logins["alice-id"] = "alice"
	col.Logins["bob-id"] = "bob"
	col.Groups = map[string][]string{
		"red-team":  {"alice-id"},
		"blue-team": {"alice-id", "bob-id"},
	}

	aliceGroups := groupsForMember(col, "alice-id")
	sort.Strings(aliceGroups)
	if len(aliceGroups) != 2 || aliceGroups[0] != "blue-team" || aliceGroups[1] != "red-team" {
		t.Errorf("groupsForMember(alice-id) = %v, want [blue-team red-team]", aliceGroups)
	}

	bobGroups := groupsForMember(col, "bob-id")
	if len(bobGroups) != 1 || bobGroups[0] != "blue-team" {
		t.Errorf("groupsForMember(bob-id) = %v, want [blue-team]", bobGroups)
	}

	charlieGroups := groupsForMember(col, "charlie-id")
	if len(charlieGroups) != 0 {
		t.Errorf("groupsForMember(charlie-id) = %v, want [] (not in any group)", charlieGroups)
	}
}

// TestAddOneMember_BatchContinuesPastFailure verifies that a batch of
// usernames continues processing after one fails — the per-item helper
// returns the error instead of panicking or short-circuiting the loop
// that runMemberAdd drives.
func TestAddOneMember_BatchContinuesPastFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Repos = []collection.RepoAccess{{Name: "r", Groups: []string{}, Users: []string{}}}

	client := newMultiAddMock()
	client.failAddFor["charlie"] = true

	var failed []string
	for _, username := range []string{"alice", "charlie", "bob"} {
		if err := addOneMember(col, "acme", "owner", username, client); err != nil {
			failed = append(failed, username)
		}
	}

	if len(failed) != 1 || failed[0] != "charlie" {
		t.Errorf("failed = %v, want [charlie]", failed)
	}
	if !col.IsMember("alice") {
		t.Error("alice should be a member (added before the failure)")
	}
	if col.IsMember("charlie") {
		t.Error("charlie should NOT be a member (sync failed)")
	}
	if !col.IsMember("bob") {
		t.Error("bob should be a member (processing continued past charlie's failure)")
	}
}
