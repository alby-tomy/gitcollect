package audit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/v3/internal/config"
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

func TestFilterByDate_From(t *testing.T) {
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	entries := []AuditEntry{
		{Actor: "alice", Timestamp: base.AddDate(0, 0, -1)}, // before
		{Actor: "alice", Timestamp: base},                   // exact from
		{Actor: "alice", Timestamp: base.AddDate(0, 0, 1)},  // after
	}
	got := FilterByDate(entries, base, time.Time{})
	if len(got) != 2 {
		t.Fatalf("expected 2 entries on or after from, got %d: %+v", len(got), got)
	}
}

func TestFilterByDate_To(t *testing.T) {
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	entries := []AuditEntry{
		{Actor: "alice", Timestamp: base.AddDate(0, 0, -1)}, // before
		{Actor: "alice", Timestamp: base},                   // exact to
		{Actor: "alice", Timestamp: base.AddDate(0, 0, 1)},  // after
	}
	got := FilterByDate(entries, time.Time{}, base)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries on or before to, got %d: %+v", len(got), got)
	}
}

func TestFilterByDate_Range(t *testing.T) {
	from := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
	entries := []AuditEntry{
		{Actor: "alice", Timestamp: from.AddDate(0, 0, -1)}, // before range
		{Actor: "alice", Timestamp: from},                   // start
		{Actor: "alice", Timestamp: from.AddDate(0, 0, 15)}, // middle
		{Actor: "alice", Timestamp: to},                     // end
		{Actor: "alice", Timestamp: to.AddDate(0, 0, 1)},    // after range
	}
	got := FilterByDate(entries, from, to)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries in range, got %d: %+v", len(got), got)
	}
}

func TestFilterByDate_ZeroFrom_NoLower(t *testing.T) {
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []AuditEntry{
		{Actor: "alice", Timestamp: old},
		{Actor: "alice", Timestamp: to.AddDate(0, 0, 1)}, // after to
	}
	got := FilterByDate(entries, time.Time{}, to)
	if len(got) != 1 || got[0].Timestamp != old {
		t.Fatalf("expected only old entry (zero from = no lower bound), got %+v", got)
	}
}

func TestFilterByDate_ZeroBoth_NoFilter(t *testing.T) {
	entries := []AuditEntry{
		{Actor: "alice", Timestamp: time.Now()},
		{Actor: "bob", Timestamp: time.Now()},
	}
	got := FilterByDate(entries, time.Time{}, time.Time{})
	if len(got) != 2 {
		t.Fatalf("expected no filter when both zero, got %d", len(got))
	}
}

func TestFilterByAction_CaseInsensitive(t *testing.T) {
	entries := []AuditEntry{
		{Action: "member.add"},
		{Action: "MEMBER.ADD"},
		{Action: "Member.Add"},
		{Action: "repo.create"},
	}
	got := FilterByAction(entries, "member.add")
	if len(got) != 3 {
		t.Fatalf("expected 3 case-insensitive matches for member.add, got %d: %+v", len(got), got)
	}
}

func TestFilterByAction_Empty_NoFilter(t *testing.T) {
	entries := []AuditEntry{
		{Action: "member.add"},
		{Action: "repo.create"},
	}
	got := FilterByAction(entries, "")
	if len(got) != 2 {
		t.Fatalf("expected no filter for empty action, got %d", len(got))
	}
}

// --- regression: a Collection value that is not a bare name must not
// escape the audit directory or silently fail to write ---

func TestSafeLogName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"cybersecurity", "cybersecurity"},
		{"acme/gitcollect-config", "acme-gitcollect-config"}, // publish's repo spec
		{"../../etc/passwd", "etc-passwd"},
		{"a b c", "a-b-c"},
		{".hidden", "hidden"},
		{"under_score.log", "under_score.log"},
	}
	for _, tc := range cases {
		got, err := safeLogName(tc.in)
		if err != nil {
			t.Errorf("safeLogName(%q) returned error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("safeLogName(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.ContainsAny(got, `/\`) || got == "." || got == ".." {
			t.Errorf("safeLogName(%q) produced an unsafe segment: %q", tc.in, got)
		}
	}
}

func TestSafeLogName_RejectsEmptyResult(t *testing.T) {
	for _, in := range []string{"", "///", "..."} {
		if _, err := safeLogName(in); !errors.Is(err, ErrInvalidLogName) {
			t.Errorf("safeLogName(%q) should report ErrInvalidLogName, got %v", in, err)
		}
	}
}

// publish records its repository spec, which contains a slash. The write
// must land in the audit directory rather than a nonexistent subdirectory.
func TestAppend_RepoSpecWritesInsideAuditDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := Append(AuditEntry{Collection: "acme/gitcollect-config", Action: "publish", Result: "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err := Read("acme/gitcollect-config")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "publish" {
		t.Fatalf("expected the entry to round-trip, got %+v", entries)
	}
}
