package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

func setupStatusTest(t *testing.T, collName string) (dest string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	mock.collabs["owner/repo1/owner"] = true
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	col, err := collection.New(collName, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Repos = []collection.RepoAccess{{Name: "repo1", Groups: []string{}, Users: []string{}}}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	dest = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "repo1"), 0o755); err != nil {
		t.Fatalf("MkdirAll repo1: %v", err)
	}
	return dest
}

func resetStatusFlags() func() {
	oldDest := statusDest
	oldVerbose := statusVerbose
	oldAll := statusAll
	oldStatusFn := statusGitStatusFn
	oldBehindFn := statusGitCommitsBehindFn
	return func() {
		statusDest = oldDest
		statusVerbose = oldVerbose
		statusAll = oldAll
		statusGitStatusFn = oldStatusFn
		statusGitCommitsBehindFn = oldBehindFn
	}
}

func TestStatus_CleanRepo(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-clean")
	statusDest = dest
	statusGitStatusFn = func(dir string) (string, error) { return "", nil }
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 0, nil }

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-clean"}); err != nil {
				t.Fatalf("runStatus: %v", err)
			}
		})
	})

	if !strings.Contains(out, "clean") {
		t.Errorf("expected 'clean' in stdout table, got: %q", out)
	}
}

func TestStatus_DirtyRepo(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-dirty")
	statusDest = dest
	statusGitStatusFn = func(dir string) (string, error) {
		return "M  file1.go\nM  file2.go", nil
	}
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 0, nil }

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-dirty"}); err != nil {
				t.Fatalf("runStatus: %v", err)
			}
		})
	})

	if !strings.Contains(out, "change") {
		t.Errorf("expected 'change' in stdout table, got: %q", out)
	}
}

func TestStatus_BehindOrigin(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-behind")
	statusDest = dest
	statusGitStatusFn = func(dir string) (string, error) { return "", nil }
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 3, nil }

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-behind"}); err != nil {
				t.Fatalf("runStatus: %v", err)
			}
		})
	})

	if !strings.Contains(out, "3") {
		t.Errorf("expected BEHIND=3 in table, got: %q", out)
	}
}

func TestStatus_BothDirtyAndBehind(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-both")
	statusDest = dest
	statusGitStatusFn = func(dir string) (string, error) {
		return "M  file.go", nil
	}
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 2, nil }

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-both"}); err != nil {
				t.Fatalf("runStatus: %v", err)
			}
		})
	})

	if !strings.Contains(out, "change") {
		t.Errorf("expected 'change' in stdout table, got: %q", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("expected BEHIND=2 in stdout table, got: %q", out)
	}
}

func TestStatus_SummaryLine(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-sum")
	statusDest = dest
	statusGitStatusFn = func(dir string) (string, error) { return "", nil }
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 0, nil }

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-sum"}); err != nil {
				t.Fatalf("runStatus: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Checked") {
		t.Errorf("expected summary line with 'Checked' in stdout, got: %q", out)
	}
}

func TestStatus_Verbose_RawOutput(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-verbose")
	statusDest = dest
	statusVerbose = true
	statusGitStatusFn = func(dir string) (string, error) {
		return "M  rawfile.go\n?? newfile.txt", nil
	}
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 0, nil }

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-verbose"}); err != nil {
				t.Fatalf("runStatus --verbose: %v", err)
			}
		})
	})

	if !strings.Contains(out, "rawfile.go") {
		t.Errorf("expected raw git output in stdout when --verbose, got: %q", out)
	}
}

func TestStatus_NoRemote_ZeroBehind(t *testing.T) {
	defer resetStatusFlags()()
	dest := setupStatusTest(t, "st-noremote")
	statusDest = dest
	statusGitStatusFn = func(dir string) (string, error) { return "", nil }
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 0, nil } // no remote → 0

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runStatus(nil, []string{"st-noremote"}); err != nil {
				t.Fatalf("runStatus: %v", err)
			}
		})
	})

	// BEHIND column should show "0" for no-remote repos
	if !strings.Contains(out, "0") {
		t.Errorf("expected BEHIND=0 when no remote, got: %q", out)
	}
}

func TestStatusAll_SummaryPerCollection(t *testing.T) {
	defer resetStatusFlags()()
	statusGitStatusFn = func(dir string) (string, error) { return "", nil }
	statusGitCommitsBehindFn = func(dir string) (int, error) { return 0, nil }

	// Setup two collections
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	mock.collabs["owner/repo1/owner"] = true
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	for _, name := range []string{"sa-col-1", "sa-col-2"} {
		col, err := collection.New(name, "github.com",
			api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
		if err != nil {
			t.Fatalf("collection.New(%s): %v", name, err)
		}
		col.Repos = []collection.RepoAccess{{Name: "repo1", Groups: []string{}, Users: []string{}}}
		if err := col.Save(); err != nil {
			t.Fatalf("col.Save(%s): %v", name, err)
		}
	}

	dest := t.TempDir()
	statusAll = true
	statusDest = dest

	stderr := captureStderr(func() {
		captureStdout(func() {
			if err := runStatus(nil, []string{}); err != nil {
				t.Fatalf("runStatus --all: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "sa-col-1") || !strings.Contains(stderr, "sa-col-2") {
		t.Errorf("expected both collection names in --all output, got: %q", stderr)
	}
}
