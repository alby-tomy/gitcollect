package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

func setupInspectTest(t *testing.T) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("inspect-col", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPublic)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id", "bob-id"}
	col.Logins["alice-id"] = "alice"
	col.Logins["bob-id"] = "bob"
	col.Repos = []collection.RepoAccess{
		{Name: "api", Groups: []string{}, Users: []string{}},
		{Name: "frontend", Groups: []string{}, Users: []string{}},
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
	return col
}

func resetInspectFlags(t *testing.T) {
	t.Helper()
	oldUser, oldRepo, oldJSON := inspectUser, inspectRepo, inspectJSON
	t.Cleanup(func() { inspectUser = oldUser; inspectRepo = oldRepo; inspectJSON = oldJSON })
	inspectUser = ""
	inspectRepo = ""
	inspectJSON = false
}

// TestInspect_Matrix verifies the full matrix output lists all members and repos.
func TestInspect_Matrix(t *testing.T) {
	setupInspectTest(t)
	resetInspectFlags(t)

	out := captureStdout(func() {
		if err := runInspect(inspectCmd, []string{"inspect-col"}); err != nil {
			t.Fatalf("runInspect: %v", err)
		}
	})

	for _, want := range []string{"alice", "bob", "api", "frontend"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in matrix output, got: %q", want, out)
		}
	}
}

// TestInspect_ByRepo verifies --repo shows per-member access for that repo.
func TestInspect_ByRepo(t *testing.T) {
	setupInspectTest(t)
	resetInspectFlags(t)
	inspectRepo = "api"

	out := captureStdout(func() {
		if err := runInspect(inspectCmd, []string{"inspect-col"}); err != nil {
			t.Fatalf("runInspect --repo api: %v", err)
		}
	})

	if !strings.Contains(out, "api") {
		t.Errorf("expected repo name in output, got: %q", out)
	}
}

// TestInspect_ByRepo_NotFound verifies --repo returns an error for an unknown repo.
func TestInspect_ByRepo_NotFound(t *testing.T) {
	setupInspectTest(t)
	resetInspectFlags(t)
	inspectRepo = "does-not-exist"

	err := runInspect(inspectCmd, []string{"inspect-col"})
	if err == nil {
		t.Error("expected error for unknown repo, got nil")
	}
}

// TestInspect_BothFlags_Error verifies --user and --repo together is a usage error.
func TestInspect_BothFlags_Error(t *testing.T) {
	setupInspectTest(t)
	resetInspectFlags(t)
	inspectUser = "alice"
	inspectRepo = "api"

	err := runInspect(inspectCmd, []string{"inspect-col"})
	if err == nil {
		t.Fatal("expected error when --user and --repo are both set, got nil")
	}
	var ue *UsageError
	if !isUsageError(err, &ue) {
		t.Errorf("expected UsageError, got %T: %v", err, err)
	}
}

// TestInspect_JSON_Matrix verifies --json produces parseable JSON.
func TestInspect_JSON_Matrix(t *testing.T) {
	setupInspectTest(t)
	resetInspectFlags(t)
	inspectJSON = true

	out := captureStdout(func() {
		if err := runInspect(inspectCmd, []string{"inspect-col"}); err != nil {
			t.Fatalf("runInspect --json: %v", err)
		}
	})

	var result inspectMatrixOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	if result.Collection != "inspect-col" {
		t.Errorf("collection = %q, want %q", result.Collection, "inspect-col")
	}
}

// TestInspect_EmptyCollection verifies inspect handles no members/repos gracefully.
func TestInspect_EmptyCollection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetInspectFlags(t)

	col, err := collection.New("empty-inspect", "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPublic)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	if err := runInspect(inspectCmd, []string{"empty-inspect"}); err != nil {
		t.Fatalf("runInspect on empty collection: %v", err)
	}
}

// isUsageError is a helper to avoid importing errors in test files that
// already use errors via other means.
func isUsageError(err error, target **UsageError) bool {
	if err == nil {
		return false
	}
	if ue, ok := err.(*UsageError); ok {
		*target = ue
		return true
	}
	return false
}
