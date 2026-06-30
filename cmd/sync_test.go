package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"
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
