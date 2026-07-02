package cmd

import (
	"strings"
	"testing"
)

// TestParseSince_ValidValues verifies that each documented --since value is
// accepted without error.
func TestParseSince_ValidValues(t *testing.T) {
	for _, v := range []string{"1h", "24h", "7d", "30d", "90d"} {
		if _, err := parseSince(v); err != nil {
			t.Errorf("parseSince(%q) = %v, want nil", v, err)
		}
	}
}

// TestParseSince_InvalidValue_ListsAllFive verifies that an unrecognised
// --since value produces an error message that names every valid option.
// Users who guess "2w" or "1d" should never have to read source code to
// discover what the tool actually accepts.
func TestParseSince_InvalidValue_ListsAllFive(t *testing.T) {
	_, err := parseSince("2w")
	if err == nil {
		t.Fatal("parseSince(2w) = nil, want an error")
	}
	msg := err.Error()
	for _, v := range []string{"1h", "24h", "7d", "30d", "90d"} {
		if !strings.Contains(msg, v) {
			t.Errorf("error message missing %q; got: %s", v, msg)
		}
	}
}
