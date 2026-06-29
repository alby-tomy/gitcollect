package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFakeGit puts a fake "git" executable on PATH that appends every
// argument vector it receives, one line per invocation, to logPath. This
// lets tests assert on exactly what arguments git.go passed to the real git
// binary without ever shelling out to it.
func installFakeGit(t *testing.T, logPath string, exitNonZero bool) {
	t.Helper()
	dir := t.TempDir()

	script := "@echo off\r\necho %* >> \"" + logPath + "\"\r\n"
	if exitNonZero {
		script += "echo fake git failure 1>&2\r\nexit /b 1\r\n"
	} else {
		script += "echo ok\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "git.bat"), []byte(script), 0o755); err != nil {
		t.Fatalf("could not write fake git: %v", err)
	}

	t.Setenv("PATH", dir)
}

func readLog(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("could not read log: %v", err)
	}
	return string(data)
}

func TestCheckInstalled(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	if err := CheckInstalled(); err != nil {
		t.Errorf("expected fake git on PATH to satisfy CheckInstalled: %v", err)
	}
}

func TestCheckInstalled_Missing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir, no git anywhere on it

	if err := CheckInstalled(); err != ErrNotInstalled {
		t.Errorf("expected ErrNotInstalled, got %v", err)
	}
}

func TestClone_RejectsNonHTTPS(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	err := Clone("git@github.com:owner/repo.git", filepath.Join(t.TempDir(), "dest"))
	if err == nil || !strings.Contains(err.Error(), "non-HTTPS") {
		t.Fatalf("expected non-HTTPS rejection, got %v", err)
	}
	if log := readLog(t, logPath); log != "" {
		t.Errorf("expected no git subprocess to run for a rejected URL, got log: %q", log)
	}
}

func TestClone_PassesURLAndDest(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	dest := filepath.Join(t.TempDir(), "dest")
	if err := Clone("https://example.com/owner/repo.git", dest); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	log := readLog(t, logPath)
	if !strings.Contains(log, "clone https://example.com/owner/repo.git") || !strings.Contains(log, dest) {
		t.Errorf("expected log to record clone args, got %q", log)
	}
}

func TestClone_PropagatesFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, true)

	err := Clone("https://example.com/owner/repo.git", filepath.Join(t.TempDir(), "dest"))
	if err == nil || !strings.Contains(err.Error(), "fake git failure") {
		t.Fatalf("expected failure surfaced from stderr, got %v", err)
	}
}

func TestPull_PassesDir(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	dir := t.TempDir()
	if err := Pull(dir); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "pull") {
		t.Errorf("expected log to record pull, got %q", log)
	}
}

func TestStatus_ReturnsTrimmedOutput(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	out, err := Status(t.TempDir())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if out != "ok" {
		t.Errorf("expected trimmed stdout %q, got %q", "ok", out)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "status --short") {
		t.Errorf("expected log to record status --short, got %q", log)
	}
}
