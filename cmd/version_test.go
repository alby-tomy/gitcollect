package cmd

import (
	"encoding/json"
	"runtime"
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

// --- version flags: -v and --version must match "gitcollect version" ---

func TestVersionLine_Format(t *testing.T) {
	got := versionLine("3.0.1")
	want := "gitcollect 3.0.1 " + runtime.GOOS + "/" + runtime.GOARCH + "\n"
	if got != want {
		t.Errorf("versionLine = %q, want %q", got, want)
	}
}

// SetVersion wires cobra's version flag. Without a non-empty Version on the
// root command, neither --version nor -v exists at all.
func TestSetVersion_RegistersFlagAndShorthand(t *testing.T) {
	prev := appVersion
	prevVersion := rootCmd.Version
	t.Cleanup(func() {
		appVersion = prev
		rootCmd.Version = prevVersion
	})

	SetVersion("3.0.1")

	if rootCmd.Version != "3.0.1" {
		t.Errorf("rootCmd.Version = %q, want 3.0.1", rootCmd.Version)
	}

	// cobra registers the flag lazily, at execution time.
	rootCmd.InitDefaultVersionFlag()

	f := rootCmd.Flags().Lookup("version")
	if f == nil {
		t.Fatal("--version flag was not registered")
	}
	if f.Shorthand != "v" {
		t.Errorf("expected -v shorthand for --version, got %q", f.Shorthand)
	}
}

// Both spellings must print the same line — asking the same question two
// ways should not produce two formats.
func TestVersionFlagTemplate_MatchesSubcommand(t *testing.T) {
	prev := appVersion
	prevVersion := rootCmd.Version
	t.Cleanup(func() {
		appVersion = prev
		rootCmd.Version = prevVersion
	})

	SetVersion("9.9.9")

	subcommandOutput := captureStdout(func() {
		if err := versionCmd.RunE(versionCmd, nil); err != nil {
			t.Fatalf("version subcommand: %v", err)
		}
	})
	if subcommandOutput != versionLine("9.9.9") {
		t.Errorf("subcommand printed %q, want %q", subcommandOutput, versionLine("9.9.9"))
	}
	if rootCmd.VersionTemplate() != versionLine("9.9.9") {
		t.Errorf("--version template %q does not match the subcommand output %q",
			rootCmd.VersionTemplate(), versionLine("9.9.9"))
	}
}
