package activity

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

	entries := []Entry{
		{Collection: "acme", Repo: "widgets", Branch: "main", CommitSHA: "aaa111", Author: "alice", Message: "First", CommittedAt: time.Now().UTC()},
		{Collection: "acme", Repo: "widgets", Branch: "main", CommitSHA: "bbb222", Author: "bob", Message: "Second", CommittedAt: time.Now().UTC()},
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
	// Read returns newest-appended first.
	if got[0].CommitSHA != "bbb222" || got[1].CommitSHA != "aaa111" {
		t.Fatalf("expected newest-first order, got %+v", got)
	}
}

func TestAppend_OpenFailure(t *testing.T) {
	useTempHome(t)

	dir, err := config.ActivityDir()
	if err != nil {
		t.Fatalf("ActivityDir: %v", err)
	}
	if err := config.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "acme.log"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := Append(Entry{Collection: "acme", Repo: "widgets"}); err == nil {
		t.Fatal("expected Append to fail when the log path is a directory")
	}
}

func TestRead_MalformedLine(t *testing.T) {
	useTempHome(t)

	dir, err := config.ActivityDir()
	if err != nil {
		t.Fatalf("ActivityDir: %v", err)
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

func TestFilterByRepoAndUser(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{Repo: "widgets", Author: "alice", CommittedAt: now},
		{Repo: "widgets", Author: "bob", CommittedAt: now},
		{Repo: "gadgets", Author: "alice", CommittedAt: now},
	}

	got := Filter(entries, "widgets", "", 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for repo=widgets, got %d: %+v", len(got), got)
	}

	got = Filter(entries, "", "alice", 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for author=alice, got %d: %+v", len(got), got)
	}

	got = Filter(entries, "widgets", "alice", 0)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry for repo=widgets AND author=alice, got %d: %+v", len(got), got)
	}

	got = Filter(entries, "", "", 0)
	if len(got) != 3 {
		t.Fatalf("expected no filtering with empty repo and user, got %d", len(got))
	}
}

func TestFilterBySince(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{Repo: "widgets", CommittedAt: now},
		{Repo: "widgets", CommittedAt: now.Add(-48 * time.Hour)},
	}

	got := Filter(entries, "", "", 24*time.Hour)
	if len(got) != 1 {
		t.Fatalf("expected only the recent commit within 24h, got %d: %+v", len(got), got)
	}

	got = Filter(entries, "", "", 0)
	if len(got) != 2 {
		t.Fatalf("expected since=0 to mean no time filter, got %d", len(got))
	}
}

func TestKnownSHAs(t *testing.T) {
	entries := []Entry{
		{Repo: "widgets", CommitSHA: "aaa111"},
		{Repo: "gadgets", CommitSHA: "aaa111"}, // same SHA, different repo: distinct key
	}
	known := KnownSHAs(entries)

	if !known["widgets\x00aaa111"] {
		t.Error("expected widgets/aaa111 to be known")
	}
	if !known["gadgets\x00aaa111"] {
		t.Error("expected gadgets/aaa111 to be known as a distinct entry from widgets/aaa111")
	}
	if known["widgets\x00bbb222"] {
		t.Error("expected an unseen SHA to not be known")
	}
}
