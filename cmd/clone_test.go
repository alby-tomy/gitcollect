package cmd

import (
	"reflect"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

func TestSplitPick(t *testing.T) {
	cases := []struct {
		name string
		raw  []string
		want []string
	}{
		{"nil", nil, nil},
		{"single value", []string{"repo1"}, []string{"repo1"}},
		{"space-separated within one value", []string{"repo1 repo2"}, []string{"repo1", "repo2"}},
		{"repeated flag", []string{"repo1", "repo2"}, []string{"repo1", "repo2"}},
		{"mixed", []string{"repo1 repo2", "repo3"}, []string{"repo1", "repo2", "repo3"}},
		{"extra whitespace collapses", []string{"  repo1   repo2  "}, []string{"repo1", "repo2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitPick(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitPick(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFirstPendingInvite(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}

	client := &pendingInviteMock{pending: map[string]bool{"repo2/alice": true}}

	if got := firstPendingInvite(col, "alice", []string{"repo1"}, client); got != "" {
		t.Errorf("expected no pending invite among repo1 alone, got %q", got)
	}
	if got := firstPendingInvite(col, "alice", []string{"repo1", "repo2"}, client); got != "repo2" {
		t.Errorf("expected repo2 to be reported as pending, got %q", got)
	}
	if got := firstPendingInvite(col, "alice", nil, client); got != "" {
		t.Errorf("expected no pending invite for an empty skipped list, got %q", got)
	}
}
