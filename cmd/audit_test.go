package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// TestParseSince_ValidValues verifies that each documented --since value is
// accepted without error.
func TestParseSince_ValidValues(t *testing.T) {
	for _, v := range []string{"1h", "24h", "7d", "30d", "90d"} {
		if _, err := parseSince(v); err != nil {
			t.Errorf("parseSince(%q) = %v, want nil", v, err)
		}
	}
}

// TestParseSince_InvalidValue_ListsAllFive verifies that an unrecognised
// --since value produces an error message that names every valid option.
// Users who guess "2w" or "1d" should never have to read source code to
// discover what the tool actually accepts.
func TestParseSince_InvalidValue_ListsAllFive(t *testing.T) {
	_, err := parseSince("2w")
	if err == nil {
		t.Fatal("parseSince(2w) = nil, want an error")
	}
	msg := err.Error()
	for _, v := range []string{"1h", "24h", "7d", "30d", "90d"} {
		if !strings.Contains(msg, v) {
			t.Errorf("error message missing %q; got: %s", v, msg)
		}
	}
}

// ── B5: --from/--to/--action flag tests ──────────────────────────────────────

func setupAuditCmdTest(t *testing.T, collName string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	col, err := collection.New(collName, "github.com",
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPublic)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
}

func resetAuditFlags() func() {
	oldUser := auditUser
	oldSince := auditSince
	oldFrom := auditFrom
	oldTo := auditTo
	oldAction := auditAction
	oldJSON := auditJSON
	auditUser = ""
	auditSince = ""
	auditFrom = ""
	auditTo = ""
	auditAction = ""
	auditJSON = true
	return func() {
		auditUser = oldUser
		auditSince = oldSince
		auditFrom = oldFrom
		auditTo = oldTo
		auditAction = oldAction
		auditJSON = oldJSON
	}
}

func TestAudit_FromFlag(t *testing.T) {
	defer resetAuditFlags()()
	setupAuditCmdTest(t, "from-col")

	old := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	recent := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	for _, e := range []audit.AuditEntry{
		{Collection: "from-col", Actor: "alice", Action: "member.add", Target: "bob", Result: "ok", Timestamp: old},
		{Collection: "from-col", Actor: "alice", Action: "member.add", Target: "carol", Result: "ok", Timestamp: recent},
	} {
		if err := audit.Append(e); err != nil {
			t.Fatalf("audit.Append: %v", err)
		}
	}

	auditFrom = "2024-06-01"

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runAudit(nil, []string{"from-col"}); err != nil {
				t.Fatalf("runAudit: %v", err)
			}
		})
	})

	var entries []audit.AuditEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entries); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out)
	}
	if len(entries) != 1 || entries[0].Target != "carol" {
		t.Errorf("expected only carol (after from date), got %+v", entries)
	}
}

func TestAudit_ToFlag(t *testing.T) {
	defer resetAuditFlags()()
	setupAuditCmdTest(t, "to-col")

	old := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	recent := time.Date(2024, 8, 1, 12, 0, 0, 0, time.UTC)

	for _, e := range []audit.AuditEntry{
		{Collection: "to-col", Actor: "alice", Action: "member.add", Target: "bob", Result: "ok", Timestamp: old},
		{Collection: "to-col", Actor: "alice", Action: "member.add", Target: "carol", Result: "ok", Timestamp: recent},
	} {
		if err := audit.Append(e); err != nil {
			t.Fatalf("audit.Append: %v", err)
		}
	}

	auditTo = "2024-06-30"

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runAudit(nil, []string{"to-col"}); err != nil {
				t.Fatalf("runAudit: %v", err)
			}
		})
	})

	var entries []audit.AuditEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entries); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out)
	}
	if len(entries) != 1 || entries[0].Target != "bob" {
		t.Errorf("expected only bob (before to date), got %+v", entries)
	}
}

func TestAudit_ActionFlag(t *testing.T) {
	defer resetAuditFlags()()
	setupAuditCmdTest(t, "action-col")

	now := time.Now().UTC()
	for _, e := range []audit.AuditEntry{
		{Collection: "action-col", Actor: "alice", Action: "member.add", Target: "bob", Result: "ok", Timestamp: now},
		{Collection: "action-col", Actor: "alice", Action: "repo.create", Target: "my-repo", Result: "ok", Timestamp: now},
		{Collection: "action-col", Actor: "alice", Action: "MEMBER.ADD", Target: "carol", Result: "ok", Timestamp: now},
	} {
		if err := audit.Append(e); err != nil {
			t.Fatalf("audit.Append: %v", err)
		}
	}

	auditAction = "member.add"

	out := captureStdout(func() {
		captureStderr(func() {
			if err := runAudit(nil, []string{"action-col"}); err != nil {
				t.Fatalf("runAudit: %v", err)
			}
		})
	})

	var entries []audit.AuditEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entries); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 member.add entries (case-insensitive), got %d: %+v", len(entries), entries)
	}
}

func TestAudit_SinceAndFromMutuallyExclusive(t *testing.T) {
	defer resetAuditFlags()()
	setupAuditCmdTest(t, "mutex-col")

	auditSince = "7d"
	auditFrom = "2024-01-01"

	var runErr error
	captureStdout(func() {
		captureStderr(func() {
			runErr = runAudit(nil, []string{"mutex-col"})
		})
	})

	if runErr == nil {
		t.Error("expected error when --since and --from are both set, got nil")
	}
	if !strings.Contains(runErr.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %v", runErr)
	}
}
