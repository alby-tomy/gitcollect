# gitcollect

A standalone Go CLI that groups GitHub/GitLab repositories into named
**collections** and controls who can access them — at both the collection
level (membership) and the repo level (which groups or individuals can
reach which repos).

gitcollect's local YAML manifest is a *declaration of intent*; the
GitHub/GitLab platform is the *enforcement point*. Every access mutation
drives the platform API to completion before the local YAML is written.

See [PROMPT.md](PROMPT.md) for the full design spec.

## Prerequisites

- Go 1.26+ (see `go.mod`)
- `git` on your `PATH` (required for `clone`/`pull`/`status`)
- A GitHub or GitLab personal access token (for `gitcollect auth`)

## Setup

```bash
git clone <this-repo>
cd gitcollect
go mod download
```

## Running the project

Build a binary:

```bash
make build          # -> bin/gitcollect (or bin/gitcollect.exe on Windows)
./bin/gitcollect --help
```

> `make` isn't preinstalled on plain Windows. If you don't have it (e.g. via
> [Git for Windows](https://gitforwindows.org/), [Chocolatey](https://chocolatey.org/) `choco install make`, or WSL),
> run the build's underlying command directly instead:
>
> ```powershell
> go build -ldflags="-s -w -X main.version=dev" -o bin/gitcollect.exe .
> .\bin\gitcollect.exe --help
> ```

Or run directly without building (handy while developing):

```bash
go run . --help
go run . version
```

Install it onto your `$GOPATH/bin` (or `$GOBIN`):

```bash
make install
```

### First-time use

```bash
gitcollect auth                       # store a GitHub token (hidden prompt)
gitcollect auth --host gitlab.com     # or authenticate against GitLab
gitcollect whoami                     # confirm it worked

gitcollect init my-collection
gitcollect add my-collection some-repo
gitcollect member add my-collection some-username
gitcollect show my-collection
```

All local state lives under `~/.gitcollect/` (`config` for tokens at file
mode `0600`, `collections/*.yaml` for manifests, `audit/*.log` for the audit
trail). Nothing is written there until you run `auth` or `init`.

> **Windows note:** `gitcollect`'s local state path is resolved via Go's
> `os.UserHomeDir()`, which reads `%USERPROFILE%` on Windows (not `$HOME`).
> If you're pointing the CLI at a custom home directory for testing, set
> `USERPROFILE`, not `HOME`, in PowerShell/cmd. Git Bash's `export HOME=...`
> has no effect on the compiled binary.

## Running the tests

Run everything:

```bash
go test ./...
```

With coverage (the project targets 80%+ on every `internal/` package):

```bash
go test ./... -cover
```

Per-package, with a function-level coverage breakdown:

```bash
go test ./internal/collection/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Or view it in a browser:

```bash
go tool cover -html=coverage.out
```

Via the Makefile (matches CI):

```bash
make test            # go test ./... -race -cover -coverprofile=coverage.out
```

(No `make`? Run the `go test ./... -race -cover -coverprofile=coverage.out` command shown above directly.)

> **Note on `-race`:** the race detector requires CGO and a C toolchain.
> If you see `-race requires cgo; enable cgo by setting CGO_ENABLED=1` on a
> Windows box without a configured C compiler, run the plain (non-race)
> command above instead — CI runs the race-enabled target on Linux.

Run a single package or test by name:

```bash
go test ./internal/access/... -run TestAccessDecisionMatrix -v
```

### What the tests don't need

All tests are self-contained: they use `t.TempDir()` and `t.Setenv("HOME"/"USERPROFILE", ...)`
to isolate `~/.gitcollect`, in-memory mock `api.Client` implementations to
avoid real network calls, and (for `internal/git`) a fake `git` executable
placed on `PATH` to verify subprocess arguments without invoking real git
commands. No tokens, network access, or real GitHub/GitLab accounts are
needed to run the suite.

## Linting

```bash
make lint            # requires golangci-lint on PATH
```

## Releasing

Release builds are configured in [.goreleaser.yaml](.goreleaser.yaml) and
cut via:

```bash
make release          # goreleaser release --clean
```
