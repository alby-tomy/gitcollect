package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func TestPublish_RequiresRepo(t *testing.T) {
	old := publishRepo
	publishRepo = ""
	defer func() { publishRepo = old }()

	err := runPublish(nil, nil)
	if err == nil {
		t.Fatal("expected error when --repo is not set")
	}
}

func TestPublish_MissingCollection_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	old := publishRepo
	oldColl := publishCollection
	publishRepo = "acme-corp/gitcollect-config"
	publishCollection = "does-not-exist"
	defer func() {
		publishRepo = old
		publishCollection = oldColl
	}()

	err := runPublish(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing collection")
	}
}

func TestPublish_NoCollections_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	old := publishRepo
	oldColl := publishCollection
	publishRepo = "acme-corp/gitcollect-config"
	publishCollection = ""
	defer func() {
		publishRepo = old
		publishCollection = oldColl
	}()

	err := runPublish(nil, nil)
	if err == nil {
		t.Fatal("expected error when no collections exist")
	}
}

func TestPublish_RepoCloneURL(t *testing.T) {
	cases := []struct {
		host, ownerSlashRepo, want string
	}{
		{"github.com", "acme-corp/gitcollect-config", "https://github.com/acme-corp/gitcollect-config.git"},
		{"gitlab.com", "acme-corp/cfg", "https://gitlab.com/acme-corp/cfg.git"},
	}
	for _, tc := range cases {
		if got := repoCloneURL(tc.host, tc.ownerSlashRepo); got != tc.want {
			t.Errorf("repoCloneURL(%q, %q) = %q, want %q", tc.host, tc.ownerSlashRepo, got, tc.want)
		}
	}
}

func TestPublish_CopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.yaml")
	dst := filepath.Join(dir, "dst.yaml")

	content := []byte("name: test\n")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("copyFile content = %q, want %q", got, content)
	}
}

func TestPublish_CopyFile_MissingSrc(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nonexistent.yaml"), filepath.Join(dir, "dst.yaml"))
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

// TestPublish_CollectionFilter verifies that when --collection is set, only
// that collection's file would be included (via the filtering logic, not the
// actual git operations which require a real remote).
func TestPublish_CollectionFilter(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// Create two collections.
	for _, name := range []string{"team-a", "team-b"} {
		col, err := collection.New(name, "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
		if err != nil {
			t.Fatalf("New(%s): %v", name, err)
		}
		if err := col.Save(); err != nil {
			t.Fatalf("Save(%s): %v", name, err)
		}
	}

	// Run with --collection=team-a. This will fail at the git clone step,
	// but the collection-exists check happens before that. We verify the
	// error is about git, not about collection-not-found.
	old := publishRepo
	oldColl := publishCollection
	publishRepo = "acme-corp/gitcollect-config"
	publishCollection = "team-a"
	defer func() {
		publishRepo = old
		publishCollection = oldColl
	}()

	err := runPublish(nil, nil)
	// Expected to fail at git clone (no real remote). We only verify that the
	// error is not about "collection not found".
	if err != nil && err.Error() == "publish: collection \"team-a\" not found" {
		t.Errorf("should not fail on collection-not-found when team-a exists: %v", err)
	}
}
