package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"gopkg.in/yaml.v3"
)

func setupExportTest(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	for _, name := range names {
		col, err := collection.New(name, "github.com",
			api.UserInfo{ID: "owner-id", Login: "owner"}, collection.VisibilityPrivate)
		if err != nil {
			t.Fatalf("collection.New(%s): %v", name, err)
		}
		col.Repos = []collection.RepoAccess{{Name: "repo1"}}
		if err := col.Save(); err != nil {
			t.Fatalf("col.Save(%s): %v", name, err)
		}
	}
}

func resetExportFlags(t *testing.T) {
	t.Helper()
	oldAll, oldJSON := exportAll, exportJSON
	t.Cleanup(func() { exportAll = oldAll; exportJSON = oldJSON })
	exportAll = false
	exportJSON = false
}

// TestExport_SingleCollection_ValidYAML verifies single-collection YAML is parseable.
func TestExport_SingleCollection_ValidYAML(t *testing.T) {
	setupExportTest(t, "export-yaml")
	resetExportFlags(t)

	out := captureStdout(func() {
		if err := runExport(exportCmd, []string{"export-yaml"}); err != nil {
			t.Fatalf("runExport = %v", err)
		}
	})

	var col collection.Collection
	if err := yaml.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("output is not valid YAML: %v\noutput: %q", err, out)
	}
	if col.Name != "export-yaml" {
		t.Errorf("col.Name = %q, want %q", col.Name, "export-yaml")
	}
}

// TestExport_SingleCollection_JSON verifies single-collection JSON is parseable.
func TestExport_SingleCollection_JSON(t *testing.T) {
	setupExportTest(t, "export-json-col")
	resetExportFlags(t)
	exportJSON = true

	out := captureStdout(func() {
		if err := runExport(exportCmd, []string{"export-json-col"}); err != nil {
			t.Fatalf("runExport --json = %v", err)
		}
	})

	var col collection.Collection
	if err := json.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	if col.Name != "export-json-col" {
		t.Errorf("col.Name = %q, want %q", col.Name, "export-json-col")
	}
}

// TestExport_All_MultiDocYAML verifies --all produces --- separator between docs.
func TestExport_All_MultiDocYAML(t *testing.T) {
	setupExportTest(t, "export-all-a", "export-all-b", "export-all-c")
	resetExportFlags(t)
	exportAll = true

	out := captureStdout(func() {
		if err := runExport(exportCmd, nil); err != nil {
			t.Fatalf("runExport --all = %v", err)
		}
	})

	if !strings.Contains(out, "---") {
		t.Errorf("expected --- separator between YAML docs, output: %q", out)
	}
}

// TestExport_All_JSONArray verifies --all --json produces a JSON array.
func TestExport_All_JSONArray(t *testing.T) {
	setupExportTest(t, "export-arr-a", "export-arr-b")
	resetExportFlags(t)
	exportAll = true
	exportJSON = true

	out := captureStdout(func() {
		if err := runExport(exportCmd, nil); err != nil {
			t.Fatalf("runExport --all --json = %v", err)
		}
	})

	var cols []collection.Collection
	if err := json.Unmarshal([]byte(out), &cols); err != nil {
		t.Fatalf("output is not valid JSON array: %v\noutput: %q", err, out)
	}
	if len(cols) < 2 {
		t.Errorf("expected at least 2 collections in JSON array, got %d", len(cols))
	}
}

// TestExport_NoColor_NoDecoration verifies stdout has no ANSI escape codes.
func TestExport_NoColor_NoDecoration(t *testing.T) {
	setupExportTest(t, "export-nocolor")
	resetExportFlags(t)

	out := captureStdout(func() {
		_ = runExport(exportCmd, []string{"export-nocolor"})
	})

	if strings.Contains(out, "\x1b[") {
		t.Errorf("output contains ANSI escape codes: %q", out)
	}
}

// TestExport_NonExistent_Error verifies a missing collection returns an error.
func TestExport_NonExistent_Error(t *testing.T) {
	setupExportTest(t) // no collections created
	resetExportFlags(t)

	err := runExport(exportCmd, []string{"does-not-exist"})
	if err == nil {
		t.Error("expected error for non-existent collection, got nil")
	}
}
