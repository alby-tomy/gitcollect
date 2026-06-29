package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/internal/config"
)

func useTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestAppendAndRead(t *testing.T) {
	useTempHome(t)

	entries := []AuditEntry{
		{Timestamp: time.Now().UTC(), Collection: "acme", Actor: "alice", Action: "member.add", Target: "bob", Detail: "Added bob", Result: "ok"},
		{Timestamp: time.Now().UTC(), Collection: "acme", Actor: "alice", Action: "visibility.change", Target: "private→public", Detail: "Changed", Result: "ok"},
	}
	for _, e := range entries {
		if err := Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := Read("acme")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	// Read returns newest first.
	if got[0].Action != "visibility.change" || got[1].Action != "member.add" {
		t.Fatalf("expected newest-first order, got %+v", got)
	}
}

func TestAppend_OpenFailure(t *testing.T) {
	useTempHome(t)

	// Pre-create a directory where the log file should go, so OpenFile fails.
	dir, err := config.AuditDir()
	if err != nil {
		t.Fatalf("AuditDir: %v", err)
	}
	if err := config.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "acme.log"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := Append(AuditEntry{Collection: "acme", Actor: "alice"}); err == nil {
		t.Fatal("expected Append to fail when the log path is a directory")
	}
}

func TestRead_MalformedLine(t *testing.T) {
	useTempHome(t)

	dir, err := config.AuditDir()
	if err != nil {
		t.Fatalf("AuditDir: %v", err)
	}
	if err := config.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acme.log"), []byte("not json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Read("acme"); err == nil {
		t.Fatal("expected Read to fail on a malformed log line")
	}
}

func TestReadMissingLogReturnsEmpty(t *testing.T) {
	useTempHome(t)

	got, err := Read("never-existed")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice for missing log, got %v", got)
	}
}

func TestFilterByUser(t *testing.T) {
	now := time.Now().UTC()
	entries := []AuditEntry{
		{Actor: "alice", Target: "bob", Timestamp: now},
		{Actor: "alice", Target: "charlie", Timestamp: now},
		{Actor: "diana", Target: "alice", Timestamp: now},
		{Actor: "diana", Target: "eve", Timestamp: now},
	}

	got := Filter(entries, "alice", 0)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries involving alice (as actor or target), got %d: %+v", len(got), got)
	}

	got = Filter(entries, "", 0)
	if len(got) != 4 {
		t.Fatalf("expected no filtering with empty user, got %d", len(got))
	}
}

func TestFilterBySince(t *testing.T) {
	now := time.Now().UTC()
	entries := []AuditEntry{
		{Actor: "alice", Timestamp: now},
		{Actor: "alice", Timestamp: now.Add(-48 * time.Hour)},
	}

	got := Filter(entries, "", 24*time.Hour)
	if len(got) != 1 {
		t.Fatalf("expected only the recent entry within 24h, got %d: %+v", len(got), got)
	}

	got = Filter(entries, "", 0)
	if len(got) != 2 {
		t.Fatalf("expected since=0 to mean no time filter, got %d", len(got))
	}
}
