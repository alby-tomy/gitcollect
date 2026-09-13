package main

import (
	"os"
	"runtime/debug"

	"github.com/alby-tomy/gitcollect/v3/cmd"
)

// version is injected at build time via:
//
//	go build -ldflags="-X main.version=$(git describe --tags --always)"
//
// Release builds (goreleaser, Makefile) always set it. Builds produced by
// "go install github.com/alby-tomy/gitcollect/v3@latest" do not — go install
// applies no ldflags — so it stays "dev" there and resolveVersion recovers
// the real version from the module metadata the go tool embeds instead.
var version = "dev"

// resolveVersion returns the version to report. The ldflags-injected value
// wins when present; otherwise the module version recorded in the binary's
// build info is used, which is what makes a "go install ...@v3.0.0" binary
// report "v3.0.0" rather than "dev". Falls back to "dev" for a plain
// "go build" from a source tree, where neither is available.
func resolveVersion() string {
	if version != "dev" && version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	// "(devel)" is what the go tool records for a build from a local
	// source tree rather than a resolved module version — no more
	// informative than "dev", so keep the name gitcollect already uses.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return version
}

func main() {
	cmd.SetVersion(resolveVersion())
	os.Exit(cmd.Execute())
}
