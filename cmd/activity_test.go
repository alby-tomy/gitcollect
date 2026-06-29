package cmd

import (
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/internal/activity"
)

func TestShortSHA(t *testing.T) {
	if got := shortSHA("abcdef1234567890"); got != "abcdef1" {
		t.Errorf("shortSHA(long) = %q, want %q", got, "abcdef1")
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("shortSHA(short) = %q, want unchanged %q", got, "abc")
	}
}

func TestMergeActivity_Deduplicates(t *testing.T) {
	now := time.Now().UTC()
	existing := []activity.Entry{
		{Repo: "widgets", CommitSHA: "aaa111", Message: "old fetch"},
	}
	fetched := []activity.Entry{
		{Repo: "widgets", CommitSHA: "aaa111", Message: "old fetch", CommittedAt: now}, // same commit, refetched
		{Repo: "widgets", CommitSHA: "bbb222", Message: "new commit", CommittedAt: now},
		{Repo: "gadgets", CommitSHA: "aaa111", Message: "different repo, same SHA text", CommittedAt: now},
	}

	got := mergeActivity(existing, fetched)
	if len(got) != 3 {
		t.Fatalf("expected 3 distinct (repo, sha) entries, got %d: %+v", len(got), got)
	}

	seen := map[string]bool{}
	for _, e := range got {
		key := e.Repo + "\x00" + e.CommitSHA
		if seen[key] {
			t.Fatalf("duplicate entry for %s in merged result: %+v", key, got)
		}
		seen[key] = true
	}
	if !seen["widgets\x00aaa111"] || !seen["widgets\x00bbb222"] || !seen["gadgets\x00aaa111"] {
		t.Fatalf("missing expected keys in merged result: %+v", got)
	}
}

func TestMergeActivity_EmptyInputs(t *testing.T) {
	if got := mergeActivity(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty merge of two nils, got %v", got)
	}
	existing := []activity.Entry{{Repo: "widgets", CommitSHA: "aaa111"}}
	if got := mergeActivity(existing, nil); len(got) != 1 {
		t.Fatalf("expected merge with nil fetched to just return existing, got %v", got)
	}
	if got := mergeActivity(nil, existing); len(got) != 1 {
		t.Fatalf("expected merge with nil existing to just return fetched, got %v", got)
	}
}
