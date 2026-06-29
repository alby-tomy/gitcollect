package config

import (
	"errors"
	"os"
	"runtime"
	"testing"
)

func useTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

func TestSaveAndLoadToken(t *testing.T) {
	useTempHome(t)

	if _, err := LoadToken("github.com"); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("expected ErrNotAuthenticated before any token is saved, got %v", err)
	}

	if err := SaveToken("github.com", "ghp_secret"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	token, err := LoadToken("github.com")
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if token != "ghp_secret" {
		t.Errorf("LoadToken = %q, want %q", token, "ghp_secret")
	}
}

func TestSaveAndLoadUser(t *testing.T) {
	useTempHome(t)

	user, err := LoadUser("github.com")
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if user != "" {
		t.Errorf("expected no cached user yet, got %q", user)
	}

	if err := SaveUser("github.com", "alice"); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	user, err = LoadUser("github.com")
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if user != "alice" {
		t.Errorf("LoadUser = %q, want %q", user, "alice")
	}
}

func TestHosts(t *testing.T) {
	useTempHome(t)

	if err := SaveToken("github.com", "t1"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	if err := SaveToken("gitlab.com", "t2"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	hosts, err := Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	found := map[string]bool{}
	for _, h := range hosts {
		found[h] = true
	}
	if !found["github.com"] || !found["gitlab.com"] {
		t.Fatalf("Hosts() = %v, want both github.com and gitlab.com", hosts)
	}
}

func TestConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permission bits are not meaningful on Windows")
	}

	home := useTempHome(t)
	if err := SaveToken("github.com", "secret"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat config file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected config file mode 0600, got %v", info.Mode().Perm())
	}

	dirInfo, err := os.Stat(home + string(os.PathSeparator) + ".gitcollect")
	if err != nil {
		t.Fatalf("Stat config dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("expected config dir mode 0700, got %v", dirInfo.Mode().Perm())
	}
}

func TestLoad_MalformedConfig(t *testing.T) {
	useTempHome(t)
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	if err := os.WriteFile(path, []byte("not: valid: yaml: [: :"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail on a malformed config file")
	}
}

func TestSaveToken_FailsWhenDirIsBlockedByAFile(t *testing.T) {
	useTempHome(t)
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	// Put a regular file where ~/.gitcollect should be, so EnsureDir's
	// MkdirAll fails.
	if err := os.WriteFile(dir, []byte("blocking"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := SaveToken("github.com", "t"); err == nil {
		t.Fatal("expected SaveToken to fail when ~/.gitcollect cannot be created")
	}
}

func TestEnsureDir_FailsWhenPathIsAFile(t *testing.T) {
	useTempHome(t)
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	blocked := dir + string(os.PathSeparator) + "blocked"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := EnsureDir(blocked + string(os.PathSeparator) + "child"); err == nil {
		t.Fatal("expected EnsureDir to fail when a path component is a file")
	}
}

func TestHomeResolutionFailure(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	if _, err := homeDir(); err == nil {
		t.Fatal("expected homeDir to fail with no home-related env vars set")
	}
	if _, err := Dir(); err == nil {
		t.Fatal("expected Dir to fail when homeDir fails")
	}
	if _, err := CollectionsDir(); err == nil {
		t.Fatal("expected CollectionsDir to fail when homeDir fails")
	}
	if _, err := AuditDir(); err == nil {
		t.Fatal("expected AuditDir to fail when homeDir fails")
	}
	if _, err := configPath(); err == nil {
		t.Fatal("expected configPath to fail when homeDir fails")
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when homeDir fails")
	}
	if err := SaveToken("github.com", "t"); err == nil {
		t.Fatal("expected SaveToken (via save) to fail when homeDir fails")
	}
}

func TestCollectionsDirAndAuditDir(t *testing.T) {
	useTempHome(t)

	collDir, err := CollectionsDir()
	if err != nil {
		t.Fatalf("CollectionsDir: %v", err)
	}
	auditDir, err := AuditDir()
	if err != nil {
		t.Fatalf("AuditDir: %v", err)
	}
	activityDir, err := ActivityDir()
	if err != nil {
		t.Fatalf("ActivityDir: %v", err)
	}
	if collDir == auditDir || collDir == activityDir || auditDir == activityDir {
		t.Errorf("expected CollectionsDir, AuditDir, and ActivityDir to all differ: %q, %q, %q", collDir, auditDir, activityDir)
	}

	if err := EnsureDir(collDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(collDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected CollectionsDir to exist after EnsureDir: err=%v info=%v", err, info)
	}
}
