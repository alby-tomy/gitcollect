package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func setupDescribeTest(t *testing.T, collName string) {
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
		api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}
}

func TestDescribe_SetsDescription(t *testing.T) {
	setupDescribeTest(t, "desc-col")

	captureStdout(func() {
		captureStderr(func() {
			if err := runDescribe(nil, []string{"desc-col", "My project collection"}); err != nil {
				t.Fatalf("runDescribe: %v", err)
			}
		})
	})

	col, err := collection.Load("desc-col")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if col.Description != "My project collection" {
		t.Errorf("expected Description='My project collection', got %q", col.Description)
	}
}

func TestDescribe_ClearsDescription(t *testing.T) {
	setupDescribeTest(t, "clr-col")

	// First set a description
	captureStdout(func() {
		captureStderr(func() {
			_ = runDescribe(nil, []string{"clr-col", "Initial description"})
		})
	})

	// Then clear it
	captureStdout(func() {
		captureStderr(func() {
			if err := runDescribe(nil, []string{"clr-col", ""}); err != nil {
				t.Fatalf("runDescribe clear: %v", err)
			}
		})
	})

	col, err := collection.Load("clr-col")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if col.Description != "" {
		t.Errorf("expected empty Description after clear, got %q", col.Description)
	}
}

func TestDescribe_UpdatesTimestamp(t *testing.T) {
	setupDescribeTest(t, "ts-col")

	before := time.Now().Add(-time.Second)

	captureStdout(func() {
		captureStderr(func() {
			if err := runDescribe(nil, []string{"ts-col", "Timestamped"}); err != nil {
				t.Fatalf("runDescribe: %v", err)
			}
		})
	})

	col, err := collection.Load("ts-col")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if !col.UpdatedAt.After(before) {
		t.Errorf("expected UpdatedAt to be updated, got %v (before: %v)", col.UpdatedAt, before)
	}
}

func TestDescribe_RequiresOwner(t *testing.T) {
	setupDescribeTest(t, "owner-col")

	// Switch to a non-owner identity
	cachedUserID = "non-owner-id"
	t.Cleanup(func() { cachedUserID = "owner-id" })

	var runErr error
	captureStdout(func() {
		captureStderr(func() {
			runErr = runDescribe(nil, []string{"owner-col", "Should fail"})
		})
	})

	if runErr == nil {
		t.Error("expected error for non-owner caller, got nil")
	}
}

func TestDescribe_AuditEntry(t *testing.T) {
	setupDescribeTest(t, "audit-col")

	captureStdout(func() {
		captureStderr(func() {
			if err := runDescribe(nil, []string{"audit-col", "Audit test"}); err != nil {
				t.Fatalf("runDescribe: %v", err)
			}
		})
	})

	entries, err := audit.Read("audit-col")
	if err != nil {
		t.Fatalf("audit.Read: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry, got none")
	}
	last := entries[0]
	if last.Action != "collection.describe" {
		t.Errorf("expected action='collection.describe', got %q", last.Action)
	}
	if !strings.Contains(last.Detail, "updated") && !strings.Contains(last.Detail, "Cleared") {
		t.Errorf("expected audit Detail to mention 'updated' or 'Cleared', got %q", last.Detail)
	}
}
