package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func setupDoctorTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// TestDoctor_NoAuth_ReportsError verifies that a missing token produces an error check.
func TestDoctor_NoAuth_ReportsError(t *testing.T) {
	setupDoctorTest(t)

	old := doctorCheckFn
	t.Cleanup(func() { doctorCheckFn = old })
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUTH/github.com", Status: "error", Message: "github.com — no token stored", Fix: "gitcollect auth"},
		}
	}

	captureStderr(func() {
		captureStdout(func() {
			err := runDoctor(doctorCmd, nil)
			if err == nil {
				t.Error("expected error when auth check fails, got nil")
			}
		})
	})
}

// TestDoctor_ValidToken_ReportsOk verifies that a valid token produces an ok check.
func TestDoctor_ValidToken_ReportsOk(t *testing.T) {
	setupDoctorTest(t)

	old := doctorCheckFn
	t.Cleanup(func() { doctorCheckFn = old })
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUTH/github.com", Status: "ok", Message: "github.com — authenticated as alice"},
		}
	}

	captureStderr(func() {
		captureStdout(func() {
			if err := runDoctor(doctorCmd, nil); err != nil {
				t.Errorf("expected nil when all checks ok, got %v", err)
			}
		})
	})
}

// TestDoctor_StaleCollection_ReportsWarn verifies that a stale collection produces a warn check.
func TestDoctor_StaleCollection_ReportsWarn(t *testing.T) {
	setupDoctorTest(t)

	old := doctorCheckFn
	t.Cleanup(func() { doctorCheckFn = old })
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "COLLECTIONS/my-col", Status: "warn", Message: "my-col — last updated 45 days ago"},
		}
	}

	stderr := captureStderr(func() {
		captureStdout(func() {
			_ = runDoctor(doctorCmd, nil)
		})
	})

	if !strings.Contains(stderr, "warning") && !strings.Contains(stderr, "warn") && !strings.Contains(stderr, "⚠") {
		t.Errorf("expected warning indicator in stderr, got: %q", stderr)
	}
}

// TestDoctor_JSON_ValidOutput verifies --json produces parseable JSON.
func TestDoctor_JSON_ValidOutput(t *testing.T) {
	setupDoctorTest(t)

	old := doctorCheckFn
	oldJSON := doctorJSON
	t.Cleanup(func() { doctorCheckFn = old; doctorJSON = oldJSON })
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUTH/github.com", Status: "ok", Message: "github.com — authenticated as alice"},
		}
	}
	doctorJSON = true

	out := captureStdout(func() {
		_ = runDoctor(doctorCmd, nil)
	})

	var checks []doctorCheck
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\noutput: %q", err, out)
	}
	if len(checks) == 0 {
		t.Error("expected at least one check in JSON output")
	}
}

// TestDoctor_ExitCode_ZeroOnlyWarns verifies exit 0 when only warnings.
func TestDoctor_ExitCode_ZeroOnlyWarns(t *testing.T) {
	setupDoctorTest(t)

	old := doctorCheckFn
	t.Cleanup(func() { doctorCheckFn = old })
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUDIT", Status: "warn", Message: "Audit logs stored locally only"},
		}
	}

	captureStderr(func() {
		captureStdout(func() {
			if err := runDoctor(doctorCmd, nil); err != nil {
				t.Errorf("expected nil (exit 0) with only warnings, got %v", err)
			}
		})
	})
}

// TestDoctor_ExitCode_OneOnErrors verifies exit 1 (non-nil error) when any check errors.
func TestDoctor_ExitCode_OneOnErrors(t *testing.T) {
	setupDoctorTest(t)

	old := doctorCheckFn
	t.Cleanup(func() { doctorCheckFn = old })
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUTH/github.com", Status: "error", Message: "github.com — no token stored"},
		}
	}

	captureStderr(func() {
		captureStdout(func() {
			err := runDoctor(doctorCmd, nil)
			if err == nil {
				t.Error("expected non-nil error (exit 1) when error checks present")
			}
		})
	})
}

// --- regression: doctor must be able to report success ---

// An unconditional "warn" reminder meant every run ended with at least one
// warning, so the "all checks passed" branch was unreachable and the
// warning count carried no signal.
func TestDoctor_AllOkChecksReportSuccess(t *testing.T) {
	prev := doctorCheckFn
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUTH/github.com", Status: "ok", Message: "authenticated"},
			{Label: "AUDIT", Status: "info", Message: "stored locally only"},
		}
	}
	t.Cleanup(func() { doctorCheckFn = prev })

	// The summary line goes to stdout (output.Success); the per-check
	// listing and any warnings go to stderr. Capture both.
	var errOut string
	stdOut := captureStdout(func() {
		errOut = captureStderr(func() {
			if err := runDoctor(nil, nil); err != nil {
				t.Fatalf("runDoctor: %v", err)
			}
		})
	})

	if !strings.Contains(stdOut, "all checks passed") {
		t.Errorf("expected a clean run to report success, got stdout:\n%s", stdOut)
	}
	if strings.Contains(stdOut+errOut, "warning(s)") {
		t.Errorf("an info note must not be counted as a warning:\nstdout:\n%s\nstderr:\n%s", stdOut, errOut)
	}
}

func TestDoctor_InfoDoesNotAffectExitCode(t *testing.T) {
	prev := doctorCheckFn
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{{Label: "AUDIT", Status: "info", Message: "note"}}
	}
	t.Cleanup(func() { doctorCheckFn = prev })

	if err := runDoctor(nil, nil); err != nil {
		t.Errorf("an info-only run must exit 0, got: %v", err)
	}
}

func TestDoctor_ErrorStillFails(t *testing.T) {
	prev := doctorCheckFn
	doctorCheckFn = func() []doctorCheck {
		return []doctorCheck{
			{Label: "AUTH/github.com", Status: "error", Message: "no token stored"},
			{Label: "AUDIT", Status: "info", Message: "note"},
		}
	}
	t.Cleanup(func() { doctorCheckFn = prev })

	if err := runDoctor(nil, nil); err == nil {
		t.Error("expected a failing check to produce an error")
	}
}

// The real check list must not reintroduce a permanent warning.
func TestDoctorChecks_AuditNoteIsInformational(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, c := range runDoctorChecks() {
		if c.Label == "AUDIT" && c.Status != "info" {
			t.Errorf("AUDIT note should be informational, got status %q", c.Status)
		}
	}
}
