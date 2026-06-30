// Package git wraps the git CLI for the clone, pull, and status commands.
// gitcollect never speaks the git protocol directly — it shells out, the
// same way a developer would.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNotInstalled is returned when the git executable cannot be found on
// PATH.
var ErrNotInstalled = errors.New("git is not installed or not on PATH")

// CheckInstalled verifies git is available before any git command runs.
func CheckInstalled() error {
	if _, err := exec.LookPath("git"); err != nil {
		return ErrNotInstalled
	}
	return nil
}

// run executes git with args in dir (if dir is non-empty) and returns
// trimmed stdout, or a combined error including stderr on failure.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Clone clones cloneURL into dest. cloneURL must be HTTPS — gitcollect
// never clones over SSH.
func Clone(cloneURL, dest string) error {
	if !strings.HasPrefix(cloneURL, "https://") {
		return fmt.Errorf("refusing to clone non-HTTPS URL: %s", cloneURL)
	}
	if _, err := run("", "clone", cloneURL, dest); err != nil {
		return err
	}
	return nil
}

// Pull runs "git pull" inside dir.
func Pull(dir string) error {
	if _, err := run(dir, "pull"); err != nil {
		return err
	}
	return nil
}

// PullWithSummary runs "git pull" inside dir and reports how many new
// commits it brought in, by comparing HEAD before and after. Returns 0 if
// the repo was already up to date.
func PullWithSummary(dir string) (newCommits int, err error) {
	before, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		return 0, err
	}

	if _, err := run(dir, "pull"); err != nil {
		return 0, err
	}

	after, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		return 0, err
	}
	if before == after {
		return 0, nil
	}

	count, err := run(dir, "rev-list", "--count", before+".."+after)
	if err != nil {
		// The pull itself already succeeded; not being able to count the
		// commits afterward shouldn't be reported as a failed sync.
		return 0, nil
	}
	n, err := strconv.Atoi(count)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

// Status returns the output of "git status --short" for dir.
func Status(dir string) (string, error) {
	return run(dir, "status", "--short")
}
