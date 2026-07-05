package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func TestFormatSyncLine(t *testing.T) {
	cases := []struct {
		name   string
		r      syncResult
		dryRun bool
		want   []string // substrings that must all appear
	}{
		{
			name:   "cloned successfully",
			r:      syncResult{name: "repo1", kind: syncKindClone, duration: 1200 * time.Millisecond},
			want:   []string{"repo1", "not cloned", "cloning", "✓ done (1.2s)"},
		},
		{
			name:   "pulled, up to date",
			r:      syncResult{name: "repo2", kind: syncKindPull, newCommits: 0},
			want:   []string{"repo2", "already cloned", "pulling", "✓ up to date"},
		},
		{
			name:   "pulled, new commits",
			r:      syncResult{name: "repo3", kind: syncKindPull, newCommits: 3},
			want:   []string{"repo3", "✓ 3 new commit(s)"},
		},
		{
			name:   "failed",
			r:      syncResult{name: "repo4", kind: syncKindClone, err: errors.New("boom")},
			want:   []string{"repo4", "✗ failed: boom"},
		},
		{
			name:   "dry run clone",
			r:      syncResult{name: "repo5", kind: syncKindClone},
			dryRun: true,
			want:   []string{"repo5", "not cloned", "would sync"},
		},
		{
			name:   "dry run pull",
			r:      syncResult{name: "repo6", kind: syncKindPull},
			dryRun: true,
			want:   []string{"repo6", "already cloned", "would sync"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSyncLine(tc.r, tc.dryRun)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("formatSyncLine(%+v, dryRun=%v) = %q, expected it to contain %q", tc.r, tc.dryRun, got, want)
				}
			}
		})
	}
}

func TestSyncAll_RunsOnAllCollections(t *testing.T) {
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

	for _, name := range []string{"sync-all-a", "sync-all-b"} {
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

	// dryRun=true: no real git operations, no repo dirs needed
	stderr := captureStderr(func() {
		captureStdout(func() {
			if err := runSyncAll(dest, true, 1); err != nil {
				t.Fatalf("runSyncAll: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "sync-all-a") || !strings.Contains(stderr, "sync-all-b") {
		t.Errorf("expected both collection names in output, got: %q", stderr)
	}
}
