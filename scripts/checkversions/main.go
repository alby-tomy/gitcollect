// Command checkversions fails if a documented version sample disagrees with
// the version being released.
//
// These samples have drifted three times. Twice they were merely stale; once
// the sample read v1.0.0, which is exactly what you get when you install
// without the /v3 module suffix — so the page confirmed the very failure it
// warned about two sections earlier. Correcting them by hand after each
// release is what kept failing, so the release gates on this instead.
//
// It is written in Go rather than shell so it runs natively wherever the
// project does. The first version was a bash script, which is no use to a
// contributor on Windows without a POSIX shell — and the release is exactly
// the moment you want to reproduce a failure locally.
//
// Usage:
//
//	go run ./scripts/checkversions -version v3.3.0
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// checked are the files that carry version samples.
var checked = []string{
	"README.md",
	"MANUAL.md",
	filepath.Join("docs", "index.html"),
	filepath.Join("docs", "commands.html"),
}

// sample matches the two shapes that claim to be the *running* version: the
// platform line printed by "gitcollect version", and get-update's up-to-date
// message.
//
// Prose that names an older version deliberately — the get-update walkthrough
// upgrading one release to the next, or a "requires vX or newer" note — is
// left alone, because those are correct precisely by not being current. That
// is why this matches two specific sentences rather than every version-shaped
// string in the docs.
var sample = regexp.MustCompile(`gitcollect v(\d+\.\d+\.\d+) (?:(?:linux|darwin|windows)/[a-z0-9]+|is the latest release)`)

func main() {
	version := flag.String("version", os.Getenv("GITHUB_REF_NAME"),
		"the version being released, e.g. v3.3.0 (defaults to $GITHUB_REF_NAME)")
	flag.Parse()

	want := strings.TrimPrefix(strings.TrimSpace(*version), "v")
	if want == "" {
		fmt.Fprintln(os.Stderr, "checkversions: no version given; pass -version v3.3.0")
		os.Exit(2)
	}

	checkedCount, bad := scan(want)

	if checkedCount == 0 {
		// Finding nothing means the pattern has fallen out of step with the
		// docs, which would make this check silently useless — the failure
		// mode it exists to prevent.
		fmt.Fprintln(os.Stderr, "checkversions: no version samples found — the pattern may be out of date")
		os.Exit(1)
	}

	if len(bad) > 0 {
		for _, b := range bad {
			// GitHub Actions renders this as an annotation on the line.
			fmt.Printf("::error file=%s,line=%d::version sample says v%s but the release is v%s — update it\n",
				b.file, b.line, b.got, want)
			fmt.Fprintf(os.Stderr, "  %s:%d: %s\n", b.file, b.line, b.text)
		}
		fmt.Fprintf(os.Stderr, "\nVersion samples must match the tag being released. Update them in the\n"+
			"commit you tag, so a released page never advertises a version that is\n"+
			"not the one shipping.\n")
		os.Exit(1)
	}

	fmt.Printf("version samples agree with v%s (%d checked)\n", want, checkedCount)
}

type mismatch struct {
	file string
	line int
	got  string
	text string
}

// scan reports how many samples were found and which disagree with want.
func scan(want string) (int, []mismatch) {
	var found int
	var bad []mismatch

	for _, name := range checked {
		data, err := os.ReadFile(name)
		if err != nil {
			// A missing file is not a failure: the docs set may differ
			// between branches, and this check is about disagreement, not
			// about enforcing which files exist.
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range sample.FindAllStringSubmatch(line, -1) {
				found++
				if m[1] != want {
					bad = append(bad, mismatch{
						file: filepath.ToSlash(name),
						line: i + 1,
						got:  m[1],
						text: strings.TrimSpace(line),
					})
				}
			}
		}
	}
	return found, bad
}
