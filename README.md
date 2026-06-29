# gitcollect

A standalone Go CLI that groups GitHub/GitLab repositories into named
**collections** and controls who can access them — at both the collection
level (membership) and the repo level (which groups or individuals can
reach which repos).

gitcollect's local YAML manifest is a *declaration of intent*; the
GitHub/GitLab platform is the *enforcement point*. Every access mutation
drives the platform API to completion before the local YAML is written.

See [PROMPT.md](PROMPT.md) for the full design spec, or browse the
**[full command reference and docs page](docs/index.html)** for every
command and flag with descriptions, plus a [two-person
walkthrough](docs/index.html#walkthrough) of sharing a collection with a
teammate end to end.
(It's a static HTML file — open `docs/index.html` directly in a browser,
or enable GitHub Pages on this repo pointed at `/docs` to host it online;
clicking the link on GitHub's own file viewer shows the raw source, not
the rendered page.)

## Prerequisites

- Go 1.26+ (see `go.mod`)
- `git` on your `PATH` (required for `clone`/`pull`/`status`)
- A GitHub or GitLab personal access token (for `gitcollect auth`)

All commands below are plain `go` commands — no Make, no shell scripts.
They work identically once you're in the right shell for your OS; the only
differences are path separators and the home-directory environment
variable, called out per OS below.

## Setup, build, and run

### Windows (PowerShell)

```powershell
git clone <this-repo>
cd gitcollect
go mod download

go build -ldflags="-s -w -X main.version=dev" -o bin/gitcollect.exe .
.\bin\gitcollect.exe --help

# or run without building a binary first
go run . --help
```

Install onto `%GOBIN%` (or `%GOPATH%\bin`):

```powershell
go install -ldflags="-s -w" .
```

> **Home directory:** gitcollect's local state path resolves via Go's
> `os.UserHomeDir()`, which on Windows reads `%USERPROFILE%`, **not**
> `$HOME`. If you use Git Bash, `export HOME=...` will *not* affect the
> compiled binary — set `$env:USERPROFILE` in PowerShell instead if you
> need to point it at a custom directory.

### macOS

```bash
git clone <this-repo>
cd gitcollect
go mod download

go build -ldflags="-s -w -X main.version=dev" -o bin/gitcollect .
./bin/gitcollect --help

# or run without building a binary first
go run . --help
```

Install onto `$GOBIN` (or `$GOPATH/bin`):

```bash
go install -ldflags="-s -w" .
```

> macOS ships Xcode's Clang, so `go test -race` (see below) works out of
> the box with no extra setup.

### Linux

```bash
git clone <this-repo>
cd gitcollect
go mod download

go build -ldflags="-s -w -X main.version=dev" -o bin/gitcollect .
./bin/gitcollect --help

# or run without building a binary first
go run . --help
```

Install onto `$GOBIN` (or `$GOPATH/bin`):

```bash
go install -ldflags="-s -w" .
```

> If `go test -race` complains about a missing C compiler, install one
> (e.g. `sudo apt install build-essential` on Debian/Ubuntu, `sudo dnf
> groupinstall "Development Tools"` on Fedora) — most desktop/CI Linux
> images already have gcc preinstalled.

### First-time use (all platforms)

```bash
gitcollect auth                       # store a GitHub token (hidden prompt)
gitcollect auth --host gitlab.com     # or authenticate against GitLab
gitcollect whoami                     # confirm it worked

gitcollect init my-collection          # create the collection first
gitcollect add my-collection some-repo # then add repos to it
gitcollect member add my-collection some-username
gitcollect show my-collection
```

> `add`/`member add`/etc. all require the collection to already exist —
> `gitcollect init <name>` first, or you'll get
> `collection "..." not found. Run: gitcollect list`.

`gitcollect auth` only needs to be run once per host. The token is saved
to `~/.gitcollect/config` and reused by every later command — gitcollect
never re-prompts just because time has passed. It only stops working once
the token itself actually expires/is revoked on GitHub's or GitLab's side
(whatever expiration you picked when generating it); the next command you
run after that will fail with something like `invalid or missing token`
plus a `Run: gitcollect auth` hint telling you to generate a fresh one.

`gitcollect list` shows every collection you own or are a member of, public
or private. Narrow it to one visibility with `--private` or `--public`:

```bash
gitcollect list             # everything you own or are a member of
gitcollect list --private   # just the private ones
gitcollect list --public    # just the public ones
```

All local state lives under `~/.gitcollect/` (`config` for tokens at file
mode `0600`, `collections/*.yaml` for manifests, `audit/*.log` for the
access-change audit trail, `activity/*.log` for the code-activity log — see
below). Nothing is written there until you run `auth` or `init`.

### Listing the repos inside a collection — and which ones you can reach

```bash
gitcollect show my-collection
```

Prints a summary including a `REPO | ACCESS RULE | YOU` table. `ACCESS
RULE` is the configured rule (open to all members / restricted to groups
or users); `YOU` is personal to whoever runs the command — `✓ yes`, or
`✗ no — <reason>` if you're denied (e.g. `✗ no — no access — group
red-team required`). If anything is denied, a line below the table lists
every repo you can't reach and points you at `inspect --user` for more
detail. `gitcollect clone` only ever clones the repos marked `✓` here —
the two are backed by the same access decision, so `show` always tells
you in advance what `clone` will actually fetch.

For a per-member access breakdown (who can reach which repo, and why),
use `inspect` instead:

```bash
gitcollect inspect my-collection
```

```
Collection:  my-collection
Visibility:  private
Members:     1

MEMBER         learning-hub
sreekutty2728  ✓
```

Each column after `MEMBER` is one repo in the collection (`learning-hub`
above is a repo name, not a label) — `✓`/`✗` shows whether that row's
member can access it. Use `gitcollect inspect my-collection --repo
<repo-name>` to flip the view (one repo, every member), or `--user
<username>` to see one member's full access map with the reason for each
decision.

### Granting or revoking one user's access to a specific repo

`gitcollect repo access <collection> <repo> --users u1,u2` *replaces* a
repo's entire individual-access list, so you need to already know everyone
currently on it. For adding or removing just one person without touching
anyone else's access, use `grant`/`revoke` instead:

```bash
gitcollect repo grant my-collection some-repo some-username   # add one user
gitcollect repo revoke my-collection some-repo some-username  # remove one user
```

Both require the collection owner's token (same as every other access
mutation) and leave the repo's group restrictions untouched. A couple of
guardrails worth knowing:

- `repo grant` refuses if the repo is currently **open to all members**
  (no group/user restriction at all) — adding one user to an empty list
  would flip the rule from "everyone" to "only this one user," silently
  locking everyone else out. Use `repo access --users <name>` if you
  actually mean to restrict an open repo.
- `repo revoke` refuses if removing that user would leave the repo with
  **no restriction at all**, which would silently re-open it to every
  member. Use `repo access --users <remaining-names>` if that's what you
  actually want.

Both commands are no-ops (not errors) if the user already has, or already
lacks, that individual grant.

### Seeing code changes across a collection's repos

`gitcollect audit` only tracks *access* changes — member/group/repo-access
mutations made through gitcollect itself. It has no idea what's actually
been committed to the repos. For that, use `activity`:

```bash
gitcollect activity my-collection
```

This fetches the most recent commits on each accessible repo's **default
branch** directly from GitHub/GitLab (live — no daemon, no polling, just
whatever's true right now), records any genuinely new ones to
`~/.gitcollect/activity/<collection>.log`, and prints the combined history
(everything previously recorded plus this run's fetch) as one table sorted
newest-first, with the author and branch for every commit:

```
REPO          BRANCH  AUTHOR  SHA      MESSAGE         WHEN
learning-hub  main    alice   a1b2c3d  Fix login bug   2026-06-28 14:02
learning-hub  main    bob     9f8e7d6  Add tests        2026-06-27 09:11
```

Useful flags:
- `--repo <name>` — only check one repo instead of every accessible one
- `--since 7d` — only show commits within the last N days (also accepts `24h`, `30m`, etc.)
- `--limit N` — how many commits to fetch per repo this run (default 10) — this only bounds the live fetch; previously recorded commits beyond that window are still shown from the log
- `--json` — machine-readable output

Like `clone`/`pull`/`status`, this always needs a live, authenticated API
call (to resolve the default branch and list commits), even for public
collections — there's no local-only path for "what got committed."

> A [Makefile](Makefile) and [.goreleaser.yaml](.goreleaser.yaml) wrap these
> same commands (`make build`, `make test`, `goreleaser release`, etc.) for
> anyone who already has GNU Make / goreleaser on their `PATH` — they're
> optional conveniences, not requirements. Everything in this README runs
> with plain `go` commands on every OS.

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

The full CI-equivalent command:

```bash
go test ./... -race -cover -coverprofile=coverage.out
```

> **Note on `-race`:** the race detector requires CGO and a C toolchain.
> macOS and most Linux setups have one already (see the OS sections
> above); on Windows you'd need a C compiler such as
> [mingw-w64](https://www.mingw-w64.org/) on `PATH` with
> `CGO_ENABLED=1` set. If you don't have that configured, drop `-race`
> and run the plain command above instead — CI runs the race-enabled
> target on Linux regardless.

Run a single package or test by name:

```bash
go test ./internal/access/... -run TestAccessDecisionMatrix -v
```

### What the tests don't need

All tests are self-contained: they use `t.TempDir()` and
`t.Setenv("HOME"/"USERPROFILE", ...)` to isolate `~/.gitcollect`, in-memory
mock `api.Client` implementations to avoid real network calls, and (for
`internal/git`) a fake `git` executable placed on `PATH` to verify
subprocess arguments without invoking real git commands. No tokens, network
access, or real GitHub/GitLab accounts are needed to run the suite, on any
OS.

## Linting

```bash
golangci-lint run ./...
```

## Releasing

Release builds are configured in [.goreleaser.yaml](.goreleaser.yaml) and
cut via:

```bash
goreleaser release --clean
```
