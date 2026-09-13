package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// TestPrintVisibilityImpact verifies that printVisibilityImpact includes the
// collection name and member/repo counts in the impact preview.
func TestPrintVisibilityImpact(t *testing.T) {
	col, err := collection.New("acme", "github.com", api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"alice-id", "bob-id"}
	col.Repos = []collection.RepoAccess{{Name: "repo1"}, {Name: "repo2"}, {Name: "repo3"}}

	var buf bytes.Buffer
	printVisibilityImpact(&buf, col)

	out := buf.String()
	if !strings.Contains(out, "acme") {
		t.Errorf("expected collection name in impact output, got: %q", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("expected member count (2) in impact output, got: %q", out)
	}
	if !strings.Contains(out, "3") {
		t.Errorf("expected repo count (3) in impact output, got: %q", out)
	}
}
