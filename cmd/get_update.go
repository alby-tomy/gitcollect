package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/output"
	"github.com/alby-tomy/gitcollect/v3/internal/selfupdate"
)

var (
	getUpdateCheck bool
	getUpdateYes   bool

	// latestReleaseFn is injectable so tests can exercise the command
	// without reaching GitHub.
	latestReleaseFn = func() (selfupdate.Release, error) { return selfupdate.Latest(nil) }
	// downloadReleaseFn and replaceBinaryFn are likewise injectable: the
	// real ones fetch over the network and overwrite the running binary,
	// neither of which belongs in a unit test.
	downloadReleaseFn = func(rel selfupdate.Release) ([]byte, error) {
		return selfupdate.Download(nil, rel, runtime.GOOS, runtime.GOARCH)
	}
	replaceBinaryFn = selfupdate.Replace
	// executablePathFn resolves the binary to replace.
	executablePathFn = os.Executable
	// confirmFn is the y/N prompt, injectable for the same reason.
	confirmFn = func(msg string) bool { return output.Confirm(msg) }
	// goInstallFn runs the go toolchain. It is injectable for a sharper
	// reason than the others: without it a test that misclassifies its
	// version shells out to a real "go install", hitting the network and
	// rewriting the developer's GOBIN. That is not hypothetical — it is
	// what this suite did before the pseudo-version fix.
	goInstallFn = runGoInstall
)

var getUpdateCmd = &cobra.Command{
	Use:     "get-update",
	Aliases: []string{"update", "upgrade", "self-update"},
	Short:   "Check for a newer gitcollect and install it",
	Long: `Compare the running version against the latest published release and, if
there is a newer one, install it.

How the upgrade happens depends on how gitcollect was installed:

  go install    re-runs "go install ` + selfupdate.Module + `@latest",
                so the Go toolchain stays the source of truth for the
                binary it manages.
  release       downloads the archive for this platform, verifies it
  binary        against the published SHA-256 checksum, and replaces the
                binary in place. An unverifiable download is refused, not
                installed.

Use --check to report only and change nothing — useful in a script or a
shell prompt. Use --yes to skip the confirmation.

A binary built from source reports a development version (either "dev" or
a Go pseudo-version such as v3.0.2-0.20260914175157-eb4982ebc6d1+dirty) and
has no release to compare against; get-update says so and stops rather than
overwriting your own build.

Examples:
  gitcollect get-update
  gitcollect get-update --check
  gitcollect get-update --yes`,
	Args: cobra.NoArgs,
	RunE: runGetUpdate,
}

func init() {
	getUpdateCmd.Flags().BoolVar(&getUpdateCheck, "check", false, "report whether an update exists, change nothing")
	getUpdateCmd.Flags().BoolVar(&getUpdateYes, "yes", false, "do not prompt before installing")
	rootCmd.AddCommand(getUpdateCmd)
}

// installMethod is how this binary was most likely produced, which decides
// how it can be upgraded safely.
type installMethod int

const (
	// installFromSource is a plain "go build" in a working tree: no
	// ldflags version and no module version. Never upgraded automatically.
	installFromSource installMethod = iota
	// installGoInstall came from "go install <module>@version": the Go
	// tool recorded a module version but injected no ldflags.
	installGoInstall
	// installRelease came from a goreleaser archive, which sets the
	// version through ldflags.
	installRelease
)

// ldflagsInjected records whether main's version variable was set at build
// time. Release builds set it; "go install" does not. That difference is
// the only reliable signal of how the binary was produced, so main reports
// it here rather than having this package guess.
var ldflagsInjected bool

// SetBuildSource records whether the version came from ldflags. Called once
// by main, before Execute, alongside SetVersion.
func SetBuildSource(fromLDFlags bool) { ldflagsInjected = fromLDFlags }

// pseudoVersion matches the version Go stamps into a binary built from a
// source tree rather than installed from a published tag:
//
//	v3.0.2-0.20260914175157-eb4982ebc6d1
//	v0.0.0-20260914175157-eb4982ebc6d1
//
// Note the two separators before the timestamp: a bare pseudo-version uses
// "-", but the form built on a preceding tag uses ".", after the "0". A
// pattern anchored on "-" alone silently misses the second form, which is
// the one this repository actually produces.
//
// The "+dirty" suffix Go appends for an uncommitted tree is caught
// separately, since it can also decorate a real tag.
var pseudoVersion = regexp.MustCompile(`[-.]\d{14}-[0-9a-f]{12}`)

// isPublishedVersion reports whether v names an actual release rather than
// a build from a working tree.
//
// Checking for the literal "dev" is not enough, and assuming otherwise was
// a real defect: a plain "go build" in this repository reports
// "v3.0.2-0.20260914175157-eb4982ebc6d1+dirty", which parses as 3.0.2 and
// therefore compared as *older* than the current release — so get-update
// offered to overwrite a developer's own uncommitted build with a
// published binary, which is exactly what the source-build guard exists to
// prevent.
func isPublishedVersion(v string) bool {
	if v == "" || v == "dev" {
		return false
	}
	if strings.HasSuffix(v, "+dirty") || pseudoVersion.MatchString(v) {
		return false
	}
	return true
}

func currentInstallMethod() installMethod {
	switch {
	case ldflagsInjected && isPublishedVersion(appVersion):
		return installRelease
	case isPublishedVersion(appVersion):
		// A real version that did not come from ldflags can only have come
		// from the module metadata the go tool embeds.
		return installGoInstall
	default:
		return installFromSource
	}
}

func runGetUpdate(_ *cobra.Command, _ []string) error {
	if IsOffline() {
		return fmt.Errorf("get-update: this command requires a network connection\n  Remove --offline to enable network access")
	}

	method := currentInstallMethod()
	if method == installFromSource && !getUpdateCheck {
		output.Warn("This build reports version %q, so there is nothing to compare against.", appVersion)
		output.Dim("  It looks like a local 'go build'. Rebuild from source, or install a release:")
		output.Dim("    go install %s@latest", selfupdate.Module)
		return nil
	}

	output.Info("Checking for a newer gitcollect...")
	rel, err := latestReleaseFn()
	if err != nil {
		return fmt.Errorf("get-update: %w", err)
	}

	cmp := selfupdate.Compare(appVersion, rel.Version)
	switch {
	// Compare already ignores the leading "v", so the equality check must
	// too: a release binary reports "v3.1.0" while the release itself is
	// named "3.1.0", and comparing the raw strings sent an up-to-date user
	// down the "not comparable" branch.
	case cmp == 0 && strings.TrimPrefix(appVersion, "v") == strings.TrimPrefix(rel.Version, "v"):
		output.Success("gitcollect %s is the latest release", appVersion)
		return nil
	case cmp >= 0:
		// Ahead of, or not comparable with, the published release. Either
		// way there is nothing to install and overwriting would be wrong.
		output.Info("Running %s; the latest release is %s. Nothing to install.", appVersion, rel.Version)
		return nil
	}

	output.Success("Update available: %s → %s", appVersion, rel.Version)
	fmt.Printf("  https://github.com/alby-tomy/gitcollect/releases/tag/%s\n", rel.Tag)

	if getUpdateCheck {
		fmt.Println()
		output.Suggestion(upgradeCommand(method))
		return nil
	}

	if !getUpdateYes && !confirmFn(fmt.Sprintf("Install %s now?", rel.Version)) {
		output.Info("Not installing.")
		output.Suggestion(upgradeCommand(method))
		return nil
	}

	switch method {
	case installGoInstall:
		return goInstallFn()
	default:
		return upgradeViaRelease(rel)
	}
}

// upgradeCommand is the command a user would run by hand to upgrade a
// binary installed the given way.
func upgradeCommand(method installMethod) string {
	if method == installGoInstall {
		return "go install " + selfupdate.Module + "@latest"
	}
	return "gitcollect get-update --yes"
}

// upgradeViaGoInstall re-runs go install rather than overwriting the
// binary directly. The Go tool owns everything under GOBIN, including the
// module cache entry that records what is installed, so replacing the file
// behind its back would leave the two disagreeing.
func runGoInstall() error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf(
			"get-update: this gitcollect was installed with 'go install', but go is not on PATH\n"+
				"  Install Go, or run: go install %s@latest", selfupdate.Module)
	}

	target := selfupdate.Module + "@latest"
	output.Info("Running: go install %s", target)

	c := exec.Command(goBin, "install", target)
	c.Stdout = os.Stderr // keep stdout clean for data, as every other command does
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("get-update: go install failed: %w", err)
	}

	output.Success("Updated via go install")
	output.Dim("  Run 'gitcollect --version' to confirm; make sure GOBIN precedes any older copy on PATH.")
	return nil
}

// upgradeViaRelease downloads, verifies and swaps in the release archive
// for this platform.
func upgradeViaRelease(rel selfupdate.Release) error {
	path, err := executablePathFn()
	if err != nil {
		return fmt.Errorf("get-update: could not locate the running binary: %w", err)
	}

	output.Info("Downloading %s for %s/%s...", rel.Version, runtime.GOOS, runtime.GOARCH)
	binary, err := downloadReleaseFn(rel)
	if err != nil {
		return fmt.Errorf("get-update: %w", err)
	}
	output.Success("Checksum verified")

	if err := replaceBinaryFn(path, binary); err != nil {
		// Permission is the overwhelmingly common cause: the binary often
		// lives somewhere only root can write.
		if os.IsPermission(err) {
			return fmt.Errorf(
				"get-update: no permission to replace %s\n"+
					"  Re-run with elevated privileges, or reinstall the release archive by hand", path)
		}
		return fmt.Errorf("get-update: %w", err)
	}

	output.Success("Updated to %s", rel.Version)
	output.Dim("  Replaced %s", path)
	output.Dim("  Run 'gitcollect --version' to confirm.")
	return nil
}
