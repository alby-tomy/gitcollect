package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// pendingInviteMock is a minimal api.Client stub for exercising
// hasPendingInvite without any network access — every method except
// GetPendingInvite is a fixed, unused stub.
type pendingInviteMock struct {
	pending map[string]bool // "repo/username" -> has a pending invite
}

func (m *pendingInviteMock) GetRepo(owner, repo string) (api.RepoInfo, error) {
	return api.RepoInfo{}, nil
}
func (m *pendingInviteMock) GetAuthenticatedUser() (string, error) { return "", nil }
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
	col, err := collection.New("acme", "github.com", "owner", collection.VisibilityPrivate)
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
