package main

import (
	"os"

	"github.com/alby-tomy/gitcollect/cmd"
)

// version is injected at build time via:
//   go build -ldflags="-X main.version=$(git describe --tags --always)"
// Defaults to "dev" for local, non-release builds.
var version = "dev"

func main() {
	cmd.SetVersion(version)
	os.Exit(cmd.Execute())
}
