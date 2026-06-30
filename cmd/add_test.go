package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/collection"
)

// TestAddOneRepo exercises the per-repo helper behind the add command's
// multi-value support: a fresh repo succeeds, a repo already in the
// collection is rejected, and a sync failure surfaces an error without
// leaving the repo in col.Repos — covering the same three outcomes runAdd
// loops over for a batch of repo names.
func TestAddOneRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("acme", "github.com", "owner", collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	client := newMultiAddMock()

	if err := addOneRepo(col, "acme", "owner", "repo1", client); err != nil {
		t.Fatalf("addOneRepo(repo1) = %v, want nil", err)
	}
	if len(col.Repos) != 1 || col.Repos[0].Name != "repo1" {
		t.Fatalf("expected repo1 to be added, got %+v", col.Repos)
	}

	if err := addOneRepo(col, "acme", "owner", "repo1", client); err == nil {
		t.Error("addOneRepo(repo1 again) = nil, want an error (already in the collection)")
	}

	col.Members = []string{"bob"}
	client.failAddFor["bob"] = true
	if err := addOneRepo(col, "acme", "owner", "repo2", client); err == nil {
		t.Fatal("addOneRepo(repo2) = nil, want an error from the failing sync")
	}
	for _, r := range col.Repos {
		if r.Name == "repo2" {
			t.Error("expected repo2 NOT to remain in col.Repos after a failed sync (rolled back)")
		}
	}
}
