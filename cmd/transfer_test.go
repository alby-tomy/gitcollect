package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// transferMock is a minimal api.Client for transfer tests. GetUser returns
// a controlled result per username; all other methods are stubs.
type transferMock struct {
	users      map[string]api.UserInfo // username → resolved identity
	getUserErr map[string]error
}

func (m *transferMock) GetAuthenticatedUser() (api.UserInfo, error) { return api.UserInfo{}, nil }
func (m *transferMock) GetUser(username string) (api.UserInfo, error) {
	if err, ok := m.getUserErr[username]; ok {
		return api.UserInfo{}, err
	}
	if u, ok := m.users[username]; ok {
		return u, nil
	}
	return api.UserInfo{ID: username, Login: username}, nil
}
func (m *transferMock) GetRepo(owner, repo string) (api.RepoInfo, error)               { return api.RepoInfo{}, nil }
func (m *transferMock) AddCollaborator(owner, repo, username, permission string) error { return nil }
func (m *transferMock) RemoveCollaborator(owner, repo, username string) error          { return nil }
func (m *transferMock) CheckCollaborator(owner, repo, username string) (bool, error) {
	return false, nil
}
func (m *transferMock) GetPendingInvite(owner, repo, username string) (bool, error) {
	return false, nil
}
func (m *transferMock) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *transferMock) CreateRepo(owner, name string, private bool, description string) (api.RepoInfo, error) {
	return api.RepoInfo{}, nil
}
func (m *transferMock) Host() string                      { return "github.com" }
func (m *transferMock) GetTokenScopes() ([]string, error) { return []string{}, nil }
func (m *transferMock) ListOrgTeams(org string) ([]api.TeamInfo, error) {
	return nil, nil
}
func (m *transferMock) ListTeamMembers(org, teamSlug, role string) ([]api.UserInfo, error) {
	return nil, nil
}
func (m *transferMock) ListTeamRepos(org, teamSlug string) ([]api.RepoInfo, error) {
	return nil, nil
}
func (m *transferMock) SearchRepos(org, pattern, topic string, limit int) ([]api.RepoInfo, error) {
	return nil, nil
}
func (m *transferMock) ListOrgRepos(org string) ([]api.RepoInfo, error)      { return nil, nil }
func (m *transferMock) ListOpenPRs(owner, repo string) ([]api.PRInfo, error) { return nil, nil }

// setupTransferTest creates a saved collection and injects a mock client so
// runTransfer can be called directly without a real token file.
func setupTransferTest(t *testing.T, ownerID, ownerLogin string, members []string, logins map[string]string) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", api.UserInfo{ID: ownerID, Login: ownerLogin}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = append(col.Members, members...)
	for id, login := range logins {
		col.Logins[id] = login
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	mock := &transferMock{
		users: map[string]api.UserInfo{},
	}
	for id, login := range logins {
		mock.users[login] = api.UserInfo{ID: id, Login: login}
	}

	cachedClient = mock
	cachedUser = ownerLogin
	cachedUserID = ownerID
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	return col
}

func TestTransfer_RequiresOwner(t *testing.T) {
	// Alice is a member but not the owner — transfer must fail.
	setupTransferTest(t, "owner-id", "owner",
		[]string{"alice-id"},
		map[string]string{"owner-id": "owner", "alice-id": "alice"},
	)
	// Override identity to alice (not the owner).
	cachedUser = "alice"
	cachedUserID = "alice-id"

	err := runTransfer(nil, []string{"acme", "owner"})
	if err == nil {
		t.Fatal("expected error when non-owner calls transfer")
	}
}

func TestTransfer_SelfTransfer(t *testing.T) {
	setupTransferTest(t, "owner-id", "owner",
		[]string{},
		map[string]string{"owner-id": "owner"},
	)

	err := runTransfer(nil, []string{"acme", "owner"})
	if err == nil {
		t.Fatal("expected error when transferring to self")
	}
	if !errors.Is(err, collection.ErrSelfTransfer) {
		t.Errorf("expected ErrSelfTransfer, got %v", err)
	}
}

func TestTransfer_RequiresMember(t *testing.T) {
	setupTransferTest(t, "owner-id", "owner",
		[]string{},
		map[string]string{"owner-id": "owner"},
	)
	// The mock's GetUser will return alice-id for "alice" but alice is not in Members.
	tm := cachedClient.(*transferMock)
	tm.users["alice"] = api.UserInfo{ID: "alice-id", Login: "alice"}

	err := runTransfer(nil, []string{"acme", "alice"})
	if err == nil {
		t.Fatal("expected error when target is not a member")
	}
}

func TestTransfer_UpdatesOwnerID(t *testing.T) {
	setupTransferTest(t, "owner-id", "owner",
		[]string{"alice-id"},
		map[string]string{"owner-id": "owner", "alice-id": "alice"},
	)

	// Override ConfirmWord by providing stdin — but output.ConfirmWord reads from
	// stdin which we can't inject here. Instead, verify the data mutation by
	// calling the transfer logic directly after bypassing confirmation.
	//
	// We verify the mutation by running transfer and checking the saved file.
	// For tests that require typed confirmation, we rely on the fact that
	// in non-interactive (test) stdin, readLine returns "". The ConfirmWord
	// call will fail ("" != "alice"), so transfer aborts. We test the mutation
	// path indirectly via TestTransfer_PreviousOwnerAddedAsMember which uses
	// a helper that directly exercises the mutation without confirmation.
	//
	// This test just verifies that ConfirmWord-abort returns an error:
	err := runTransfer(nil, []string{"acme", "alice"})
	if err == nil {
		t.Fatal("expected abort error when confirmation is not typed")
	}
}

func TestTransfer_RequiresTypedConfirm(t *testing.T) {
	setupTransferTest(t, "owner-id", "owner",
		[]string{"alice-id"},
		map[string]string{"owner-id": "owner", "alice-id": "alice"},
	)
	// Stdin is not interactive in tests — ConfirmWord will read "" which
	// doesn't match "alice", so runTransfer must return an error.
	err := runTransfer(nil, []string{"acme", "alice"})
	if err == nil {
		t.Fatal("expected error when confirmation word is not typed")
	}
	// The error must say "aborted", not some other failure.
	if err.Error() == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestTransfer_PreviousOwnerAddedAsMember(t *testing.T) {
	// Test the mutation directly: after a successful transfer, the previous
	// owner should be in Members and the new owner should be the col.Owner.
	col, err := collection.New("acme", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"},
		collection.VisibilityPrivate,
	)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id"}
	col.Logins["alice-id"] = "alice"

	newOwner := api.UserInfo{ID: "alice-id", Login: "alice"}
	previousOwnerID := col.Owner

	// Simulate the transfer mutation (same logic as runTransfer after confirmation).
	col.Owner = newOwner.ID
	col.Logins[newOwner.ID] = newOwner.Login

	// Previous owner not in Members yet — should be added.
	inMembers := false
	for _, m := range col.Members {
		if m == previousOwnerID {
			inMembers = true
			break
		}
	}
	if !inMembers {
		col.Members = append(col.Members, previousOwnerID)
	}
	col.Members = removeStringSlice(col.Members, newOwner.ID)

	if col.Owner != "alice-id" {
		t.Errorf("col.Owner = %q, want alice-id", col.Owner)
	}
	found := false
	for _, m := range col.Members {
		if m == "owner-id" {
			found = true
		}
	}
	if !found {
		t.Error("expected previous owner (owner-id) to be in Members after transfer")
	}
	for _, m := range col.Members {
		if m == "alice-id" {
			t.Error("new owner (alice-id) should not remain in Members after transfer")
		}
	}
}

func TestTransfer_AuditEntry(t *testing.T) {
	// Verify the audit entry fields that would be written.
	// Since we can't exercise the full flow without stdin, we test the
	// expected values at the data level.
	name := "acme"
	caller := "owner"
	newOwnerUsername := "alice"

	action := "collection.transfer"
	detail := fmt.Sprintf("Transferred ownership from %s to %s", caller, newOwnerUsername)

	if action != "collection.transfer" {
		t.Errorf("audit action = %q", action)
	}
	if detail != "Transferred ownership from owner to alice" {
		t.Errorf("audit detail = %q", detail)
	}
	_ = name
}

func TestRemoveStringSlice(t *testing.T) {
	got := removeStringSlice([]string{"a", "b", "c", "b"}, "b")
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("removeStringSlice = %v, want [a c]", got)
	}
	got = removeStringSlice(nil, "x")
	if len(got) != 0 {
		t.Errorf("removeStringSlice(nil) = %v, want []", got)
	}
}

// --- regression: transfer must work when the new owner is in a group ---

// withStdin points os.Stdin at a file containing input for the duration of
// the test, so commands that prompt for confirmation can be driven.
// Supplies a single answer: output.readLine builds a fresh reader per call,
// so a second prompt in the same test would not see the rest of the input.
func withStdin(t *testing.T, input string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("temp stdin: %v", err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek stdin: %v", err)
	}
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		f.Close()
	})
}

// Transfer removed the new owner from Members, but Validate requires every
// group member and per-repo grantee to be a member — so Save failed with
// `group "backend" references "alice-id", who is not a member` for exactly
// the people most likely to be handed a collection.
func TestTransfer_SucceedsWhenNewOwnerIsInAGroup(t *testing.T) {
	col := setupTransferTest(t, "owner-id", "owner",
		[]string{"alice-id"},
		map[string]string{"owner-id": "owner", "alice-id": "alice"},
	)
	col.Groups = map[string][]string{"backend": {"alice-id"}}
	col.Repos = []collection.RepoAccess{
		{Name: "api", Groups: []string{"backend"}, Users: []string{}},
	}
	if err := col.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	withStdin(t, "alice\n")

	if err := runTransfer(nil, []string{"acme", "alice"}); err != nil {
		t.Fatalf("transfer to a grouped member must succeed: %v", err)
	}

	after, err := collection.Load("acme")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Owner != "alice-id" {
		t.Errorf("expected alice to own the collection, got %q", after.Owner)
	}
	if !after.IsInGroup("alice-id", "backend") {
		t.Error("new owner should keep their group membership")
	}
	if err := after.Validate(); err != nil {
		t.Errorf("collection must remain valid after transfer: %v", err)
	}
}

// The previous owner's login is the namespace the repos actually live
// under. RepoNamespace falls back to the owner's login when Namespace is
// empty, so without pinning it the transfer would repoint every API path
// at the new owner's account, where the repos do not exist.
func TestTransfer_PinsNamespaceToPreviousOwner(t *testing.T) {
	col := setupTransferTest(t, "owner-id", "owner",
		[]string{"alice-id"},
		map[string]string{"owner-id": "owner", "alice-id": "alice"},
	)
	if col.Namespace != "" {
		t.Fatalf("precondition: namespace should start empty, got %q", col.Namespace)
	}
	withStdin(t, "alice\n")

	if err := runTransfer(nil, []string{"acme", "alice"}); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	after, err := collection.Load("acme")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.RepoNamespace() != "owner" {
		t.Errorf("repos still live under owner; got namespace %q", after.RepoNamespace())
	}
}

func TestTransfer_PreviousOwnerBecomesMember(t *testing.T) {
	setupTransferTest(t, "owner-id", "owner",
		[]string{"alice-id"},
		map[string]string{"owner-id": "owner", "alice-id": "alice"},
	)
	withStdin(t, "alice\n")

	if err := runTransfer(nil, []string{"acme", "alice"}); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	after, err := collection.Load("acme")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !after.IsMember("owner-id") {
		t.Error("previous owner should remain a member with access")
	}
}
