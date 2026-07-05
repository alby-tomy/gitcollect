package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func setupFindTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func makeFindCollection(t *testing.T, name string, repos []string) {
	t.Helper()
	col, err := collection.New(name, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New(%s): %v", name, err)
	}
	for _, r := range repos {
		col.Repos = append(col.Repos, collection.RepoAccess{Name: r})
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save(%s): %v", name, err)
	}
}

func resetFindFlags() func() {
	old := findJSON
	findJSON = false
	return func() { findJSON = old }
}

func TestFind_FoundInOneCollection(t *testing.T) {
	defer resetFindFlags()()
	setupFindTest(t)
	makeFindCollection(t, "alpha", []string{"my-repo", "other-repo"})

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runFind(nil, []string{"my-repo"}); err != nil {
				t.Fatalf("runFind: %v", err)
			}
		})
	})

	if !strings.Contains(out, "alpha") {
		t.Errorf("expected collection name 'alpha' in output, got: %q", out)
	}
}

func TestFind_FoundInMultiple(t *testing.T) {
	defer resetFindFlags()()
	setupFindTest(t)
	makeFindCollection(t, "col-a", []string{"shared-repo"})
	makeFindCollection(t, "col-b", []string{"shared-repo", "other"})

	var matchCount int
	captureStdout(func() {
		captureStderr(func() {
			if err := runFind(nil, []string{"shared-repo"}); err != nil {
				t.Fatalf("runFind: %v", err)
			}
		})
	})
	// just verifying no error when found in multiple
	_ = matchCount
}

func TestFind_NotFound(t *testing.T) {
	defer resetFindFlags()()
	setupFindTest(t)
	makeFindCollection(t, "mycol", []string{"repo-a", "repo-b"})

	var runErr error
	captureStdout(func() {
		captureStderr(func() {
			runErr = runFind(nil, []string{"nonexistent-repo"})
		})
	})
	if runErr == nil {
		t.Error("expected error for repo not found in any collection, got nil")
	}
}

func TestFind_JSON_ValidOutput(t *testing.T) {
	defer resetFindFlags()()
	findJSON = true
	setupFindTest(t)
	makeFindCollection(t, "proj", []string{"target-repo"})

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runFind(nil, []string{"target-repo"}); err != nil {
				t.Fatalf("runFind: %v", err)
			}
		})
	})

	var result findResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	if result.Repo != "target-repo" {
		t.Errorf("expected repo='target-repo', got %q", result.Repo)
	}
	if len(result.FoundIn) != 1 {
		t.Errorf("expected 1 match, got %d", len(result.FoundIn))
	}
}

func TestFind_JSON_NotFound(t *testing.T) {
	defer resetFindFlags()()
	findJSON = true
	setupFindTest(t)
	makeFindCollection(t, "proj", []string{"other-repo"})

	out := captureStdout(func() {
		captureStderr(func() {
			// --json mode exits 0 even when not found, returns empty found_in
			if err := runFind(nil, []string{"missing-repo"}); err != nil {
				t.Fatalf("runFind --json not-found: %v", err)
			}
		})
	})

	var result findResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	if len(result.FoundIn) != 0 {
		t.Errorf("expected empty found_in for missing repo, got %d entries", len(result.FoundIn))
	}
}

func TestFind_NoNetworkCall(t *testing.T) {
	defer resetFindFlags()()
	setupFindTest(t)
	makeFindCollection(t, "offline-col", []string{"offline-repo"})

	// cachedClient is intentionally nil — if runFind panics, a network call
	// was attempted. The test passing means find works without a client.
	saved := cachedClient
	cachedClient = nil
	t.Cleanup(func() { cachedClient = saved })

	captureStdout(func() {
		captureStderr(func() {
			if err := runFind(nil, []string{"offline-repo"}); err != nil {
				t.Fatalf("runFind with nil client: %v", err)
			}
		})
	})
}

func TestFind_WorksOffline(t *testing.T) {
	defer resetFindFlags()()
	setupFindTest(t)
	makeFindCollection(t, "wol-col", []string{"wol-repo"})

	// Run without any network credentials set.
	saved := cachedClient
	cachedClient = nil
	t.Cleanup(func() { cachedClient = saved })

	var runErr error
	captureStdout(func() {
		captureStderr(func() {
			runErr = runFind(nil, []string{"wol-repo"})
		})
	})
	if runErr != nil {
		t.Errorf("find should work offline, got error: %v", runErr)
	}
}
