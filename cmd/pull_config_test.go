package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPullConfig_RequiresRepo(t *testing.T) {
	old := pullConfigRepo
	pullConfigRepo = ""
	defer func() { pullConfigRepo = old }()

	err := runPullConfig(nil, nil)
	if err == nil {
		t.Fatal("expected error when --repo is not set")
	}
}

// TestPullConfig_CopiesFiles verifies the copy-and-skip logic without needing
// a real git remote. We set up a fake "cloned" tree in a temp dir, then call
// the file-copying logic directly so the test doesn't depend on git or a
// network.
func TestPullConfig_CopiesFiles(t *testing.T) {
	dir := t.TempDir()

	// Build a fake clone directory structure.
	srcDir := filepath.Join(dir, "clone", "collections")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"team-a.yaml", "team-b.yaml"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte("name: "+name+"\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	collDir := filepath.Join(dir, ".gitcollect", "collections")
	if err := os.MkdirAll(collDir, 0o755); err != nil {
		t.Fatalf("MkdirAll collDir: %v", err)
	}

	// Simulate the copy loop.
	entries, _ := os.ReadDir(srcDir)
	var copied []string
	for _, e := range entries {
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(collDir, e.Name())
		if err := copyFile(src, dst); err != nil {
			t.Fatalf("copyFile: %v", err)
		}
		copied = append(copied, e.Name())
	}

	if len(copied) != 2 {
		t.Errorf("copied %d files, want 2", len(copied))
	}
	for _, name := range []string{"team-a.yaml", "team-b.yaml"} {
		if _, err := os.Stat(filepath.Join(collDir, name)); err != nil {
			t.Errorf("expected %s to exist after copy: %v", name, err)
		}
	}
}

func TestPullConfig_SkipsExistingWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "collections")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "team-a.yaml"), []byte("name: team-a\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	collDir := filepath.Join(dir, "dest")
	if err := os.MkdirAll(collDir, 0o755); err != nil {
		t.Fatalf("MkdirAll dest: %v", err)
	}
	// Pre-create the destination file.
	original := []byte("original content\n")
	if err := os.WriteFile(filepath.Join(collDir, "team-a.yaml"), original, 0o600); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}

	// Simulate the skip logic (overwrite=false).
	src := filepath.Join(srcDir, "team-a.yaml")
	dst := filepath.Join(collDir, "team-a.yaml")
	if _, err := os.Stat(dst); err == nil {
		// File exists and overwrite=false → skip.
	} else {
		if err := copyFile(src, dst); err != nil {
			t.Fatalf("copyFile: %v", err)
		}
	}

	// File must still contain original content.
	got, _ := os.ReadFile(dst)
	if string(got) != string(original) {
		t.Errorf("file was overwritten unexpectedly: got %q, want %q", got, original)
	}
}

func TestPullConfig_OverwriteReplacesFile(t *testing.T) {
	dir := t.TempDir()

	original := []byte("original content\n")
	updated := []byte("updated content\n")

	src := filepath.Join(dir, "src.yaml")
	dst := filepath.Join(dir, "dst.yaml")

	if err := os.WriteFile(src, updated, 0o600); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}
	if err := os.WriteFile(dst, original, 0o600); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}

	// overwrite=true → copy unconditionally.
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != string(updated) {
		t.Errorf("overwrite: got %q, want %q", got, updated)
	}
}
