package cmd

import (
	"strings"
	"testing"
)

// TestHelpConcepts_Runs verifies the concepts command executes without error.
func TestHelpConcepts_Runs(t *testing.T) {
	captureStdout(func() {
		if err := runHelpConcepts(nil, nil); err != nil {
			t.Fatalf("runHelpConcepts returned error: %v", err)
		}
	})
}

// TestHelpConcepts_ContainsKeywords verifies the output contains the required
// concept headings and config paths.
func TestHelpConcepts_ContainsKeywords(t *testing.T) {
	out := captureStdout(func() {
		_ = runHelpConcepts(nil, nil)
	})
	keywords := []string{
		"Collection",
		"Identity",
		"Platform enforcement",
		"~/.gitcollect/collections/",
		"~/.gitcollect/audit/",
	}
	for _, kw := range keywords {
		if !strings.Contains(out, kw) {
			t.Errorf("output missing keyword %q", kw)
		}
	}
}

// TestRootCmd_LongContainsQuickStart verifies the root command Long field
// includes the QUICK START section.
func TestRootCmd_LongContainsQuickStart(t *testing.T) {
	if !strings.Contains(rootCmd.Long, "QUICK START") {
		t.Error("rootCmd.Long does not contain QUICK START section")
	}
}
