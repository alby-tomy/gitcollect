package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/v3/internal/selfupdate"
)

// setUpdateEnv fixes the reported version and install method for one test,
// and stubs everything that would reach the network or touch a binary.
// Returns a pointer that records whether an install was attempted.
func setUpdateEnv(t *testing.T, version string, fromLDFlags bool, latest selfupdate.Release, latestErr error) *bool {
	t.Helper()

	prevVersion, prevLD := appVersion, ldflagsInjected
	prevLatest, prevDownload := latestReleaseFn, downloadReleaseFn
	prevReplace, prevExec, prevConfirm := replaceBinaryFn, executablePathFn, confirmFn
	prevGoInstall := goInstallFn
	prevCheck, prevYes := getUpdateCheck, getUpdateYes

	installed := false

	appVersion = version
	ldflagsInjected = fromLDFlags
	latestReleaseFn = func() (selfupdate.Release, error) { return latest, latestErr }
	downloadReleaseFn = func(selfupdate.Release) ([]byte, error) { return []byte("new binary"), nil }
	replaceBinaryFn = func(string, []byte) error { installed = true; return nil }
	executablePathFn = func() (string, error) { return "/usr/local/bin/gitcollect", nil }
	confirmFn = func(string) bool { return true }
	// Record a go-install upgrade as an install too, and never run the real
	// toolchain from a unit test.
	goInstallFn = func() error { installed = true; return nil }
	getUpdateCheck, getUpdateYes = false, true

	t.Cleanup(func() {
		appVersion, ldflagsInjected = prevVersion, prevLD
		latestReleaseFn, downloadReleaseFn = prevLatest, prevDownload
		replaceBinaryFn, executablePathFn, confirmFn = prevReplace, prevExec, prevConfirm
		goInstallFn = prevGoInstall
		getUpdateCheck, getUpdateYes = prevCheck, prevYes
	})
	return &installed
}

func release(version string) selfupdate.Release {
	return selfupdate.Release{
		Tag:     "v" + version,
		Version: version,
		Assets:  map[string]string{},
	}
}

// A release binary reports "v3.1.0" while the release is named "3.1.0", so
// the up-to-date case has to survive the leading v.
func TestGetUpdate_AlreadyLatestInstallsNothing(t *testing.T) {
	for _, version := range []string{"3.1.0", "v3.1.0"} {
		t.Run(version, func(t *testing.T) {
			installed := setUpdateEnv(t, version, true, release("3.1.0"), nil)

			out := captureStdout(func() {
				if err := runGetUpdate(nil, nil); err != nil {
					t.Fatalf("runGetUpdate: %v", err)
				}
			})

			if *installed {
				t.Error("nothing should be installed when already on the latest release")
			}
			if !strings.Contains(out, "is the latest release") {
				t.Errorf("expected an up-to-date message, got:\n%s", out)
			}
		})
	}
}

func TestGetUpdate_InstallsWhenNewer(t *testing.T) {
	installed := setUpdateEnv(t, "3.0.0", true, release("3.1.0"), nil)

	captureStdout(func() {
		if err := runGetUpdate(nil, nil); err != nil {
			t.Fatalf("runGetUpdate: %v", err)
		}
	})

	if !*installed {
		t.Error("expected the newer release to be installed")
	}
}

// --check must never modify anything, however far behind the binary is.
func TestGetUpdate_CheckReportsButDoesNotInstall(t *testing.T) {
	installed := setUpdateEnv(t, "3.0.0", true, release("3.1.0"), nil)
	getUpdateCheck = true

	out := captureStdout(func() {
		if err := runGetUpdate(nil, nil); err != nil {
			t.Fatalf("runGetUpdate: %v", err)
		}
	})

	if *installed {
		t.Error("--check must not install anything")
	}
	if !strings.Contains(out, "3.0.0") || !strings.Contains(out, "3.1.0") {
		t.Errorf("expected both versions in the report, got:\n%s", out)
	}
}

// Declining the prompt is a decision, not an error.
func TestGetUpdate_DeclinedPromptInstallsNothing(t *testing.T) {
	installed := setUpdateEnv(t, "3.0.0", true, release("3.1.0"), nil)
	getUpdateYes = false
	confirmFn = func(string) bool { return false }

	captureStdout(func() {
		if err := runGetUpdate(nil, nil); err != nil {
			t.Fatalf("declining should not be an error, got: %v", err)
		}
	})

	if *installed {
		t.Error("nothing should be installed when the prompt is declined")
	}
}

// A binary built from source has no release to compare against. Silently
// replacing a developer's own build with a published one would be worse
// than doing nothing.
//
// Both spellings matter. "dev" is the ldflags default, but a real "go build"
// in a checkout reports a Go pseudo-version instead, and the pseudo-version
// case is the one that used to slip through: it parses as an ordinary
// 3.0.2, which compares older than the current release, so get-update
// offered to overwrite an uncommitted local build. Asserting only on "dev"
// is what let that ship, so both are pinned here.
func TestGetUpdate_SourceBuildRefusesToOverwriteItself(t *testing.T) {
	for _, version := range []string{
		"dev",
		"v3.0.2-0.20260914175157-eb4982ebc6d1+dirty", // real "go build" output
		"v3.0.2-0.20260914175157-eb4982ebc6d1",       // committed tree
		"v0.0.0-20260914175157-eb4982ebc6d1",         // untagged module
		"v3.1.0+dirty",                               // tagged but modified
	} {
		t.Run(version, func(t *testing.T) {
			installed := setUpdateEnv(t, version, false, release("3.1.0"), nil)

			var out string
			stdout := captureStdout(func() {
				out = captureStderr(func() {
					if err := runGetUpdate(nil, nil); err != nil {
						t.Fatalf("runGetUpdate: %v", err)
					}
				})
			})
			out += stdout

			if *installed {
				t.Error("a source build must never be overwritten automatically")
			}
			if !strings.Contains(out, "nothing to compare against") {
				t.Errorf("expected a source-build message, got:\n%s", out)
			}
		})
	}
}

func TestIsPublishedVersion(t *testing.T) {
	published := []string{"3.1.0", "v3.1.0", "v3.1.0-rc1", "10.2.3"}
	source := []string{
		"", "dev",
		"v3.0.2-0.20260914175157-eb4982ebc6d1+dirty",
		"v3.0.2-0.20260914175157-eb4982ebc6d1",
		"v0.0.0-20260914175157-eb4982ebc6d1",
		"v3.1.0+dirty",
	}
	for _, v := range published {
		if !isPublishedVersion(v) {
			t.Errorf("isPublishedVersion(%q) = false, want true", v)
		}
	}
	for _, v := range source {
		if isPublishedVersion(v) {
			t.Errorf("isPublishedVersion(%q) = true, want false", v)
		}
	}
}

// Running ahead of the published release (a release candidate, or a local
// build tagged higher) is not a reason to downgrade.
func TestGetUpdate_AheadOfLatestInstallsNothing(t *testing.T) {
	installed := setUpdateEnv(t, "4.0.0", true, release("3.1.0"), nil)

	captureStdout(func() {
		if err := runGetUpdate(nil, nil); err != nil {
			t.Fatalf("runGetUpdate: %v", err)
		}
	})

	if *installed {
		t.Error("a newer local version must not be downgraded")
	}
}

func TestGetUpdate_ReportsLookupFailure(t *testing.T) {
	setUpdateEnv(t, "3.0.0", true, selfupdate.Release{}, errors.New("network unreachable"))

	err := runGetUpdate(nil, nil)
	if err == nil {
		t.Fatal("expected an error when the release lookup fails")
	}
	if !strings.Contains(err.Error(), "network unreachable") {
		t.Errorf("expected the cause to be surfaced, got: %v", err)
	}
}

func TestGetUpdate_OfflineIsRefused(t *testing.T) {
	setUpdateEnv(t, "3.0.0", true, release("3.1.0"), nil)
	prev := offlineMode
	offlineMode = true
	t.Cleanup(func() { offlineMode = prev })

	err := runGetUpdate(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Errorf("expected --offline to be refused with a network message, got: %v", err)
	}
}

// --- install-method detection ---

func TestCurrentInstallMethod(t *testing.T) {
	cases := []struct {
		name        string
		version     string
		fromLDFlags bool
		want        installMethod
	}{
		{"goreleaser sets ldflags", "3.1.0", true, installRelease},
		{"go install embeds a module version", "v3.1.0", false, installGoInstall},
		{"plain go build has neither", "dev", false, installFromSource},
		// A source build reports a pseudo-version, not "dev" — treating that
		// as a go install version is what made the guard miss.
		{"go build stamps a pseudo-version", "v3.0.2-0.20260914175157-eb4982ebc6d1+dirty", false, installFromSource},
		{"a dirty release build is still a source build", "v3.1.0+dirty", true, installFromSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prevV, prevL := appVersion, ldflagsInjected
			appVersion, ldflagsInjected = tc.version, tc.fromLDFlags
			t.Cleanup(func() { appVersion, ldflagsInjected = prevV, prevL })

			if got := currentInstallMethod(); got != tc.want {
				t.Errorf("currentInstallMethod() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The suggested command has to match how the binary got there: telling a
// release-binary user to run "go install" sends them somewhere that will
// not update the binary on their PATH.
func TestUpgradeCommand_MatchesInstallMethod(t *testing.T) {
	if got := upgradeCommand(installGoInstall); !strings.Contains(got, "go install") {
		t.Errorf("go install builds should be told to re-run go install, got %q", got)
	}
	if got := upgradeCommand(installGoInstall); !strings.Contains(got, "/v3@latest") {
		t.Errorf("the suggested command must carry the /v3 suffix, got %q", got)
	}
	if got := upgradeCommand(installRelease); strings.Contains(got, "go install") {
		t.Errorf("release binaries should not be told to use go install, got %q", got)
	}
}
