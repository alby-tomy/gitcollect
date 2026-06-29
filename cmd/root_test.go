package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/internal/config"
)

func TestStaleDays(t *testing.T) {
	if got := staleDays(time.Now()); got != 0 {
		t.Errorf("staleDays(now) = %d, want 0", got)
	}
	if got := staleDays(time.Now().Add(-29 * 24 * time.Hour)); got != 0 {
		t.Errorf("staleDays(29 days ago) = %d, want 0 (under the 30-day threshold)", got)
	}
	if got := staleDays(time.Now().Add(-45 * 24 * time.Hour)); got != 45 {
		t.Errorf("staleDays(45 days ago) = %d, want 45", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"cybersecurity", "cybersecurity", 0},
		{"cybersecurty", "cybersecurity", 1},   // one deletion
		{"cybersecurityy", "cybersecurity", 1}, // one insertion
		{"cybersecurety", "cybersecurity", 1},  // one substitution
		{"kitten", "sitting", 3},
	}
	for _, tc := range cases {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSuggestCollectionName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	collDir, err := config.CollectionsDir()
	if err != nil {
		t.Fatalf("CollectionsDir: %v", err)
	}
	if err := config.EnsureDir(collDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	for _, name := range []string{"cybersecurity", "machine-learning"} {
		if err := os.WriteFile(filepath.Join(collDir, name+".yaml"), []byte("name: "+name+"\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	if got := suggestCollectionName("cybersecurty"); got != "cybersecurity" {
		t.Errorf("suggestCollectionName(typo) = %q, want %q", got, "cybersecurity")
	}
	if got := suggestCollectionName("totally-unrelated-name"); got != "" {
		t.Errorf("suggestCollectionName(unrelated) = %q, want empty", got)
	}
	if got := suggestCollectionName("cybersecurity"); got != "" {
		t.Errorf("suggestCollectionName(exact match) = %q, want empty (nothing to suggest)", got)
	}
}
