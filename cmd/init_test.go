package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// captureMock wraps multiAddMock and records the owner argument passed to
// GetRepo — used by TestGetRepo_UsesNamespace to verify the namespace field
// routes to the right API path segment.
type captureMock struct {
	*multiAddMock
	lastGetRepoOwner string
}

func (m *captureMock) GetRepo(owner, repo string) (api.RepoInfo, error) {
	m.lastGetRepoOwner = owner
	return api.RepoInfo{Name: repo, CloneURL: "https://example.com/" + owner + "/" + repo + ".git"}, nil
}

// TestInit_AcceptsNamespaceFlag verifies that the --namespace cobra flag is
// wired to the initNamespace package var. Exercises the flag registration in
// init() without needing a real authenticated client.
func TestInit_AcceptsNamespaceFlag(t *testing.T) {
	initNamespace = ""
	if err := initCmd.Flags().Set("namespace", "acme-corp"); err != nil {
		t.Fatalf("Flags().Set(namespace): %v", err)
	}
	t.Cleanup(func() { initCmd.Flags().Set("namespace", "") }) //nolint:errcheck
	if initNamespace != "acme-corp" {
		t.Errorf("initNamespace = %q after flag set, want acme-corp", initNamespace)
	}
}

// TestGetRepo_UsesNamespace verifies that cloneOne passes col.Namespace (not
// col.Logins[col.Owner]) as the owner segment in the GetRepo API call — the
// core behavioural goal of the Priority 3 namespace fix.
func TestGetRepo_UsesNamespace(t *testing.T) {
	dir := t.TempDir()

	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "alice"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Namespace = "acme-corp"

	client := &captureMock{multiAddMock: newMultiAddMock()}

	if err := cloneOne(col, client, "my-repo", dir, true); err != nil {
		t.Fatalf("cloneOne(dryRun=true): %v", err)
	}
	if client.lastGetRepoOwner != "acme-corp" {
		t.Errorf("GetRepo owner = %q, want acme-corp (namespace should override owner login %q)",
			client.lastGetRepoOwner, col.Logins[col.Owner])
	}
}
