# gitcollect — Master Build Prompt v3.0

> Paste this entire file into Claude (or any capable LLM) to generate the
> complete gitcollect codebase. Every section is intentional — do not skip,
> summarise, or reorder any part. Read the PROGRESS TRACKER at the bottom
> before writing a single line of code, and update it before ending the session.

---

## Role & mindset

You are a senior Go engineer with 8+ years shipping production CLI tools used
by thousands of developers. You prioritise **correctness over cleverness**,
**explicit over implicit**, and **user clarity over code brevity**. You think
about failure modes before happy paths.

**The most important principle in this codebase:**
gitcollect's local YAML is a *declaration of intent* — it describes who should
have access. The GitHub/GitLab platform is the *enforcement point* — it is
where access is actually granted or revoked. These two must never diverge.
Every access mutation must drive the platform API to completion before the
local YAML is written. If the API call fails, the YAML does not change.
There is no in-between state. No shadow permission system.

---

## What this tool is

**gitcollect** is a standalone Go CLI that lets developers group
GitHub/GitLab repositories into named **collections** and control who can
access them — at both the collection level (who is a member) and the
repo level (which groups or individuals can reach which repos).

It does not replace Git. It wraps Git and the GitHub/GitLab APIs to add the
grouping and access-control layer that neither platform provides natively.
When gitcollect grants or revokes access, it does so by calling the real
GitHub/GitLab collaborator APIs — not by maintaining a shadow list.

---

## Complete command surface

```
── Authentication ──────────────────────────────────────────────────────────
gitcollect auth                               Store GitHub token (hidden prompt)
gitcollect auth --host gitlab.com            Authenticate a GitLab instance
gitcollect whoami                            Show authenticated user + token scope

── Collection lifecycle ────────────────────────────────────────────────────
gitcollect init <name>                        Create collection (default: private)
gitcollect delete <collection>               Delete collection + revoke all access
gitcollect list                              List your collections (owned + member)
gitcollect list --all                        Include private if owner
gitcollect show <collection>                 Summary: repos, members, groups
gitcollect visibility <collection> public|private   Change visibility

── Repo management ─────────────────────────────────────────────────────────
gitcollect add <collection> <repo>           Add repo (default: all members)
gitcollect remove <collection> <repo>        Remove repo + revoke access on platform
gitcollect repo access <collection> <repo> --groups g1,g2   Restrict to groups
gitcollect repo access <collection> <repo> --users u1,u2    Restrict to individuals
gitcollect repo access <collection> <repo> --open           Open to all members
gitcollect repo show <collection> <repo>    Show who can access this repo and why

── Member management ───────────────────────────────────────────────────────
gitcollect member add <collection> <username>      Add member to collection
gitcollect member remove <collection> <username>   Remove member + revoke all access
gitcollect member list <collection>               List members + their group memberships

── Group management ────────────────────────────────────────────────────────
gitcollect group create <collection> <group>       Create a group
gitcollect group delete <collection> <group>       Delete group (blocked if repos use it)
gitcollect group add <collection> <group> <user>   Add member to group
gitcollect group remove <collection> <group> <user> Remove member from group
gitcollect group list <collection>                List groups + members
gitcollect group show <collection> <group>        Show group members + accessible repos

── Access inspection ───────────────────────────────────────────────────────
gitcollect inspect <collection> --user <username>  Show full access map for a user
gitcollect inspect <collection> --repo <repo>      Show who can access a repo and why
gitcollect inspect <collection>                    Show full collection access matrix

── Audit trail ─────────────────────────────────────────────────────────────
gitcollect audit <collection>                 Show access change log
gitcollect audit <collection> --user <u>      Filter log by user
gitcollect audit <collection> --since 7d      Filter log by time (7d, 30d, 90d)
gitcollect audit <collection> --json          Machine-readable output

── Git operations ──────────────────────────────────────────────────────────
gitcollect clone <collection>                 Clone all accessible repos
gitcollect clone <collection> --pick r1 r2   Clone selected repos only
gitcollect clone <collection> --dry-run       Preview without executing
gitcollect clone <collection> --concurrency 8 Override parallel limit (default: 4)
gitcollect clone <collection> --dest <dir>   Clone into specific directory
gitcollect pull <collection>                 git pull inside all cloned repos
gitcollect status <collection>               git status inside all repos

── System ──────────────────────────────────────────────────────────────────
gitcollect version                           Print version + platform
gitcollect completion bash|zsh|fish          Generate shell completion script
```

---

## Project structure — generate every file listed

```
gitcollect/
├── main.go
├── cmd/
│   ├── root.go
│   ├── auth.go
│   ├── whoami.go
│   ├── init.go
│   ├── delete.go
│   ├── list.go
│   ├── show.go
│   ├── visibility.go
│   ├── add.go
│   ├── remove.go
│   ├── repo.go              # repo access + repo show subcommands
│   ├── member.go            # member add / remove / list
│   ├── group.go             # group create / delete / add / remove / list / show
│   ├── inspect.go           # inspect --user / --repo / full matrix
│   ├── audit.go             # audit log viewer
│   ├── clone.go
│   ├── pull.go
│   ├── status.go
│   └── version.go
├── internal/
│   ├── collection/
│   │   ├── collection.go        # Collection struct, Load, Save, Validate
│   │   ├── access.go            # IsMember, CanAccessRepo, AccessibleRepos
│   │   ├── mutation.go          # AddMember, RemoveMember, AddToGroup, etc.
│   │   └── collection_test.go
│   ├── access/
│   │   ├── enforce.go           # CheckCollectionAccess, CheckRepoAccess
│   │   ├── sync.go              # SyncCollaborators — drives platform API
│   │   ├── inspect.go           # UserAccessMap, RepoAccessMap, FullMatrix
│   │   └── access_test.go
│   ├── audit/
│   │   ├── audit.go             # AuditLog, append entry, read+filter
│   │   └── audit_test.go
│   ├── git/
│   │   ├── git.go               # Clone, Pull, Status wrappers
│   │   └── git_test.go
│   ├── api/
│   │   ├── client.go            # Client interface + NewClient factory
│   │   ├── github.go            # GitHub implementation
│   │   ├── gitlab.go            # GitLab implementation
│   │   └── api_test.go
│   ├── config/
│   │   ├── config.go            # Token storage, host map, ~/.gitcollect/
│   │   └── config_test.go
│   └── output/
│       └── output.go            # Success, Error, Warn, Table, JSON, Confirm
├── .goreleaser.yaml
├── go.mod
├── go.sum
├── Makefile
└── PROGRESS.md
```

---

## Dependencies — exactly these, no others

```
github.com/spf13/cobra     v1.8.0+    CLI framework
gopkg.in/yaml.v3           v3.0.1+    YAML parsing
github.com/fatih/color     v1.16.0+   Terminal colours (respects NO_COLOR)
golang.org/x/term          latest     Secure token input (no echo)
```

Do NOT add: HTTP client libraries, config frameworks, logging frameworks,
test assertion libraries, ORMs. Use stdlib for all of these.

---

## Data model — complete YAML format

```yaml
# ~/.gitcollect/collections/cybersecurity.yaml
version: "1"
name: cybersecurity
description: "Penetration testing and security research tools"
host: github.com
owner: yourusername
visibility: private           # public | private
created_at: "2025-01-15T10:00:00Z"
updated_at: "2025-01-20T14:32:00Z"

# Members: everyone who belongs to this collection.
# On a private collection, non-members cannot discover it exists.
members:
  - alice
  - bob
  - charlie
  - diana

# Groups: named subsets of members.
# A member can belong to zero, one, or many groups.
groups:
  red-team:
    - alice
    - bob
  analysts:
    - charlie
    - alice
  ops:
    - diana

# Repos with access control per repo.
# Access to a repo can be:
#   - open to all members:     groups: []  users: []
#   - restricted to groups:    groups: [red-team]  users: []
#   - restricted to users:     groups: []  users: [alice, bob]
#   - restricted to both:      groups: [red-team]  users: [diana]
#     (union: anyone in red-team OR diana can access)
repos:
  - name: pen-test-tools
    groups: []
    users: []               # open to all members

  - name: vuln-scanner
    groups: [red-team]
    users: []               # only red-team members (alice, bob)

  - name: threat-reports
    groups: [analysts]
    users: [diana]          # analysts group OR diana individually

  - name: ops-runbooks
    groups: [ops]
    users: []               # only ops group (diana)

  - name: ctf-writeups
    groups: []
    users: []               # open to all members
```

---

## Go structs

```go
// internal/collection/collection.go

type Visibility string

const (
    VisibilityPublic  Visibility = "public"
    VisibilityPrivate Visibility = "private"
)

// RepoAccess defines who can access a single repo within the collection.
// Access is granted if:
//   - Groups and Users are both empty (open to all members), OR
//   - caller is in any listed Group, OR
//   - caller is in the Users list
// These are unioned — not intersected.
type RepoAccess struct {
    Name   string   `yaml:"name"`
    Groups []string `yaml:"groups"` // group names; empty = no group restriction
    Users  []string `yaml:"users"`  // individual usernames; empty = no user restriction
}

type Collection struct {
    Version     string              `yaml:"version"`
    Name        string              `yaml:"name"`
    Description string              `yaml:"description"`
    Host        string              `yaml:"host"`
    Owner       string              `yaml:"owner"`
    Visibility  Visibility          `yaml:"visibility"`
    Members     []string            `yaml:"members"`
    Groups      map[string][]string `yaml:"groups"`  // group name → []username
    Repos       []RepoAccess        `yaml:"repos"`
    CreatedAt   time.Time           `yaml:"created_at"`
    UpdatedAt   time.Time           `yaml:"updated_at"`

    path string // not serialised — absolute path on disk
}
```

---

## Access logic — internal/collection/access.go

All functions operate on the local manifest only. No API calls here.

```go
// IsMember returns true if the collection is public (implicit member)
// or username is in the Members list.
func (c *Collection) IsMember(username string) bool

// IsInGroup returns true if username belongs to the named group.
func (c *Collection) IsInGroup(username, group string) bool

// CanAccessRepo returns true if username can clone/pull the named repo.
//
// Decision table:
//   collection public                       → true
//   user not a member                       → false
//   repo.Groups=[] AND repo.Users=[]        → true  (open to all members)
//   user in any repo.Groups                 → true
//   user in repo.Users                      → true
//   none of the above                       → false
func (c *Collection) CanAccessRepo(username, repoName string) bool

// AccessibleRepos returns repos the username can access, preserving order.
func (c *Collection) AccessibleRepos(username string) []RepoAccess

// WhyCanAccess returns a human-readable reason string for access decisions.
// Used by inspect commands.
//   "open to all members"
//   "member of group red-team"
//   "individually granted"
//   "owner"
//   "no access — not a member"
//   "no access — group red-team required"
func (c *Collection) WhyCanAccess(username, repoName string) string
```

---

## Mutation layer — internal/collection/mutation.go

Every mutation here follows the same three-step pattern:
  1. Validate inputs
  2. Call platform API (via passed-in api.Client)
  3. Only if API succeeds: update local struct and save atomically

```go
// AddMember adds username to Members, then calls SyncCollaborators.
// No-op if already a member (idempotent).
func (c *Collection) AddMember(username string, client api.Client) error

// RemoveMember removes username from Members and all Groups,
// then calls the API to remove their collaborator access from all repos.
// Requires --confirm-self flag if removing the caller's own username.
func (c *Collection) RemoveMember(username string, client api.Client) error

// AddToGroup adds username to group. Username must already be a member.
// After success: calls SyncCollaborators to update platform access.
func (c *Collection) AddToGroup(username, group string, client api.Client) error

// RemoveFromGroup removes username from group.
// After success: calls SyncCollaborators to recalculate their repo access.
func (c *Collection) RemoveFromGroup(username, group string, client api.Client) error

// SetRepoAccess updates the Groups and Users for a repo.
// After success: calls SyncCollaborators so platform reflects new rules.
// Setting groups=[] and users=[] opens the repo to all members.
func (c *Collection) SetRepoAccess(repoName string, groups, users []string, client api.Client) error

// CreateGroup creates a new empty group. Fails if name already exists.
func (c *Collection) CreateGroup(group string) error

// DeleteGroup deletes a group. Fails if any repo references this group —
// caller must clear those repo restrictions first.
func (c *Collection) DeleteGroup(group string) error
```

---

## Access enforcement — internal/access/enforce.go

Bridges the local manifest and the platform API.

```go
// CheckCollectionAccess verifies the caller can use this collection.
// On private collections: same error for "not found" and "not a member"
// to prevent existence disclosure.
func CheckCollectionAccess(col *collection.Collection, caller string) error

// CheckRepoAccess verifies:
//   1. Caller is a collection member
//   2. Caller passes local CanAccessRepo check
//   3. Caller actually has collaborator access on the platform (API call)
// All three must pass. Fails with specific error on each.
func CheckRepoAccess(
    col *collection.Collection,
    repoName, caller string,
    client api.Client,
) error

// FilterAccessible returns only the repos accessible to caller,
// combining local rules (step 1-2) and platform verification (step 3).
func FilterAccessible(
    col *collection.Collection,
    caller string,
    client api.Client,
) ([]collection.RepoAccess, error)
```

---

## Platform sync — internal/access/sync.go

```go
// SyncCollaborators computes the correct GitHub/GitLab collaborator state
// for every (member, repo) pair in the collection and drives the API
// to match it. Runs all API calls concurrently (max 4 in parallel).
//
// For each pair:
//   col.CanAccessRepo(member, repo) == true  → AddCollaborator (permission: "pull")
//   col.CanAccessRepo(member, repo) == false → RemoveCollaborator
//
// Partial failures are collected and returned as a joined error.
// Successful pairs are applied even if others fail.
// The local YAML is NOT modified by this function.
func SyncCollaborators(col *collection.Collection, client api.Client) (added, removed int, err error)
```

---

## Inspect commands — internal/access/inspect.go

```go
// UserAccessMap returns the access detail for every repo for a given user.
type RepoAccessDetail struct {
    RepoName   string
    CanAccess  bool
    Reason     string   // from WhyCanAccess
}
func UserAccessMap(col *collection.Collection, username string) []RepoAccessDetail

// RepoAccessMap returns the access detail for every member for a given repo.
type MemberAccessDetail struct {
    Username  string
    CanAccess bool
    Reason    string
}
func RepoAccessMap(col *collection.Collection, repoName string) []MemberAccessDetail

// FullMatrix returns a grid of all members × all repos for display as a table.
type AccessMatrix struct {
    Members []string
    Repos   []string
    // Grid[i][j] = true if Members[i] can access Repos[j]
    Grid    [][]bool
    // Reasons[i][j] = why/why not
    Reasons [][]string
}
func FullMatrix(col *collection.Collection) AccessMatrix
```

---

## Audit trail — internal/audit/audit.go

Every access mutation is appended to `~/.gitcollect/audit/<collection>.log`
as a newline-delimited JSON file (one JSON object per line).

```go
type AuditEntry struct {
    Timestamp  time.Time `json:"timestamp"`
    Collection string    `json:"collection"`
    Actor      string    `json:"actor"`      // who ran the command
    Action     string    `json:"action"`     // "member.add" | "member.remove" |
                                             // "group.add" | "group.remove" |
                                             // "repo.access.set" | "visibility.change" |
                                             // "member.add_to_group" | "member.remove_from_group"
    Target     string    `json:"target"`     // username, group name, or repo name
    Detail     string    `json:"detail"`     // human-readable summary of change
    Result     string    `json:"result"`     // "ok" | "error: <message>"
}

// Append writes one entry to the audit log atomically.
func Append(entry AuditEntry) error

// Read returns all entries for the collection, newest first.
func Read(collection string) ([]AuditEntry, error)

// Filter applies optional filters: username, since duration.
func Filter(entries []AuditEntry, user string, since time.Duration) []AuditEntry
```

Every mutation command (member add/remove, group add/remove, repo access,
visibility change) must call `audit.Append` after completing. If the operation
fails, still log it with `result: "error: <message>"` so failures are auditable.

---

## API client interface — internal/api/client.go

```go
type Client interface {
    GetRepo(owner, repo string) (RepoInfo, error)
    GetAuthenticatedUser() (string, error)
    AddCollaborator(owner, repo, username, permission string) error
    RemoveCollaborator(owner, repo, username string) error
    CheckCollaborator(owner, repo, username string) (bool, error)
    Host() string
}

type RepoInfo struct {
    Name     string
    CloneURL string  // always HTTPS
    Private  bool
    Archived bool
}

func NewClient(host, token string) Client

var (
    ErrNotFound     = errors.New("repository not found")
    ErrUnauthorized = errors.New("invalid or missing token")
    ErrForbidden    = errors.New("insufficient permissions")
    ErrRateLimit    = errors.New("API rate limit exceeded")
)
```

---

## Command behaviour — key interactions

### gitcollect inspect

```
$ gitcollect inspect cybersecurity --user bob

User:        bob
Collection:  cybersecurity (private)
Member:      yes
Groups:      red-team

REPO              ACCESS   REASON
pen-test-tools    ✓ yes    open to all members
vuln-scanner      ✓ yes    member of group red-team
threat-reports    ✗ no     analysts group required (not a member)
ops-runbooks      ✗ no     ops group required (not a member)
ctf-writeups      ✓ yes    open to all members
```

```
$ gitcollect inspect cybersecurity --repo vuln-scanner

Repo:       vuln-scanner
Access:     groups: red-team

MEMBER    ACCESS   REASON
alice     ✓ yes    member of group red-team
bob       ✓ yes    member of group red-team
charlie   ✗ no     red-team group required
diana     ✗ no     red-team group required
```

```
$ gitcollect inspect cybersecurity

Collection:  cybersecurity
Visibility:  private
Members:     4

         pen-test  vuln-scan  threat  ops-run  ctf
alice    ✓         ✓          ✓       ✗        ✓
bob      ✓         ✓          ✗       ✗        ✓
charlie  ✓         ✗          ✓       ✗        ✓
diana    ✓         ✗          ✓       ✓        ✓
```

### gitcollect audit

```
$ gitcollect audit cybersecurity --since 7d

2025-01-20 14:32  alice  member.add           bob         Added bob as member
2025-01-20 14:35  alice  member.add_to_group  bob → red-team  Added bob to red-team
2025-01-19 09:10  alice  repo.access.set      vuln-scanner  Restricted to groups: red-team
2025-01-18 16:00  alice  visibility.change    private→private  No change
2025-01-15 10:00  alice  init                 cybersecurity  Collection created (private)
```

### gitcollect member add — output

```
$ gitcollect member add cybersecurity diana

✓ Added diana to cybersecurity
  Syncing platform access...
  ✓ Granted pull access: pen-test-tools, ctf-writeups (2 open repos)
  ✗ Skipped: vuln-scanner (red-team group required)
  ✗ Skipped: ops-runbooks (ops group required)
  ✗ Skipped: threat-reports (analysts group required)

  To grant diana access to a restricted repo, add her to the relevant group:
  gitcollect group add cybersecurity ops diana
```

### gitcollect repo access — output

```
$ gitcollect repo access cybersecurity vuln-scanner --groups red-team

Updating access for vuln-scanner...
  Before: open to all members (4 people)
  After:  restricted to group red-team (2 people: alice, bob)

  Revoking access for 2 members: charlie, diana
  Syncing platform...
  ✓ Revoked charlie
  ✓ Revoked diana

✓ Access updated. Run: gitcollect inspect cybersecurity --repo vuln-scanner
```

### gitcollect group add — guided error

```
$ gitcollect group add cybersecurity red-team diana

✗ group add: "diana" is not a member of cybersecurity
  Add them first: gitcollect member add cybersecurity diana
```

### Clone — access-aware output

```
$ gitcollect clone cybersecurity

✓ Access verified (bob · groups: red-team)
  3 of 5 repos accessible

[1/3] Cloning pen-test-tools...   ✓ done  (1.2s)
[2/3] Cloning vuln-scanner...     ✓ done  (0.8s)
[3/3] Cloning ctf-writeups...     ✓ done  (2.1s)

✓ Cloned 3 repos in 2.4s
  2 repos skipped (no access): threat-reports, ops-runbooks
  Run: gitcollect inspect cybersecurity --user bob  to see why
```

### Private collection non-disclosure

```
$ gitcollect show secret-collection   # run by non-member

✗ show: collection "secret-collection" not found or access denied
```
Same error whether the collection exists or not.
Never confirm a private collection's existence to non-members.

---

## Non-negotiable engineering constraints

### 1. Efficiency

- Concurrent clone/pull with semaphore — default 4, `--concurrency` to override.
- `SyncCollaborators` runs API calls concurrently (max 4 in parallel).
- Cache `GetAuthenticatedUser()` per command invocation in a package var. One call only.
- Zero network calls on startup. `gitcollect list` reads local YAML only.
- `strings.Builder` not `+` in loops. One `http.Client` per command invocation.
- Audit log append is fire-and-forget (non-blocking) but errors are logged to stderr.

### 2. Security

- **Token**: stored at `~/.gitcollect/config` (0600). Echo disabled on input.
  Never in logs, errors, or flags. Truncated in debug: `ghp_xxxx...`
- **API-first**: local YAML is updated only AFTER the platform API call succeeds.
  If SyncCollaborators fails, the local YAML is not written.
- **Non-disclosure**: private collections return identical errors for "not found"
  and "access denied" — never confirm existence to non-members.
- **Double-check on clone**: before cloning any repo, verify platform collaborator
  status via API. Local manifest alone is not sufficient.
- **Validation regexes** (compile once, reuse):
  - Collection name: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`
  - Repo name:       `^[a-zA-Z0-9._-]{1,100}$`
  - Username:        `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`
  - Group name:      `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,30}$`
  - Reject inputs containing `../`, `/`, `\`, null bytes.
- **HTTPS only**: use `clone_url` from API always. Reject SSH patterns.
- **File permissions**: all files under `~/.gitcollect/`: 0600. Directories: 0700.
- **Destructive confirmations**: delete, remove, visibility→public, self-removal
  require explicit confirmation prompt before executing.
- **Self-removal guard**: `member remove` on yourself requires `--confirm-self` flag.

### 3. Reliability

- **Atomic YAML writes**: temp file + `os.Rename`. Never write directly to target.
- **API-then-YAML order**: for every mutation — API succeeds first, then write YAML.
  If API fails, YAML is unchanged and the error is returned.
- **Partial clone recovery**: all repos attempted; failures collected and reported at end.
- **SyncCollaborators partial failure**: apply successful pairs, collect errors,
  return joined error. Caller decides whether to surface or continue.
- **git prerequisite**: `exec.LookPath("git")` before any git command.
- **API timeout**: `context.WithTimeout` of 15s on all HTTP requests.
- **Idempotent mutations**: AddMember, AddToGroup, SetRepoAccess safe to run twice.
- **Audit on failure**: log failed operations too. Auditability requires seeing what was attempted.

### 4. Consistency

- Error format: `<command>: <what happened>: <why>` — never raw Go error strings.
- stdout = data and success. stderr = errors, warnings, progress, prompts.
- Exit codes: 0 = success · 1 = operational error · 2 = usage/argument error.
- Colours: green = success · red = error · yellow = warning · cyan = info.
  Respect `NO_COLOR`. Auto-disable when not a TTY.
- Flag names: kebab-case always.
- Timestamps: stored RFC3339 UTC · displayed in local time.
- After every mutation: print what changed, what was skipped, and a follow-up
  suggestion command the user can run next.

### 5. Ease of use

- Not authenticated: `Not authenticated. Run: gitcollect auth`
- Collection not found: `Collection "x" not found. Run: gitcollect list`
- Typo in repo name: suggest closest match if Levenshtein ≤ 2
- Group add of non-member: suggest `gitcollect member add` first
- Group delete blocked by repo: list which repos block it
- Every access denial includes the reason and the command to fix it
- `--dry-run` on clone and pull: show exactly what would happen
- `--json` on list, show, inspect, audit: machine-readable output
- Shell completion for bash, zsh, fish via Cobra built-ins

---

## Output package — internal/output/output.go

```go
func Success(format string, args ...any)           // green ✓  → stdout
func Error(format string, args ...any)             // red ✗    → stderr
func Warn(format string, args ...any)              // yellow ⚠ → stderr
func Info(format string, args ...any)              // cyan →   → stderr
func Dim(format string, args ...any)               // muted    → stderr (skipped items)
func Progress(current, total int, label string)    // \r overwrite → stderr
func Table(headers []string, rows [][]string)      // aligned columns → stdout
func JSON(v any) error                             // marshal → stdout
func Confirm(prompt string) bool                   // "prompt [y/N]: " → bool
func ConfirmWord(prompt, word string) bool         // require typing exact word
func Suggestion(cmd string)                        // "Run: <cmd>" hint line → stderr
```

---

## Makefile

```makefile
.PHONY: build test lint clean install release

build:
	go build -ldflags="-s -w -X main.version=$$(git describe --tags --always)" -o bin/gitcollect .

test:
	go test ./... -race -cover -coverprofile=coverage.out

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ dist/ coverage.out

install:
	go install -ldflags="-s -w" .

release:
	goreleaser release --clean
```

---

## .goreleaser.yaml

```yaml
version: 2
project_name: gitcollect

builds:
  - env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude: ["^docs:", "^test:", "Merge pull request"]
```

---

## Testing requirements

Use stdlib `testing` only. No external assertion libraries.

| Package                       | Test coverage required                                              |
|-------------------------------|---------------------------------------------------------------------|
| `internal/collection`         | Load, Save, Validate, AddMember, RemoveMember, AddToGroup, RemoveFromGroup, SetRepoAccess |
| `internal/collection/access`  | Full decision table (see matrix below)                              |
| `internal/access`             | CheckCollectionAccess, CheckRepoAccess, FilterAccessible, SyncCollaborators using mock client |
| `internal/audit`              | Append, Read, Filter by user and duration                           |
| `internal/api`                | GitHub + GitLab against `httptest.Server` mocks                     |
| `internal/git`                | Correct args to git subprocess (mock exec)                          |
| `internal/config`             | File at 0600, directory at 0700                                     |
| `internal/output`             | Table alignment, JSON, Confirm true/false                           |

Minimum 80% coverage on all `internal/` packages.
Use `t.TempDir()` for all file tests. Never touch real `~/.gitcollect/`.

**Access control test matrix — cover every row:**

```
Scenario                                               Expected
─────────────────────────────────────────────────────  ─────────────────────
public collection + any caller                       → allowed
private + owner                                      → allowed
private + member + repo open (groups=[] users=[])    → allowed
private + member + repo groups=[G] + member in G     → allowed
private + member + repo users=[U] + caller == U      → allowed
private + member + repo groups=[G] + member in G
         AND users=[U] (union test)                  → allowed (group match)
private + member + repo groups=[G] users=[U]
         + caller not in G but caller == U           → allowed (user match)
private + member + repo groups=[G] + member NOT in G
         + member NOT in users                       → ErrGroupDenied
private + non-member                                 → ErrForbidden (non-disclosure)
private + non-member + any repo                      → ErrForbidden (non-disclosure)
```

---

## Scope boundary — do NOT build

- No GUI or TUI
- No web server or daemon
- No database (YAML + newline-delimited JSON audit log only)
- No SSH clone (HTTPS only)
- No Bitbucket (GitHub + GitLab only in v1)
- No automatic git installation
- No telemetry or analytics
- No email/webhook notifications
- No encryption of collection YAML (tokens at 0600; membership lists plaintext by design)
- No `gitcollect admin` or super-user mode — all access flows through the
  authenticated user's own GitHub/GitLab token

---

## PROGRESS TRACKER

> Read this section first at the start of every agent session.
> Do not write a single line of code before reading it.
> Update the file completion table and session log before ending the session.

---

### How to continue a session

At the start of every new session, say:

> "Read the PROGRESS TRACKER. Update the session log with today's date and
> your model name. Continue from where the previous session stopped.
> Mark each file [done] in the table as you complete and verify it compiles.
> Write 'Next session should start with: <filename>' before ending."

---

### Session log

```
Session 1 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
Completed:    main.go, go.mod, cmd/root.go, cmd/auth.go
In progress:  (none)
Blockers:     (none)
Next session: Start with cmd/whoami.go

Session 2 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
NOTE: this session's table below was stale on arrival — cmd/whoami.go
through cmd/repo.go were already done and compiling, despite being
marked "todo". Re-verified every file against `go build ./...` before
trusting the table; do this first in any future session too.
Completed:    cmd/member.go, cmd/group.go, cmd/inspect.go, cmd/audit.go,
              cmd/clone.go, cmd/pull.go, cmd/status.go, cmd/version.go,
              Makefile, .goreleaser.yaml, and every *_test.go file listed
              in the testing-requirements table (collection, access,
              audit, git, config, output, api) — all packages now at
              80%+ coverage (see `go test ./... -cover`).
              Also fixed a real bug found while smoke-testing: internal/
              output/output.go's Table used byte length instead of rune
              count for column width, misaligning every table containing
              ✓/✗ (i.e. most of inspect/member/group/repo output).
In progress:  (none)
Blockers:     (none)
Next session: Feature-complete per this spec. If resuming, run
              `go build ./... && go vet ./... && go test ./... -cover`
              first — everything should be green — then look at
              cmd/auth.go and cmd/list.go's public-collection-visibility
              edge case noted in the architecture decisions log below
              before changing anything in cmd/.
```

---

### File completion table

Update STATUS as each file is finished and verified compilable.
STATUS values: todo | in-progress | done | blocked (add reason)

```
FILE                                         STATUS       NOTES
───────────────────────────────────────────  ───────────  ─────────────────────
main.go                                      done
go.mod                                       done
Makefile                                     done
.goreleaser.yaml                             done

cmd/root.go                                  done         grew loadForGit (see decisions log)
cmd/auth.go                                  done
cmd/whoami.go                                done
cmd/init.go                                  done
cmd/delete.go                                done
cmd/list.go                                  done         see decisions log: public-collection edge case
cmd/show.go                                  done
cmd/visibility.go                            done
cmd/add.go                                   done
cmd/remove.go                                done
cmd/repo.go                                  done
cmd/member.go                                done
cmd/group.go                                 done
cmd/inspect.go                               done
cmd/audit.go                                 done
cmd/clone.go                                 done
cmd/pull.go                                  done
cmd/status.go                                done
cmd/version.go                               done
(shell completion: cobra's built-in `completion` subcommand covers this
 — no separate file needed; verified with `gitcollect completion --help`)

internal/collection/collection.go            done
internal/collection/access.go                done         groupsContaining (dead code) removed
internal/collection/mutation.go              done
internal/collection/collection_test.go       done         83.1% coverage

internal/access/enforce.go                   done
internal/access/sync.go                      done
internal/access/inspect.go                   done
internal/access/access_test.go               done         94.1% coverage

internal/audit/audit.go                      done
internal/audit/audit_test.go                 done         82.8% coverage

internal/git/git.go                          done
internal/git/git_test.go                     done         91.7% coverage

internal/api/client.go                       done
internal/api/github.go                       done         githubBaseURL: const → var (see decisions log)
internal/api/gitlab.go                       done
internal/api/api_test.go                     done         84.8% coverage

internal/config/config.go                    done
internal/config/config_test.go               done         82.8% coverage

internal/output/output.go                    done         Table/padRight: byte len → rune count (real bug fix)
internal/output/output_test.go               done         97.9% coverage
```

---

### Architecture decisions log

Record any design decisions made during implementation here so future
sessions do not re-debate them.

```
- Exit code classification (cmd/root.go): cobra's SilenceUsage/SilenceErrors
  are set on rootCmd only — cobra checks the root's flags even when a
  subcommand errors, so this suffices globally. To distinguish exit 2 (usage)
  from exit 1 (operational) without re-deriving it per command, root.go sets
  a package-level bool `ranPersistentPreRun` inside rootCmd.PersistentPreRunE.
  Any error returned before that runs (unknown command/flag, bad arg count)
  is necessarily a usage error. Errors returned after it (i.e. from a
  command's own RunE) are operational (exit 1) unless explicitly wrapped
  with cmd.NewUsageError(err) — use that wrapper for semantic input problems
  caught inside command logic (e.g. invalid collection/repo/username format)
  that cobra's own Args validators can't catch.
- rootCmd.Version is intentionally left unset. `gitcollect version` is its
  own subcommand (cmd/version.go, prints version + platform per spec) rather
  than relying on cobra's auto-generated `--version` flag, to avoid two
  divergent version outputs.
- cmd/auth.go fixes these signatures for not-yet-built packages — match them
  exactly when implementing those files:
    api.NewClient(host, token string) api.Client   (already in spec)
    api.Client.GetAuthenticatedUser() (string, error)   (already in spec)
    config.SaveToken(host, token string) error   — new; config.go must add
      this in addition to whatever Load/host-map functions it defines, since
      auth is the only command that writes a token.
  promptForToken in auth.go bypasses internal/output and writes the
  "Token for <host> (input hidden): " prompt straight to os.Stderr with
  fmt.Fprintf (no trailing newline), because the hidden input from
  term.ReadPassword needs to land on the same terminal line as the prompt.
  Treat internal/output's Info/Success/etc. as line-oriented (always end
  with a newline) when implementing output.go, and keep using raw
  fmt.Fprint(os.Stderr, ...) for any future same-line prompt instead of
  adding a no-newline variant to internal/output.
- internal/output/output.go implemented: fatih/color + golang.org/x/term
  (IsTerminal) decide colour; NO_COLOR env var or either stream not being a
  TTY disables colour globally via color.NoColor. Table does simple
  space-padded alignment (no truncation/wrapping). Confirm/ConfirmWord read
  one line via a fresh bufio.Reader(os.Stdin) per call — fine since no
  command issues more than one prompt per invocation.
- internal/config/config.go implemented: tokens stored as YAML
  (map[string]string, host -> token) at ~/.gitcollect/config, atomic
  temp-file+rename write, chmod 0600 explicitly (belt-and-suspenders since
  os.CreateTemp already defaults to 0600 on POSIX). Also owns
  CollectionsDir()/AuditDir()/EnsureDir() as the shared source of truth for
  paths under ~/.gitcollect, since collection.go and audit.go both need them
  and config.go is the package documented to own "~/.gitcollect/" layout.
- internal/api: NewClient dispatches purely on host string — "github.com"
  exactly gets the GitHub implementation (api.github.com), anything else
  (gitlab.com or a self-hosted GitLab domain) gets the GitLab v4 API at
  https://<host>/api/v4. No GitHub Enterprise support — out of v1 scope.
  GitLab's "pull" permission maps to access_level 20 (Reporter), not 10
  (Guest), because Guest cannot read repository code on most GitLab
  versions and gitcollect's "pull" always means read access. GitLab's
  member endpoints need a numeric user_id, so AddCollaborator/
  RemoveCollaborator/CheckCollaborator first resolve username via
  GET /users?username=. AddCollaborator on an existing member (409) falls
  back to updating their access_level via PUT instead of failing.
- IMPORTANT — avoided a package import cycle: the spec's internal/access
  package (enforce.go/sync.go/inspect.go) necessarily imports
  internal/collection (its functions take *collection.Collection). That
  means internal/collection cannot import internal/access back, even
  though the spec's prose for mutation.go says each mutation "calls
  SyncCollaborators". Resolution: the real SyncCollaborators
  implementation now lives as a method, (c *Collection) SyncCollaborators
  (client api.Client) (added, removed int, err error), directly in
  internal/collection/mutation.go — it only needs api.Client, not the
  access package. internal/access/sync.go's exported SyncCollaborators
  function (still required by the spec's public API surface, used by
  cmd/clone.go etc.) is a thin wrapper: `return col.SyncCollaborators
  (client)`. Build access/sync.go this way — do not duplicate the
  concurrency logic there.
  Relatedly, RemoveMember can't rely on SyncCollaborators' normal
  member×repo loop to revoke access, because once username is dropped from
  c.Members it no longer appears in that iteration. mutation.go therefore
  has a separate unexported helper, revokeAllAccess(username, client),
  that unconditionally calls RemoveCollaborator across every repo before
  the member is actually removed from the manifest.
- cmd/root.go grew shared cross-command infra beyond the original
  Execute/UsageError pair (still additive, never rewritten): cachedClient/
  cachedUser + currentClient(host)/currentUser(client) for the "call
  GetAuthenticatedUser once per invocation" rule; loadCollection(name) for
  owner-perspective commands (friendly "not found. Run: gitcollect list");
  loadForRead(name) (col, caller, err) for read/discovery commands (show,
  inspect, clone, pull, status), which collapses "manifest missing" and
  "exists but caller denied" into the identical access.ErrForbidden so a
  private collection's existence is never disclosed. Use loadCollection for
  mutation commands that only make sense for an authorized owner/member,
  loadForRead for anything a non-member could probe to test existence.
  loadForRead only resolves caller identity (currentClient+currentUser) if
  the collection turns out to be private — public collections return with
  caller == "" since no authentication is needed to read them at all. Don't
  resolve caller before calling this; that was an earlier, wrong revision
  of this helper (loadAccessibleCollection) that forced an API call even
  for public collections — fixed during cmd/show.go.
- DEVIATED from the spec's "Audit log append is fire-and-forget
  (non-blocking)": recordAudit (cmd/root.go) calls audit.Append
  synchronously instead of from a goroutine. Reason: main.go does
  os.Exit(cmd.Execute()) immediately on return, which would kill a
  detached goroutine mid-write before the entry was durably appended —
  directly undermining "Audit on failure: log failed operations too."
  A local append of a few hundred bytes is not slow enough to justify the
  risk. Append failures are still non-fatal (logged via output.Warn, never
  returned as the command's error).
- internal/config/config.go grew a second cache (additive, not rewritten):
  Config.Users map[string]string (host -> username), with SaveUser/
  LoadUser alongside SaveToken/LoadToken. Reason: "list reads local YAML
  only, zero network calls" is impossible to reconcile with "list shows
  *your* collections (owned + member)" unless "who am I" is resolvable
  without calling GetAuthenticatedUser. auth.go now calls SaveUser right
  after it validates a token (it already has the username for free from
  that call); cmd/root.go's currentUser() also refreshes this cache
  whenever any other command does end up calling GetAuthenticatedUser, so
  the cache only ever goes stale, never wrong-direction.
- cmd/list.go semantics: a collection is "yours" if the cached username for
  its Host equals col.Owner ("owner") or appears in col.Members ("member").
  Public collections and any membership are shown by default; --all
  additionally surfaces private collections you own but haven't added
  yourself to as a member (the literal reading of "--all: include private
  if owner"). Collections on a host you've never authenticated to (no
  cached username) are skipped with a warning rather than erroring the
  whole command — one bad/foreign manifest shouldn't break `list`.
- Mutation save semantics: every mutator only calls c.Save() if
  SyncCollaborators/revokeAllAccess returned err == nil, restoring the
  pre-mutation in-memory state on failure. The platform may have partially
  applied some pairs even on a failed sync (those successes are kept on the
  platform per spec), but local YAML only commits when the *overall* sync
  reported no error — this is what keeps YAML from ever claiming a state
  that wasn't fully confirmed.
- cmd/root.go grew a third loader, loadForGit(name) (col, caller, client,
  err), used only by clone.go/pull.go/status.go. Unlike loadForRead,
  loadForGit ALWAYS resolves an authenticated client and caller, even for
  public collections, because these three commands inherently need a real
  api.Client to fetch clone URLs (GetRepo) and verify platform collaborator
  status (CheckCollaborator via access.FilterAccessible) — there is no
  client-free path for that the way there is for purely-local commands like
  show/inspect. Don't try to make clone/pull/status reuse loadForRead's
  public-collection auth-skip optimization; it doesn't apply to them.
- cmd/member.go, cmd/group.go, cmd/inspect.go share an unexported
  groupsForMember(col, username) []string helper (defined once, in
  member.go, package-private to cmd) for "which groups is this user in."
  internal/collection/access.go used to have an equivalent groupsContaining
  method but it was dead code (never called from anywhere, including its
  own package) — deleted rather than kept as an unused duplicate. If a
  future session wants to export this from collection instead of
  duplicating it in cmd, that's a reasonable cleanup, but don't reintroduce
  the unexported version in access.go.
- internal/output/output.go's Table/padRight measured column width with
  len() (byte length) instead of utf8.RuneCountInString. Every multi-byte
  glyph used throughout this codebase's output made the affected column's
  padding too short by (byte_len - rune_count), visibly misaligning tables
  — this was already present in cmd/repo.go's `repo show` before this
  session and just got more visible once member/group/inspect started
  using the same checkmark pattern. Fixed Table and padRight to use
  utf8.RuneCountInString throughout. If you add new output helpers that pad
  strings, use rune count, not len(), whenever the string might contain
  non-ASCII output glyphs.
- internal/api/github.go's githubBaseURL was a const. Changed to a
  package-level var (same default value, "https://api.github.com") solely
  so api_test.go can redirect it at an httptest.Server — gitlabClient
  already had an equivalent per-instance baseURL struct field for the same
  reason; githubClient just never needed one until the spec's own testing
  requirement ("GitHub + GitLab against httptest.Server mocks") made it
  necessary. No production behaviour changes.
- cmd/version.go does NOT implement `gitcollect completion bash|zsh|fish`
  itself — cobra's rootCmd automatically gets a built-in `completion`
  subcommand (bash/zsh/fish/powershell) for free once the command tree has
  subcommands, with no extra code needed. Verified via
  `gitcollect completion --help`. Don't add a hand-rolled completion.go;
  there's nothing for it to do that cobra doesn't already provide.
- cmd/audit.go's --since flag accepts gitcollect's documented day-shorthand
  (7d/30d/90d) IN ADDITION to anything time.ParseDuration understands
  (24h, 30m, etc.) via a small parseSince() that special-cases a trailing
  "d" before falling back to time.ParseDuration. The spec's own example
  values (7d, 30d, 90d) are not valid time.ParseDuration input on their own.
- KNOWN EDGE CASE, not fixed this session — cmd/list.go: a public
  collection you are NOT a member of (and don't own) is silently skipped by
  `gitcollect list`, because the role-selection switch's default case
  continues past it whenever the cached username for that host is "" or
  doesn't match owner/member. list.go's own help text says "Public
  collections ... are shown by default," which overclaims relative to what
  the code does — in practice "list" only ever shows collections you own or
  are an explicit member of, public or not. Inherited from session 1,
  confirmed via manual testing this session (a public test collection
  didn't appear in `list` without a cached username for its host), and left
  alone since list.go was already marked "done" — reconciling the help
  text vs. changing actual semantics is a product decision the next
  session should make deliberately, not a drive-by fix.
- Manual end-to-end smoke testing this session required pointing
  ~/.gitcollect at a temp dir. On a Windows dev box, Go's
  os.UserHomeDir() reads %USERPROFILE%, NOT $HOME — `export HOME=...` in
  Git Bash has no effect on the compiled binary. Set USERPROFILE (and,
  for portability, HOME too) when manually testing on Windows. The test
  suites already do this correctly via t.Setenv("HOME", ...) +
  t.Setenv("USERPROFILE", ...) in every package's test helpers.
- internal/git/git_test.go mocks the real `git` subprocess (per the
  testing-requirements table: "Correct args to git subprocess (mock
  exec)") by writing a fake git.bat to a t.TempDir() and pointing PATH at
  only that directory. Confirmed empirically that Go's exec.Command on
  Windows resolves and runs .bat files placed on PATH transparently (no
  cmd.exe wrapping needed in the test itself). This requires t.Setenv, so
  these tests cannot run with t.Parallel().
```
