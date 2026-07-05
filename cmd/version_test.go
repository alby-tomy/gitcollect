package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func resetVersionFlags() func() {
	old := versionJSON
	versionJSON = false
	return func() { versionJSON = old }
}

func TestVersion_JSON_ValidOutput(t *testing.T) {
	defer resetVersionFlags()()
	versionJSON = true

	out := captureStdout(func() {
		if err := versionCmd.RunE(versionCmd, nil); err != nil {
			t.Fatalf("versionCmd.RunE: %v", err)
		}
	})

	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
}

func TestVersion_JSON_ContainsVersion(t *testing.T) {
	defer resetVersionFlags()()
	versionJSON = true

	out := captureStdout(func() {
		if err := versionCmd.RunE(versionCmd, nil); err != nil {
			t.Fatalf("versionCmd.RunE: %v", err)
		}
	})

	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := m["version"]; !ok {
		t.Errorf("expected JSON to contain 'version' key, got: %v", m)
	}
	if _, ok := m["go_version"]; !ok {
		t.Errorf("expected JSON to contain 'go_version' key, got: %v", m)
	}
	if _, ok := m["platform"]; !ok {
		t.Errorf("expected JSON to contain 'platform' key, got: %v", m)
	}
}

func TestVersion_NoJSON_PlainText(t *testing.T) {
	defer resetVersionFlags()()
	versionJSON = false

	out := captureStdout(func() {
		if err := versionCmd.RunE(versionCmd, nil); err != nil {
			t.Fatalf("versionCmd.RunE: %v", err)
		}
	})

	if !strings.HasPrefix(out, "gitcollect ") {
		t.Errorf("expected plain-text output starting with 'gitcollect ', got: %q", out)
	}
	if strings.Contains(out, "{") {
		t.Errorf("expected no JSON in plain-text output, got: %q", out)
	}
}
