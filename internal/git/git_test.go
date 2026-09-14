package git

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeGit writes an executable fake "git" into dir and returns nothing —
// callers set PATH to dir themselves. Two scripts are required because the
// shell differs by platform: Windows resolves "git" to a git.bat run by cmd,
// every other platform needs a file literally named "git" with a shebang and
// the executable bit set. Keeping both in one helper means each test below
// describes the fake git's *behaviour* twice and its plumbing zero times.
func writeFakeGit(t *testing.T, dir, batScript, shScript string) {
	t.Helper()

	name, script := "git", shScript
	if runtime.GOOS == "windows" {
		name, script = "git.bat", batScript
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("could not write fake git: %v", err)
	}
	// os.WriteFile's perm is masked by umask, so set the executable bit
	// explicitly — exec.LookPath skips a non-executable file and the test
	// would fail with a confusing "git not found on PATH".
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("could not make fake git executable: %v", err)
	}
}

// installFakeGit puts a fake "git" executable on PATH that appends every
// argument vector it receives, one line per invocation, to logPath. This
// lets tests assert on exactly what arguments git.go passed to the real git
// binary without ever shelling out to it.
func installFakeGit(t *testing.T, logPath string, exitNonZero bool) {
	t.Helper()
	dir := t.TempDir()

	bat := "@echo off\r\necho %* >> \"" + logPath + "\"\r\n"
	sh := "#!/bin/sh\necho \"$*\" >> \"" + logPath + "\"\n"
	if exitNonZero {
		bat += "echo fake git failure 1>&2\r\nexit /b 1\r\n"
		sh += "echo 'fake git failure' >&2\nexit 1\n"
	} else {
		bat += "echo ok\r\nexit /b 0\r\n"
		sh += "echo ok\nexit 0\n"
	}
	writeFakeGit(t, dir, bat, sh)

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

// installFakeGitForPull puts a fake "git" on PATH that handles the exact
// sequence PullWithSummary issues: "rev-parse HEAD" (returns before, then
// after, in that order), "pull" (always succeeds), and
// "rev-list --count before..after" (returns revListCount). Used instead of
// installFakeGit because PullWithSummary needs different output across
// multiple invocations within one call, not a single fixed response.
func installFakeGitForPull(t *testing.T, before, after, revListCount string) {
	t.Helper()
	dir := t.TempDir()
	counterPath := filepath.Join(dir, "calls")

	bat := `@echo off
if "%1"=="rev-parse" (
  if not exist "` + counterPath + `" (
    echo x >> "` + counterPath + `"
    echo ` + before + `
  ) else (
    echo ` + after + `
  )
  exit /b 0
)
if "%1"=="pull" exit /b 0
if "%1"=="rev-list" (
  echo ` + revListCount + `
  exit /b 0
)
exit /b 0
`
	sh := `#!/bin/sh
case "$1" in
  rev-parse)
    if [ ! -f "` + counterPath + `" ]; then
      echo x >> "` + counterPath + `"
      echo "` + before + `"
    else
      echo "` + after + `"
    fi
    exit 0 ;;
  pull) exit 0 ;;
  rev-list) echo "` + revListCount + `"; exit 0 ;;
esac
exit 0
`
	writeFakeGit(t, dir, bat, sh)
	t.Setenv("PATH", dir)
}

func TestPullWithSummary_UpToDate(t *testing.T) {
	installFakeGitForPull(t, "commit-a", "commit-a", "0")

	n, err := PullWithSummary(t.TempDir())
	if err != nil {
		t.Fatalf("PullWithSummary: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 new commits when HEAD doesn't change, got %d", n)
	}
}

func TestPullWithSummary_NewCommits(t *testing.T) {
	installFakeGitForPull(t, "commit-a", "commit-b", "3")

	n, err := PullWithSummary(t.TempDir())
	if err != nil {
		t.Fatalf("PullWithSummary: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 new commits, got %d", n)
	}
}

func TestPullWithSummary_PropagatesPullFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, true) // every git invocation fails, including the first rev-parse

	if _, err := PullWithSummary(t.TempDir()); err == nil {
		t.Fatal("expected an error when the underlying git commands fail")
	}
}

func TestCommitsBehind_NoRemote(t *testing.T) {
	dir := t.TempDir()
	installFakeGit(t, filepath.Join(dir, "log.txt"), true) // exits non-zero = simulates no remote

	n, err := CommitsBehind(t.TempDir())
	if err != nil {
		t.Fatalf("CommitsBehind should return nil error when git fails: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 when no remote (git error), got %d", n)
	}
}

func TestCommitsBehind_BehindByN(t *testing.T) {
	tmpDir := t.TempDir()
	scriptDir := t.TempDir()

	bat := "@echo off\r\nif \"%1\"==\"rev-list\" (\r\n  echo 5\r\n  exit /b 0\r\n)\r\nexit /b 0\r\n"
	sh := "#!/bin/sh\nif [ \"$1\" = \"rev-list\" ]; then\n  echo 5\n  exit 0\nfi\nexit 0\n"
	writeFakeGit(t, scriptDir, bat, sh)
	t.Setenv("PATH", scriptDir)

	n, err := CommitsBehind(tmpDir)
	if err != nil {
		t.Fatalf("CommitsBehind: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 commits behind, got %d", n)
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

// --- token handling: a credential must never reach argv or the clone ---

func TestCredentialArgs_EmptyTokenAddsNothing(t *testing.T) {
	if got := credentialArgs(""); got != nil {
		t.Errorf("expected no -c flags without a token, got %v", got)
	}
	if got := tokenEnv(""); got != nil {
		t.Errorf("expected no env entry without a token, got %v", got)
	}
}

// The whole point of routing the token through the environment is that
// /proc/<pid>/cmdline is readable by other users while /proc/<pid>/environ
// is not. If the token ever appears in an argument, that protection is gone.
func TestCredentialArgs_TokenNeverAppearsInArgs(t *testing.T) {
	const token = "ghp_supersecrettokenvalue"
	for _, arg := range credentialArgs(token) {
		if strings.Contains(arg, token) {
			t.Fatalf("token leaked into git arguments: %q", arg)
		}
	}
	// It must reach git some other way, or authentication cannot work.
	env := tokenEnv(token)
	if len(env) != 1 || !strings.Contains(env[0], token) {
		t.Fatalf("expected the token to be carried in the environment, got %v", env)
	}
}

// An empty helper is prepended so a credential helper the user configured
// globally cannot answer first with a stale or wrong credential.
func TestCredentialArgs_ClearsInheritedHelper(t *testing.T) {
	args := credentialArgs("t")
	if len(args) < 2 || args[0] != "-c" || args[1] != "credential.helper=" {
		t.Errorf("expected the inherited credential helper to be cleared first, got %v", args)
	}
}

func TestCloneWithToken_DoesNotPassTokenToGit(t *testing.T) {
	const token = "ghp_supersecrettokenvalue"
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	dest := filepath.Join(t.TempDir(), "dest")
	if err := CloneWithToken("https://example.com/owner/repo.git", dest, token); err != nil {
		t.Fatalf("CloneWithToken: %v", err)
	}

	log := readLog(t, logPath)
	if strings.Contains(log, token) {
		t.Errorf("token appeared in git's argument vector: %q", log)
	}
	if !strings.Contains(log, "clone") {
		t.Errorf("expected the clone to run, got %q", log)
	}
}

func TestCloneWithToken_RejectsNonHTTPS(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	err := CloneWithToken("git@github.com:owner/repo.git", filepath.Join(t.TempDir(), "dest"), "tok")
	if err == nil || !strings.Contains(err.Error(), "non-HTTPS") {
		t.Fatalf("expected non-HTTPS rejection, got %v", err)
	}
}

// --depth=1 fetches only the branch it clones, so the branch has to be
// chosen during the clone rather than checked out afterwards.
func TestShallowCloneBranch_SelectsBranchDuringClone(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	dest := filepath.Join(t.TempDir(), "dest")
	if err := ShallowCloneBranch("https://example.com/o/r.git", dest, "release", ""); err != nil {
		t.Fatalf("ShallowCloneBranch: %v", err)
	}

	log := readLog(t, logPath)
	for _, want := range []string{"--depth=1", "--branch", "release"} {
		if !strings.Contains(log, want) {
			t.Errorf("expected %q in the git invocation, got %q", want, log)
		}
	}
	if strings.Contains(log, "checkout") {
		t.Errorf("branch must be selected by the clone, not a later checkout: %q", log)
	}
}

func TestShallowCloneBranch_EmptyBranchTakesDefault(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	if err := ShallowCloneBranch("https://example.com/o/r.git", filepath.Join(t.TempDir(), "d"), "", ""); err != nil {
		t.Fatalf("ShallowCloneBranch: %v", err)
	}
	if log := readLog(t, logPath); strings.Contains(log, "--branch") {
		t.Errorf("no --branch should be passed when none was requested: %q", log)
	}
}

func TestPushWithToken_DoesNotPassTokenToGit(t *testing.T) {
	const token = "glpat-anothersecret"
	logPath := filepath.Join(t.TempDir(), "log.txt")
	installFakeGit(t, logPath, false)

	if err := PushWithToken(t.TempDir(), token); err != nil {
		t.Fatalf("PushWithToken: %v", err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, token) {
		t.Errorf("token appeared in git's argument vector: %q", log)
	}
	if !strings.Contains(log, "push") {
		t.Errorf("expected the push to run, got %q", log)
	}
}
