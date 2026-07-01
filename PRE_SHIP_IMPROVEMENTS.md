# gitcollect — Pre-Ship Improvement Session

> Read this file completely before touching any code.
> This is a structured briefing, not a spec to implement blindly.
> Every item explains WHY it matters — understand the reasoning before acting.
> Work through items in the exact priority order given below.
> After each item: run `go build ./...` and `go test ./...` before moving on.
> Update PROMPT.md's progress tracker as you complete each item.

---

## Context — where we are

As of Session 13, gitcollect has:
- `go build ./...` clean
- `go test ./...` all 9 packages green
- Identity migration complete (immutable platform IDs)
- All `internal/` packages above 80% coverage
- `cmd` package at ~15.5% coverage (known deferred gap)

A senior engineering review identified gaps that must be addressed before
this tool is published. The items below are ordered by risk — highest risk
to shipping a broken or confusing tool comes first.

---

## PRIORITY 1 — Critical: cmd package coverage

### What the gap is
The `cmd` package sits at approximately 15.5% test coverage. The session log
records nine consecutive entries of "Next session should start with:
cmd/list_test.go" — meaning this was deferred every single session and never
done. This is the highest-risk gap in the entire codebase.

### Why this matters before shipping
The `cmd` layer is where all user-facing behaviour lives. It is where:
- Arguments get parsed and validated
- Flags get applied
- The owner gate appears nine separate times across different command files
- Error messages get formatted and printed
- The authenticated caller's identity flows into every operation

At 15.5% coverage there is essentially no automated verification that any
command actually works end to end. A bug in flag parsing, argument validation,
or the owner gate would not be caught by any existing test. A user downloads
the binary, runs `gitcollect clone cybersecurity --pick "r1 r2"` and gets a
confusing error — nothing in the test suite would have caught this.

### What to build
You do not need to reach 80% on cmd — that would require testing Cobra's
framework behaviour which is not valuable. The target is 50%+ on the
pure-logic helpers and the highest-risk command paths.

Test these in priority order:

**1. `cmd/root.go` — the shared helpers every command depends on**
Test `loadForOwner`, `loadForRead`, `loadForGit`, `currentUser`,
`currentUserID`, and `migrateIfNeeded`. These are the functions called
by every single command. A bug here affects the entire tool.

```go
// Example: test that loadForOwner rejects a non-owner caller
func TestLoadForOwner_RejectsNonOwner(t *testing.T) { ... }

// Example: test that migrateIfNeeded upgrades a v1 collection
func TestMigrateIfNeeded_UpgradesV1(t *testing.T) { ... }

// Example: test that currentUser caches — GetAuthenticatedUser called once
func TestCurrentUser_CachesResult(t *testing.T) { ... }
```

**2. `cmd/clone.go` — highest user impact**
Test the `--pick` flag parsing (space-separated, must handle quotes),
`--dry-run` output, access filtering before any git calls happen,
and the pending-invite warning path.

**3. `cmd/member.go` — most complex command**
Test single-username add, multi-username add (batch), the
`--confirm-self` guard on remove, and the guided error when a
non-member is added to a group.

**4. `cmd/show.go` — two different code paths**
Test that member view (YOU column) and owner view (WHO HAS ACCESS column)
are selected correctly based on whether the caller's ID matches col.OwnerID.

**5. `cmd/list.go` — format-aware role detection**
Test that Version "2" collections compare by ID and Version "1" collections
compare by cached login, since this branching logic is new from Session 13
and has zero test coverage.

**6. `cmd/auth.go` — token storage and ID caching**
Test that after auth, both the login and the platform ID are cached in
config (the Session 13 addition). A regression here would break the entire
identity model on first use.

### Rules for cmd tests
- Use `t.TempDir()` for all file operations — never touch real `~/.gitcollect/`
- Mock the API client with the existing `mockClient` pattern from other test files
- Do not test Cobra's own flag parsing — test your logic that runs after parsing
- Each test file goes in the same `cmd` package (not `cmd_test`) so it can
  access unexported helpers

---

## PRIORITY 2 — Remove: repo grant and repo revoke

### What these commands do
`repo grant <collection> <repo> <user>` adds one user to a repo's individual
access list. `repo revoke` removes one user from it.

### Why they should be removed
These commands have confusing edge-case refusals that will frustrate real users:

- `repo grant` refuses with `ErrRepoOpen` when the repo is currently open to
  all members. This is technically correct — appending a single user to an
  empty Users list would silently restrict the repo to just that one person,
  revoking every other member's access. But the error message will confuse
  any user who just wants to "give alice access" without understanding the
  underlying model.

- `repo revoke` refuses with `ErrRepoWouldOpen` when revoking the last
  individually-granted user would leave both Groups and Users empty, which
  means "open to all members." Again technically correct but the error
  blocks the user without an obvious fix.

`repo access --users alice,bob` already handles every use case these commands
cover, and it does so explicitly — the caller sees the full before/after state
and understands what they are setting. There is no scenario where `repo grant`
or `repo revoke` is the right tool that `repo access` does not also handle.

### What to do
1. Remove `cmd/repo.go`'s `runRepoGrant` and `runRepoRevoke` functions
   and their Cobra subcommand registrations
2. Remove `ErrRepoOpen` and `ErrRepoWouldOpen` sentinel errors from
   `internal/collection/mutation.go` (verify nothing else references them
   with `grep -rn "ErrRepoOpen\|ErrRepoWouldOpen" .` before deleting)
3. Remove `GrantRepoUser` and `RevokeRepoUser` from
   `internal/collection/mutation.go` (verify no other call sites)
4. Remove the corresponding tests from any test files
5. Update `--help` text in `cmd/repo.go` to not mention grant/revoke
6. Add a decisions log entry to PROMPT.md explaining this removal

### What to add in their place
In `cmd/repo.go`, add a short usage note to the `repo access` command's
long description:

```
To grant or revoke access for individual users, use repo access with
--users to set the complete list:

  gitcollect repo access cybersecurity vuln-scanner --users alice bob
  gitcollect repo access cybersecurity vuln-scanner --open
```

This guides users to the right command without leaving a confusing dead end.

---

## PRIORITY 3 — Fix: org repo owner conflict

### What the problem is
The `--owner` flag was removed from `gitcollect init` in Session 11 because
"owner checks are literal string equality against the caller's own login."
This decision blocks a major real-world use case: a developer who wants to
create a collection of repositories from their company's GitHub organisation.

Example: alice works at Acme Corp. Their repos live at
`github.com/acme-corp/pen-test-tools` and `github.com/acme-corp/vuln-scanner`.
Alice cannot create a gitcollect collection for these repos because:
- Without `--owner acme-corp`, `init` sets `owner: alice` (her own login)
- When she runs `gitcollect add cybersecurity pen-test-tools`, the tool
  tries to call `client.GetRepo("alice", "pen-test-tools")` which 404s
  because the repo is under `acme-corp`, not `alice`
- There is no way for her to tell gitcollect to look under a different owner

### Why this matters
Team use is the primary value proposition of gitcollect's access control
features. A tool that can only manage repos under the authenticated user's
personal account cannot be used by any team that organises their work under
a shared org — which is most professional teams.

### The correct design
The "owner" field in a collection has two different meanings that got
conflated:

1. **Collection owner** — the person who administers this collection
   (creates it, adds members, controls access). This should always be
   the authenticated user at init time and stored as their immutable ID.

2. **Repo namespace** — the GitHub/GitLab username or org name under
   which the repos in this collection live. This is used for API path
   building only: `GET /repos/{namespace}/{repo}`.

These are separate fields and should be stored separately.

### What to build
Add a `namespace` field to the Collection struct:

```go
type Collection struct {
    // ... existing fields ...

    // Namespace is the GitHub/GitLab username or org name under which
    // the repos in this collection live. Used for API path building only.
    // Defaults to the collection owner's login if not set.
    // Example: "acme-corp" for repos at github.com/acme-corp/...
    Namespace string `yaml:"namespace,omitempty"`
}
```

Add `--namespace` flag to `gitcollect init`:

```
gitcollect init cybersecurity --namespace acme-corp
```

Everywhere that currently uses `col.Logins[col.Owner]` or `col.Owner` as
the first argument to `client.GetRepo`, `client.AddCollaborator`, etc.,
replace it with a new helper:

```go
// RepoNamespace returns the namespace to use for API calls.
// Falls back to the owner's cached login if no explicit namespace is set.
func (c *Collection) RepoNamespace() string {
    if c.Namespace != "" {
        return c.Namespace
    }
    return c.Logins[c.Owner]
}
```

The owner's identity check (IsOwner) remains ID-based and unchanged —
who administers the collection is still the authenticated user's immutable ID.
Only the repo path-building changes to use the namespace.

Update Validate() to check that namespace matches `^[a-zA-Z0-9._-]{1,100}$`
(same pattern as repo names, since org names follow the same rules).

Update the collection YAML example in PROMPT.md and the README.

Add tests:
- `TestRepoNamespace_DefaultsToOwnerLogin` — no namespace set
- `TestRepoNamespace_UsesExplicitNamespace` — namespace overrides owner login
- `TestInit_AcceptsNamespaceFlag` — cmd layer test
- `TestGetRepo_UsesNamespace` — verify API call uses namespace not owner login

---

## PRIORITY 4 — Document: collection sharing is manual

### What the gap is
The current user experience for sharing a collection with a teammate requires
manually copying a YAML file. There is no `gitcollect fetch` command. This
is the biggest UX friction point in the tool and real users will hit it
immediately when they try to collaborate.

### Why documentation matters before a fix exists
Without clear documentation, users will:
1. Try to find a way to share that doesn't exist
2. Assume the tool is broken
3. Give up and not come back

Setting expectations explicitly — "here is how it works today, here is what
is coming" — keeps users from feeling deceived.

### What to add to README.md
Add a dedicated section called "Sharing collections with teammates" between
the Installation section and the Quickstart. It must cover:

**The current manual flow — be specific, not vague:**
```
# Option A — commit your collections folder to a repo
cp -r ~/.gitcollect/collections/ ./my-collections/
git add . && git commit -m "share collections" && git push

# Teammate:
git clone https://github.com/you/my-collections
mkdir -p ~/.gitcollect/collections
cp my-collections/*.yaml ~/.gitcollect/collections/
```

**Why editing the YAML by hand grants nothing:**
Include the exact paragraph from the walkthrough doc explaining that a
teammate can only clone repos where both the local manifest AND the GitHub
collaborator status agree — editing YAML alone never grants real access
because gitcollect never called the API to add them as a collaborator.

**What is coming:**
```
# Coming in a future release
gitcollect fetch github.com/you/cybersecurity
```

Mark it clearly as not yet available. Do not write instructions for commands
that do not work.

---

## PRIORITY 5 — Document: error messages for edge cases

### Two specific error messages that need improvement

**`--since` invalid value**

Currently when a user types `--since 2w` (a reasonable guess), they get
some form of "invalid value" error. The error must explicitly list the
valid options:

```
✗ audit: invalid --since value "2w"
  Valid values: 1h, 24h, 7d, 30d, 90d
```

Find where `--since` is validated in `cmd/audit.go` and `cmd/activity.go`
and make sure the error message includes this exact list. Add a test that
verifies the error message contains all five valid values.

**`--pick` quoting**

The `--pick` flag changed from comma-separated to space-separated in
Session 11. The new syntax requires quoting when passing multiple repos:
`--pick "pen-test-tools vuln-scanner"`. Without quotes, the shell interprets
the space as argument separation and Cobra sees them as positional arguments,
not flag values.

The `--help` text for `clone` must show the quoted form explicitly:

```
--pick string   clone only these repos, space-separated (e.g. --pick "r1 r2")
```

Find the flag definition in `cmd/clone.go` and update the usage string.
Add a test that verifies two repo names passed as a single quoted string
are correctly split into a slice of two.

---

## PRIORITY 6 — Document: Windows install path warning

### What the risk is
`.goreleaser.yaml` builds `windows/amd64` binaries. The `os.UserHomeDir()`
+ `filepath.Join()` pattern is used everywhere for paths, which is correct
for Windows. But there is no evidence this has been tested on an actual
Windows machine, and PATH setup on Windows requires manual steps that Linux
and Mac users never encounter.

### What to add to README.md

In the Installation section, add a Windows-specific block:

```
### Windows

Download gitcollect_windows_amd64.zip from the releases page.
Extract it to get gitcollect.exe.

To use it from any terminal, add it to a folder that's on your PATH:

1. Create C:\Users\YourName\bin\ if it doesn't exist
2. Move gitcollect.exe there
3. Open Start → search "Environment Variables"
4. Click "Edit the system environment variables"
5. Under "System variables", find "Path" → Edit → New
6. Add: C:\Users\YourName\bin
7. Click OK, restart your terminal
8. Run: gitcollect version
```

Add a note at the top of the Windows section:

```
Note: Windows support is provided as a best-effort build. If you encounter
any Windows-specific issues, please open an issue at
https://github.com/alby-tomy/gitcollect/issues
```

This sets honest expectations without hiding the platform support.

---

## PRIORITY 7 — Document: Version 1 legacy file migration notice

### What the situation is
Collections created before Session 13 use Version "1" format with plain
username strings. The migration to Version "2" (with immutable IDs) happens
opportunistically — only triggered on the first write-capable load of the
collection. Read-only commands (`list`, public `show`, `clone`) never
trigger migration.

This means a user who only ever clones from collections and never runs
an owner-level command will have Version "1" files indefinitely. The
`list` command's format-aware role detection handles this correctly, but
users deserve to know it exists.

### What to add
Add a short "Upgrading from an earlier version" section to README.md:

```
### Upgrading from an earlier version

Collections created with gitcollect before July 2026 use an older format
that stores usernames instead of immutable platform user IDs. These files
are automatically upgraded the next time you run any command that modifies
the collection (member add, group add, repo access, etc.).

To force-upgrade a specific collection immediately:
  gitcollect member list <collection>    # triggers upgrade if you are the owner

You do not need to do anything if the tool is working correctly — upgrades
happen transparently. This notice exists so you know what to expect if you
see a one-line "Migrated collection to v2" message in the output.
```

Also add `gitcollect member list` to the opportunistic migration trigger
list in PROMPT.md's architecture section, since it is a reasonable command
for an owner to run that should also trigger upgrade.

---

## PRIORITY 8 — Mark activity as experimental

### What the situation is
`gitcollect activity` was added in Session 7, outside the original spec.
It is a significant addition — a new `internal/activity` package, a new log
file, new API methods (`ListCommits`, `DefaultBranch`), and a persistent
activity cache. It works, it has 84.4% test coverage, and it is genuinely
useful.

However, the feature is not mentioned in PROMPT_v2.md's spec at all, which
means it was built without the same design rigour applied to every other
command. There may be edge cases (empty repos, repos with no default branch,
very large commit histories) that have not been exercised.

### What to do
Do not remove it. It is working and tested. But:

1. Add a visible `[experimental]` badge to the `activity` command's help text:

```go
Long: `[EXPERIMENTAL] Show recent git commit activity across all accessible
repos in a collection. This command fetches recent commits from the platform
API and caches results locally at ~/.gitcollect/activity/<collection>.log.`,
```

2. Add to README.md in the command reference for `activity`:

```
> **Note:** This command is marked experimental. The output format and
> flag names may change in future versions.
```

3. Add to the roadmap section:

```
- Stabilise `gitcollect activity` (remove experimental flag in v1.1
  after real-world usage confirms the design)
```

---

## What NOT to do in this session

These were considered and explicitly ruled out — do not implement them:

- `gitcollect fetch` — collection sharing via URL. Good idea, out of scope
  for this session. Document it as coming, do not build it.
- `gitcollect update` — self-upgrade command. Out of scope. Document the
  manual upgrade path instead.
- Winget or Scoop distribution. Out of scope until there are real Windows
  users asking for it.
- Increasing `internal/` package coverage above current levels. They are
  already above 80%. Time is better spent on `cmd`.
- Rewriting any existing command behaviour. All existing commands work.
  This session is additions, removals, and documentation only — with the
  exception of the namespace fix (Priority 3) which changes behaviour
  but adds no new commands.

---

## Session discipline

Same rules as every other session:

- Read this file completely before touching any code
- Work in priority order — do not skip to easier items
- `go build ./...` and `go test ./...` after every item
- One file at a time within each priority item
- Flag any deviation from this plan before implementing it — do not
  silently improvise
- Update PROMPT.md's progress tracker before ending the session:
  - Mark completed items
  - Add a session log entry with today's date
  - Write "Next session should start with: <item>" at the bottom

---

## Delivery checklist

Mark each item done only when `go test ./...` passes with it included:

```
PRIORITY 1 — cmd coverage
  [ ] cmd/root_test.go          — loadForOwner, loadForRead, loadForGit,
                                   currentUser, currentUserID, migrateIfNeeded
  [ ] cmd/clone_test.go         — --pick parsing, --dry-run, access filter,
                                   pending-invite warning
  [ ] cmd/member_test.go        — single add, batch add, --confirm-self,
                                   non-member group add error
  [ ] cmd/show_test.go          — member view vs owner view selection
  [ ] cmd/list_test.go          — v1 vs v2 format-aware role detection
  [ ] cmd/auth_test.go          — login + ID both cached after auth

PRIORITY 2 — remove grant/revoke
  [ ] cmd/repo.go               — runRepoGrant and runRepoRevoke removed
  [ ] internal/collection/      — ErrRepoOpen, ErrRepoWouldOpen, GrantRepoUser,
       mutation.go                 RevokeRepoUser removed
  [ ] grep confirms zero        — grep -rn "ErrRepoOpen\|ErrRepoWouldOpen\|
       remaining references          GrantRepoUser\|RevokeRepoUser" . returns nothing
  [ ] cmd/repo.go help text     — updated to point users to repo access --users

PRIORITY 3 — namespace fix
  [ ] internal/collection/      — Namespace field + RepoNamespace() helper
       collection.go
  [ ] internal/collection/      — all API calls use RepoNamespace()
       mutation.go
  [ ] internal/access/          — all API calls use RepoNamespace()
       enforce.go + sync.go
  [ ] cmd/init.go               — --namespace flag added
  [ ] cmd/show.go               — displays namespace when set
  [ ] Tests (4 new tests)       — listed in Priority 3 section above
  [ ] PROMPT.md                 — YAML example updated with namespace field

PRIORITY 4 — collection sharing docs
  [ ] README.md                 — "Sharing collections" section added

PRIORITY 5 — error messages
  [ ] cmd/audit.go              — --since error lists valid values
  [ ] cmd/activity.go           — --since error lists valid values
  [ ] cmd/clone.go              — --pick help text shows quoted example
  [ ] Tests for both            — error message content verified

PRIORITY 6 — Windows docs
  [ ] README.md                 — Windows install steps added

PRIORITY 7 — legacy migration notice
  [ ] README.md                 — "Upgrading from earlier version" section
  [ ] PROMPT.md                 — migration trigger list updated

PRIORITY 8 — activity experimental
  [ ] cmd/activity.go           — [EXPERIMENTAL] in Long description
  [ ] README.md                 — experimental note + roadmap entry

FINAL CHECK
  [ ] go build ./...            — clean
  [ ] go test ./...             — all packages green
  [ ] go vet ./...              — clean
  [ ] cmd coverage              — above 50% (run: go test -cover ./cmd/...)
  [ ] PROMPT.md tracker         — updated with session log entry
```
