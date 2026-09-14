// Package git wraps the git CLI for the clone, pull, and status commands.
// gitcollect never speaks the git protocol directly — it shells out, the
// same way a developer would.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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
	return runEnv(dir, nil, args...)
}

// runEnv is run with extra environment variables appended to the inherited
// environment. Kept separate so only the calls that actually need to pass
// a secret do so — see tokenEnv.
func runEnv(dir string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
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

// tokenEnvVar names the environment variable the inline credential helper
// below reads the token out of.
const tokenEnvVar = "GITCOLLECT_GIT_TOKEN"

// credentialArgs returns the git -c flags that let a clone authenticate
// with token, or nil when there is no token to pass.
//
// The token is handed over through the environment rather than any of the
// more obvious routes, all of which leak it:
//
//   - in the URL (https://user:token@host/...) git writes it permanently
//     into .git/config and echoes it back in error messages;
//   - in -c http.extraHeader=... it lands in argv, and /proc/<pid>/cmdline
//     is readable by other users on the machine by default;
//   - in a credential-helper script it has to be written to disk.
//
// /proc/<pid>/environ, by contrast, is readable only by the process owner.
// The first, empty credential.helper drops any helper the user has
// configured globally, so ours is the only one consulted and a stale
// cached credential cannot silently win.
//
// The username is a placeholder: GitHub and GitLab both authenticate on
// the token in the password field and ignore what precedes it.
func credentialArgs(token string) []string {
	if token == "" {
		return nil
	}
	helper := `!f() { echo username=x-access-token; echo "password=$` + tokenEnvVar + `"; }; f`
	return []string{"-c", "credential.helper=", "-c", "credential.helper=" + helper}
}

// tokenEnv returns the environment entry carrying token, or nil when empty.
func tokenEnv(token string) []string {
	if token == "" {
		return nil
	}
	return []string{tokenEnvVar + "=" + token}
}

// Clone clones cloneURL into dest using whatever credentials git already
// has configured. cloneURL must be HTTPS — gitcollect never clones over
// SSH. Prefer CloneWithToken: a private repo cannot be cloned this way
// unless the user happens to have a credential helper set up, which is
// why clone used to prompt or fail for exactly the repos gitcollect
// exists to manage.
func Clone(cloneURL, dest string) error {
	return CloneWithToken(cloneURL, dest, "")
}

// CloneWithToken clones cloneURL into dest, authenticating with token.
// An empty token behaves exactly like Clone. The token is passed to git
// through the environment and never reaches argv or the cloned repo's
// config — see credentialArgs.
func CloneWithToken(cloneURL, dest, token string) error {
	if !strings.HasPrefix(cloneURL, "https://") {
		return fmt.Errorf("refusing to clone non-HTTPS URL: %s", cloneURL)
	}
	args := append(credentialArgs(token), "clone", cloneURL, dest)
	if _, err := runEnv("", tokenEnv(token), args...); err != nil {
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

// ShallowClone clones cloneURL into dest with depth 1 (only the latest
// commit), using whatever credentials git already has configured. Faster
// than a full clone for the publish/pull-config flow where only the file
// tree matters and full history is not needed.
func ShallowClone(cloneURL, dest string) error {
	return ShallowCloneBranch(cloneURL, dest, "", "")
}

// ShallowCloneBranch clones cloneURL into dest at depth 1, authenticating
// with token and checking out branch. An empty branch takes the remote's
// default; an empty token behaves like ShallowClone.
//
// The branch is selected during the clone rather than checked out
// afterwards because --depth=1 fetches only the branch it clones: a later
// "git checkout <other>" has nothing to switch to and fails. That is why
// publish could not push to any branch but the remote's default, despite
// offering a --branch flag.
func ShallowCloneBranch(cloneURL, dest, branch, token string) error {
	if !strings.HasPrefix(cloneURL, "https://") {
		return fmt.Errorf("refusing to clone non-HTTPS URL: %s", cloneURL)
	}
	args := append(credentialArgs(token), "clone", "--depth=1")
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, cloneURL, dest)
	if _, err := runEnv("", tokenEnv(token), args...); err != nil {
		return err
	}
	return nil
}

// Checkout switches the working tree inside dir to branch.
func Checkout(dir, branch string) error {
	if _, err := run(dir, "checkout", branch); err != nil {
		return err
	}
	return nil
}

// Add stages all files under path inside dir.
func Add(dir, path string) error {
	if _, err := run(dir, "add", path); err != nil {
		return err
	}
	return nil
}

// Commit creates a commit inside dir with message. Returns no error if
// there is nothing to commit (git exits 1 with "nothing to commit").
func Commit(dir, message string) error {
	_, err := run(dir, "commit", "-m", message)
	if err != nil && strings.Contains(err.Error(), "nothing to commit") {
		return nil
	}
	return err
}

// Push pushes the current branch inside dir to its upstream remote using
// whatever credentials git already has configured.
func Push(dir string) error {
	return PushWithToken(dir, "")
}

// PushWithToken pushes the current branch inside dir, authenticating with
// token. An empty token behaves exactly like Push. Needed because publish
// writes to a config repository that is usually private — without a
// credential the push prompts or fails, which is the same gap CloneWithToken
// closes on the way in.
func PushWithToken(dir, token string) error {
	args := append(credentialArgs(token), "push")
	if _, err := runEnv(dir, tokenEnv(token), args...); err != nil {
		return err
	}
	return nil
}

// Status returns the output of "git status --short" for dir.
func Status(dir string) (string, error) {
	return run(dir, "status", "--short")
}

// HasUncommittedChanges returns true if the repo at dir has any uncommitted
// changes (staged or unstaged). Runs git status --porcelain. Returns false
// cleanly if dir is not a git repo or git is not installed — never an error
// for those cases.
func HasUncommittedChanges(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, nil
	}
	return out != "", nil
}

// CurrentBranch returns the name of the currently checked-out branch in dir.
// Returns "" (not an error) when the repo is in detached HEAD state or dir is
// not a git repository.
func CurrentBranch(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", nil
	}
	if out == "HEAD" {
		return "", nil // detached HEAD
	}
	return out, nil
}

// CommitsBehind returns how many commits the local HEAD is behind origin/HEAD.
// Returns 0 (not an error) when there is no remote or the tracking branch
// does not exist yet — callers treat "no remote" and "up to date" the same.
func CommitsBehind(dir string) (int, error) {
	out, err := run(dir, "rev-list", "HEAD..origin/HEAD", "--count")
	if err != nil {
		return 0, nil
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n, nil
}
