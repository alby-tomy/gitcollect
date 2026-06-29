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
gitcollect list                              List your collections (owned + member), any visibility
gitcollect list --private                    Filter to private collections only
gitcollect list --public                     Filter to public collections only
  (REVISED session 4, on user request — original spec's "list --all" is gone;
   see decisions log. "list" with no flags now shows everything --all used to.)
gitcollect show <collection>                 Summary: repos, members, groups, plus a
                                              per-repo YOU column (caller's own
                                              ✓/✗ access + reason) — added session 9,
                                              see decisions log
gitcollect visibility <collection> public|private   Change visibility

── Repo management ─────────────────────────────────────────────────────────
gitcollect add <collection> <repo>           Add repo (default: all members)
gitcollect remove <collection> <repo>        Remove repo + revoke access on platform
                                              (requires typing the repo name to confirm,
                                               same as `delete` — changed session 8, was
                                               a plain y/N prompt before; see decisions log)
gitcollect repo access <collection> <repo> --groups g1,g2   Restrict to groups
gitcollect repo access <collection> <repo> --users u1,u2    Restrict to individuals
gitcollect repo access <collection> <repo> --open           Open to all members
gitcollect repo show <collection> <repo>    Show who can access this repo and why
gitcollect repo grant <collection> <repo> <user>   Grant one user individual access (added in session 3; not in the original spec — see decisions log)
gitcollect repo revoke <collection> <repo> <user>  Revoke one user's individually granted access (same)

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

── Code activity (added session 7; not in the original spec — see decisions
   log) ────────────────────────────────────────────────────────────────────
gitcollect activity <collection>              Show commits across accessible
                                               repos' default branch, fetched
                                               live + recorded to
                                               ~/.gitcollect/activity/<name>.log
gitcollect activity <collection> --repo <r>   Limit to one repo
gitcollect activity <collection> --since 7d   Filter by commit time
gitcollect activity <collection> --limit <n>  Max commits fetched per repo this run (default 10)
gitcollect activity <collection> --json       Machine-readable output

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
│   ├── activity.go          # commit-activity viewer (added session 7; not in original spec)
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
│   ├── activity/
│   │   ├── activity.go          # commit-activity log, append entry, read+filter+dedup (added session 7)
│   │   └── activity_test.go
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
    ListCommits(owner, repo, branch string, limit int) ([]CommitInfo, error)  // added session 7
    Host() string
}

type RepoInfo struct {
    Name          string
    CloneURL      string  // always HTTPS
    DefaultBranch string  // added session 7, for "gitcollect activity"
    Private       bool
    Archived      bool
}

// CommitInfo added session 7. Author is the platform username on GitHub
// when resolvable, else the raw commit author name; GitLab's commits
// endpoint never resolves a platform username, so Author is always the
// raw author_name there.
type CommitInfo struct {
    SHA         string
    Author      string
    Message     string  // first line only
    CommittedAt time.Time
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

> Every block below is the *real* output of the current implementation
> (verified against cmd/*.go and internal/output/output.go as of session
> 10), not the illustrative sketches this section originally shipped with.
> If you change a print statement, update the matching block here — this
> section drifting from reality is exactly what session 10 had to fix.

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
threat-reports    ✗ no     no access — group analysts required
ops-runbooks      ✗ no     no access — group ops required
ctf-writeups      ✓ yes    open to all members
```

```
$ gitcollect inspect cybersecurity --repo vuln-scanner

Repo:       vuln-scanner
Access:     groups: red-team

MEMBER    ACCESS   REASON
alice     ✓ yes    member of group red-team
bob       ✓ yes    member of group red-team
charlie   ✗ no     no access — group red-team required
diana     ✗ no     no access — group red-team required
```

```
$ gitcollect inspect cybersecurity

Collection:  cybersecurity
Visibility:  private
Members:     4

MEMBER   pen-test-tools  vuln-scanner  threat-reports  ops-runbooks  ctf-writeups
alice    ✓               ✓             ✓               ✗             ✓
bob      ✓               ✓             ✗               ✗             ✓
charlie  ✓               ✗             ✓               ✗             ✓
diana    ✓               ✗             ✓               ✓             ✓
```

If the owner themselves runs any of the three forms above, every repo
reports `✓ yes` / reason `"owner"` even if the owner isn't separately
listed in `Members` — `internal/access/inspect.go`'s `decide()` helper
guarantees this (fixed session 9; before that fix this was a real,
reachable bug: `CanAccess=false` paired with `Reason="owner"`).

### gitcollect show — repo access, personal to the caller (session 9)

```
$ gitcollect show cybersecurity

Collection:  cybersecurity
Host:        github.com
Owner:       alice
Visibility:  private
Members:     4
Groups:      3
Repos:       5

MEMBER
alice
bob
charlie
diana

GROUP      MEMBERS
red-team   2
ops        1
analysts   1

REPO             ACCESS RULE          YOU
pen-test-tools   open to all members  ✓ yes
vuln-scanner     groups: [red-team]   ✓ yes
threat-reports   groups: [analysts]   ✗ no — no access — group analysts required
ops-runbooks     groups: [ops]        ✗ no — no access — group ops required
ctf-writeups     open to all members  ✓ yes

  You can't access 2 repo(s): threat-reports, ops-runbooks
Run: gitcollect inspect cybersecurity --user diana
```

(Run by `diana`, who's in `ops` but not `analysts` or `red-team`.) `ACCESS
RULE` is the configured rule; `YOU` is whoever ran the command — `clone`
only ever fetches the repos marked `✓` here, since both are backed by the
same `access.UserAccessMap`/`FilterAccessible` decision.

### gitcollect audit

```
$ gitcollect audit cybersecurity --since 7d

2026-01-20 14:32  alice       member.add            bob             Added member
2026-01-20 14:35  alice       member.add_to_group   bob → red-team  Added bob to red-team
2026-01-19 09:10  alice       repo.access.set       vuln-scanner    open to all members → groups: red-team
2026-01-15 10:00  alice       init                  cybersecurity   Collection created (private)
```

Real format string (`cmd/audit.go`): `"%s  %-10s  %-20s  %-20s  %s%s\n"` —
timestamp, actor, action, target, detail, then `"  [<result>]"` appended
only when a row's `Result` isn't `"ok"` (failures are logged too, never
silently dropped).

### gitcollect member add — output

```
$ gitcollect member add cybersecurity diana

✓ Added diana to cybersecurity

  Granted access: pen-test-tools, ctf-writeups
  Skipped: vuln-scanner (no access — group red-team required)
  Skipped: ops-runbooks (no access — group ops required)
  Skipped: threat-reports (no access — group analysts required)
Run: gitcollect group add cybersecurity <group> diana
```

(`cmd/member.go`'s `printAccessBreakdown`. The "Granted"/"Skipped" lines
are dim/secondary text; the suggestion only appears if at least one repo
was skipped.)

### gitcollect repo access — output

```
$ gitcollect repo access cybersecurity vuln-scanner --groups red-team

✓ Updated access for vuln-scanner
  Before: open to all members
  After:  groups: red-team
Run: gitcollect inspect cybersecurity --repo vuln-scanner
```

No live "Revoking access for N members..." progress lines — `SetRepoAccess`
syncs collaborators synchronously before this prints, so by the time
"Updated access" appears the platform is already consistent with it.

### gitcollect repo grant / revoke — one user, individually (session 3)

```
$ gitcollect repo grant cybersecurity vuln-scanner eve

✓ Granted eve individual access to vuln-scanner
  Before: groups: red-team
  After:  groups: red-team, users: eve
Run: gitcollect inspect cybersecurity --repo vuln-scanner
```

```
$ gitcollect repo grant cybersecurity ctf-writeups eve   # ctf-writeups is open to all

✗ repo grant: repo is open to all members; granting one user individually would revoke everyone else
Run: gitcollect repo access cybersecurity ctf-writeups --users eve
```

`repo revoke` has the mirror-image guardrail: it refuses if removing the
last individually-granted user would silently re-open the repo to every
member, with the same kind of guided suggestion. Both are no-ops (not
errors) if the user already has, or already lacks, that individual grant.

### gitcollect group add — guided error

```
$ gitcollect group add cybersecurity red-team diana

✗ group add: "diana" is not a member of cybersecurity
Run: gitcollect member add cybersecurity diana
```

### gitcollect list — visibility filters (redesigned session 4)

```
$ gitcollect list

NAME           VISIBILITY  ROLE    MEMBERS  REPOS
cybersecurity  private     owner   4        5
ctf-public     public      member  12       3

$ gitcollect list --private

NAME           VISIBILITY  ROLE    MEMBERS  REPOS
cybersecurity  private     owner   4        5
```

Reads local YAML only — zero network calls, works fully offline. `--private`
and `--public` are mutually exclusive filters on top of the same base list
(everything you own or are a member of); passing neither (or both) shows
everything.

### gitcollect remove — type-the-name confirmation (session 8)

```
$ gitcollect remove cybersecurity ops-runbooks

This will remove "ops-runbooks" from "cybersecurity" and revoke access for 4 member(s) (type "ops-runbooks" to confirm):
```

Matches `gitcollect delete <collection>`'s existing type-to-confirm pattern
exactly (`output.ConfirmWord`) — both used to differ (`delete` already
required the exact name; `remove` only asked `[y/N]` until session 8).

### Clone — access-aware output

```
$ gitcollect clone cybersecurity

✓ Access verified (bob · red-team)
  3 of 5 repos accessible

[1/3] Cloning pen-test-tools...               ✓ done  (1.2s)
[2/3] Cloning vuln-scanner...                 ✓ done  (0.8s)
[3/3] Cloning ctf-writeups...                 ✓ done  (2.1s)

✓ Cloned 3 repo(s) in 2.4s
  2 repo(s) skipped (no access): threat-reports, ops-runbooks
Run: gitcollect inspect cybersecurity --user bob
```

The "Access verified" line is `caller · <comma-joined groups, or "no
groups">` — no literal word "groups:" in it. For a *public* collection it's
instead `output.Success("Public collection — %d of %d repos accessible", …)`.

### gitcollect activity — code changes, not access changes (session 7)

```
$ gitcollect activity cybersecurity --since 7d

REPO            BRANCH  AUTHOR  SHA      MESSAGE          WHEN
vuln-scanner    main    alice   a1b2c3d  Fix false positive  2026-01-20 11:02
pen-test-tools  main    bob     9f8e7d6  Add new module       2026-01-19 16:40

✓ recorded 2 new commit(s) to the activity log
```

Fetches live from GitHub/GitLab (default branch only, `--limit` per repo,
default 10), records genuinely new commits to
`~/.gitcollect/activity/<collection>.log`, and displays the combined history
(this run's fetch plus everything previously recorded), newest first.
Unlike `audit` (which only ever sees mutations gitcollect itself performed),
this is the only place gitcollect reports on actual git commit history.

### Private collection non-disclosure

```
$ gitcollect show secret-collection   # run by a non-member

✗ show: collection not found or access denied
```

`access.ErrForbidden`'s message is generic and never echoes the collection
name back — same error whether the collection exists or not, so a private
collection's existence is never confirmed to a non-member.

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

Session 3 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
Added a feature beyond the original spec, on user request: per-user
grant/revoke for a single repo without rewriting that repo's whole
--users list. See decisions log for the footgun this had to guard
against (the empty-Groups-and-Users-means-"open to all members" rule).
Completed:    collection.GrantRepoUser/RevokeRepoUser (mutation.go),
              cmd/repo.go's `repo grant`/`repo revoke` subcommands,
              new ErrRepoOpen/ErrRepoWouldOpen sentinels, tests for all
              of the above in collection_test.go (idempotency, the two
              refusal cases, sync-failure rollback for each).
              Also fixed a real, always-on bug (not just a -race
              finding) hit while adding those tests: internal/
              collection/collection_test.go's mockClient wrote to a
              plain map from SyncCollaborators' concurrent goroutines
              with no lock — "fatal error: concurrent map writes",
              Go's runtime-level detector, not the optional race
              detector. Added a sync.Mutex to the three mockClient
              methods that touch the map. If you add a new test mock
              for api.Client anywhere, assume SyncCollaborators will
              call it concurrently and lock accordingly from the start.
              Also diagnosed and fixed the OTHER half of session 2's
              "KNOWN EDGE CASE" note below: `gitcollect list` with no
              flags printed a silent empty table for a private
              collection you own but aren't a member of (the exact
              scenario from this session's manual testing — init
              defaults to private, and init does NOT auto-add the
              owner as a member). That's cmd/list.go's real, documented
              behavior (`--all: include private if owner`), not a bug,
              but it gave zero feedback when it happened. Added a
              hiddenPrivateOwned counter; if the final table is empty
              AND collections were hidden specifically for that reason,
              print "no collections shown — you own N private
              collection(s) you haven't joined as a member" +
              `Run: gitcollect list --all`. The session 2 note's OTHER
              scenario (a public collection you're not a member of,
              with no cached username for its host) is still open —
              that's a different code path (the role-switch's default
              case, not the owner+private+!listAll branch) and wasn't
              touched.
In progress:  (none)
Blockers:     (none)
Next session: Feature-complete + these two extensions. Same startup
              check as session 2's note above.

Session 4 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User-requested redesign of cmd/list.go, replacing --all entirely
(not additive — see decisions log for why and for the bug this caught
in my own prior advice).
Completed:    cmd/list.go: removed --all; `list` with no flags now
              shows every collection you own or are a member of, any
              visibility (this is what --all used to do). Added
              --private/--public as visibility filters instead
              (passing both = passing neither = no filter). Updated
              PROMPT.md's command surface and README.md to match.
              Reverted the hiddenPrivateOwned hint message added
              session 3 — it's dead with the filter gone, nothing
              is hidden by default anymore.
In progress:  (none)
Blockers:     (none)
Next session: Feature-complete + all extensions through this session.
              Same startup check as session 2's note. cmd/ has no
              automated tests per the testing-requirements table —
              changes to cmd/*.go are only verified by manual
              smoke-testing (hand-built ~/.gitcollect fixtures) in
              this transcript, not by a test suite. Consider whether
              cmd/ deserves test coverage now that it's accumulated
              real logic (list filtering, repo grant/revoke flows)
              beyond thin command-line plumbing.

Session 5 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
Progress-tracker housekeeping only — no code changes this session.
Verified the file completion table below is accurate (every listed
file already "done"; re-ran `go build ./... && go vet ./... && go
test ./... -cover` to confirm) and closed out session 4's pointer
with the exact "Next session should start with" phrasing the tracker's
own protocol expects.
Completed:    (none — documentation/verification only)
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go (new file). cmd/ is
the only package below the spec's 80% coverage bar — it's currently
at 0% (see `go test ./... -cover`) despite cmd/list.go now having real
branching logic (visibility filtering, the owner/member role switch)
that's only been exercised by manual smoke tests in this transcript,
not by an automated suite. Start with cmd/list.go's filtering logic
since it's the most recently changed and least covered; cmd/repo.go's
grant/revoke flows (session 3) are the next most logic-heavy and
untested after that.

Session 6 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User asked for confirmation that a saved auth token persists until it
actually expires (it already did — gitcollect has never re-prompted
based on elapsed time, only on the platform rejecting the token), then
asked for a UX improvement: surface a "Run: gitcollect auth" hint
whenever a stored token gets rejected (expired/revoked/scope-changed),
matching the hint that already existed for the no-token-at-all case.
Completed:    cmd/root.go's Execute() now checks errors.Is(err,
              api.ErrUnauthorized) on any command's final returned
              error and prints output.Suggestion("gitcollect auth") —
              centralized here (not per-command) since ErrUnauthorized
              can originate from any API call buried inside any
              command. Verified against `gitcollect show` with a
              token that real GitHub rejects (network call, not
              mocked) — hint fires correctly.
              cmd/whoami.go needed its OWN copy of this hint: it
              deliberately does NOT return an error when one host's
              token is rejected (it loops over every authenticated
              host and reports each inline, "error: ..." in that
              host's row, specifically so one bad host doesn't hide
              the others' valid status) — so Execute()'s centralized
              check never sees that error. Added an anyRejected bool
              tracked across the loop; prints the same suggestion
              once after the table if any row hit ErrUnauthorized.
              Verified against real GitHub the same way.
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go — unchanged from
session 5's pointer; this session touched cmd/root.go and
cmd/whoami.go but added no tests for either (cmd/ is still at 0%
coverage). If picking up cmd/ test coverage as planned, fold in
coverage for Execute()'s new ErrUnauthorized branch and whoami.go's
anyRejected branch while there — both are easy to hit with a fake
unauthenticated httptest.Server, same pattern as internal/api/
api_test.go already uses.

Session 7 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User asked for visibility into actual code changes in repos (not just
gitcollect's own access mutations), wanting to know who changed what
and on which branch. Confirmed design via AskUserQuestion before
building: (1) a new standalone command rather than folding into
`audit`, (2) default-branch-only commit checking (not all branches),
(3) persist fetched commits to a local log in addition to live
display. This is a genuinely new capability — gitcollect previously
had zero visibility into git commit history, only into its own
access-control mutations.
Completed:    internal/api/client.go — added CommitInfo struct and
              ListCommits(owner, repo, branch string, limit int)
              ([]CommitInfo, error) to the Client interface; added
              DefaultBranch to RepoInfo.
              internal/api/github.go — GetRepo now decodes
              default_branch; new ListCommits hits GET
              /repos/{owner}/{repo}/commits?sha={branch}&per_page={n},
              resolving Author to the linked GitHub login when
              available, falling back to the raw commit author name
              otherwise (commit.author can be null when GitHub can't
              link the email to an account).
              internal/api/gitlab.go — GetRepo now decodes
              default_branch; new ListCommits hits GET
              /projects/{id}/repository/commits?ref_name={branch}&
              per_page={n}. Author here is ALWAYS the raw
              author_name — GitLab's commits endpoint does not expose
              the pushing user's platform username, unlike GitHub's.
              internal/activity/activity.go (new package) — mirrors
              internal/audit's structure (Append/Read as
              newline-delimited JSON under
              ~/.gitcollect/activity/<collection>.log) but is
              deliberately a separate package/log from audit: audit
              tracks access mutations gitcollect performed; activity
              tracks git commits gitcollect observed via the
              platform API. Added Filter(entries, repo, user, since)
              and KnownSHAs(entries) (dedup helper keyed by
              "repo\x00sha", since the same commit can recur across
              runs within the fetch window).
              internal/config/config.go — added ActivityDir().
              cmd/activity.go (new) — `gitcollect activity
              <collection> [--repo r] [--since dur] [--limit n]
              [--json]`. Uses loadForGit + access.FilterAccessible
              (same as clone/pull/status) since checking commit
              history needs a live, authenticated API call
              regardless of collection visibility — there is no
              local-only path, same reasoning as clone. Fetches
              concurrently across repos (bounded by
              defaultActivityConcurrency=4, same pattern as
              SyncCollaborators). Resolves each repo's default
              branch via GetRepo before calling ListCommits (falls
              back to "main" if the platform ever returns an empty
              default_branch, which shouldn't happen but costs
              nothing to guard). Merges freshly fetched commits with
              everything previously recorded (mergeActivity,
              dedup by repo+SHA) so the displayed table always shows
              full known history, not just this run's fetch window —
              --limit only bounds the live fetch, not what's shown.
              Tests: internal/api/api_test.go gained
              TestGitHubListCommits(_Error), TestGitHubGetRepo_
              DefaultBranch, and GitLab equivalents, using the
              existing httptest mock-server helpers.
              internal/activity/activity_test.go (new, 84.4%
              coverage) mirrors audit_test.go's test shapes
              (Append/Read round-trip, OpenFailure, malformed line,
              missing log, Filter, plus KnownSHAs which audit has no
              equivalent of).
              cmd/activity_test.go (new) — cmd/ package's FIRST ever
              test file. Only covers the two pure-logic helpers
              (shortSHA, mergeActivity) that need no network/auth;
              cmd/ is still far below the 80% bar but this starts
              chipping at the previously-untouched 0%.
              Updated existing mocks in internal/collection/
              collection_test.go and internal/access/access_test.go
              to implement the new ListCommits method (interface
              change broke both — minimal no-op stubs, neither test
              exercises commit fetching).
              README.md — new "Seeing code changes across a
              collection's repos" section; docs/index.html — new
              `gitcollect activity` command block in the
              inspect/audit group (renamed to "Access inspection,
              audit & activity").
              Did NOT do a live end-to-end smoke test of the actual
              `gitcollect activity` CLI command against a real GitHub
              repo — loadForGit requires a real valid token (same
              constraint hit in session 6), and substituting one
              without the user's involvement felt like the wrong
              call. Verified instead via: unit tests against
              httptest mock servers (confirms the GitHub/GitLab JSON
              parsing is correct against realistic response shapes),
              `go build`/`go vet`/`go test ./...` across every
              package, and a real (but token-less, so correctly
              rejected with "not authenticated") run of the compiled
              binary to confirm the command is wired up and routes
              through loadForGit as expected.
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go — STILL the oldest
unaddressed pointer (sessions 5 and 6 both named it; cmd/ remains
under 10% coverage even after this session's activity_test.go). If
the user ever provides a real token for manual verification, the one
thing this session couldn't confirm end-to-end is `gitcollect
activity` against a real repo with actual commit history (the JSON
parsing is unit-tested, but a live run would catch anything subtly
wrong about real-world API response shapes that the hand-written
httptest fixtures might not reproduce, e.g. an author field present
but with an empty login string instead of being fully null).

Session 8 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User asked for a git/GitHub-style confirmation before deleting a
collection or repo — type the name to confirm, not just y/N.
Checked first: cmd/delete.go (collection deletion) already did this
via output.ConfirmWord since an earlier session — nothing to change
there. cmd/remove.go (repo removal from a collection) was still using
the plain y/N output.Confirm, inconsistent with delete.go.
Completed:    cmd/remove.go — swapped output.Confirm(prompt) for
              output.ConfirmWord(prompt, repoName), matching
              delete.go's pattern exactly (user must type the exact
              repo name being removed). Reworded the prompt from a
              question ("Remove ... ?") to a statement ("This will
              remove ...") to match delete.go's phrasing convention,
              since ConfirmWord's own "(type %q to confirm)" suffix
              already implies the question. Left group.go/member.go/
              visibility.go's plain y/N Confirm prompts untouched —
              user's request was specifically scoped to "collection
              or repo" deletion, not group/member removal or
              visibility changes, and those are lower-stakes (a
              group has no platform-side effect of its own; member
              removal and visibility changes are reversible by
              re-adding/re-switching, unlike delete/remove which
              revoke platform access that has to be re-granted from
              scratch).
              docs/index.html — updated remove's description to
              state "Requires typing the repo's name to confirm"
              (was "Prompts for confirmation"), matching delete's
              existing description.
              No new test needed: output.ConfirmWord already has
              full unit test coverage (TestConfirmWord in
              output_test.go) covering both the exact-match-accepts
              and mismatch-rejects cases; cmd/remove.go itself has
              no test file (consistent with the rest of cmd/, a
              known pre-existing gap — not introduced by this
              change).
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by four consecutive sessions (5, 6, 7, 8). If this keeps
getting deferred, consider just doing a focused cmd/ test-coverage
pass as its own session rather than continuing to postpone it in
favor of whatever feature request comes in next.

Session 9 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User asked for `gitcollect show <collection>` to report, per repo,
whether the CALLER specifically has access or is denied (not just the
configured rule), and confirmed clone already only downloads repos
the caller can access (it does — FilterAccessible already enforced
this; no change needed there, just confirmed and documented).
While building this, found and fixed a real latent bug: investigated
how `inspect --user <owner>` would behave, since show's new column
needed the same "can this username access this repo" logic.
internal/collection/access.go's CanAccessRepo has NO owner bypass by
design (PROMPT.md's documented decision table treats it as a pure
member rule — "user not a member → false", no owner exception) and
relies on CALLERS to special-case the owner, which enforce.go's
CheckRepoAccess/FilterAccessible already do correctly (`caller !=
col.Owner` guards before calling CanAccessRepo/AccessibleRepos). But
internal/access/inspect.go's UserAccessMap/RepoAccessMap/FullMatrix
never got that same guard, while collection.WhyCanAccess DOES special-
case the owner (returns the string "owner" unconditionally). Net
effect: before this session, `gitcollect inspect <col> --user
<owner>` on a private collection where the owner isn't separately
listed as a member would show CanAccess=false paired with
Reason="owner" for every repo — a contradictory, confusing result
that was reachable by any owner just inspecting their own collection
the normal way (not an edge case).
Completed:    internal/access/inspect.go — added a single `decide()`
              helper (Visibility-public check, then owner check, then
              fall through to col.CanAccessRepo/WhyCanAccess) and
              routed UserAccessMap/RepoAccessMap/FullMatrix through it
              instead of calling col.CanAccessRepo/WhyCanAccess
              directly. Fixes the owner-bypass inconsistency above
              for all three at once, and means cmd/show.go's new
              column inherits the fix automatically rather than
              needing its own copy of the same workaround.
              internal/access/access_test.go — added
              TestUserAccessMap_OwnerBypass, a regression test
              constructing a private collection where the owner is
              deliberately NOT a listed member, asserting
              UserAccessMap now reports CanAccess=true/Reason="owner"
              instead of the old false/"owner" contradiction. Also
              confirms RepoAccessMap correctly excludes the owner from
              its member-only rows (since they were never added to
              col.Members) — that part was never wrong, just worth
              pinning down given how close the related bug was.
              cmd/show.go — runShow now captures caller from
              loadForRead (was discarded as `_` before) and calls
              access.UserAccessMap(col, caller) to build a third
              "YOU" table column: "✓ yes" or "✗ no — <reason>". Local
              variable named `access` (shadowing the new `access`
              package import) renamed implicitly by restructuring —
              the repo-rule string is now built inside the new
              buildShowRepoRows() helper, extracted specifically so
              it's unit-testable without auth (cmd/'s tests can't
              easily exercise runShow itself, since loadForRead needs
              a real client for private collections). Added a footer
              — "You can't access N repo(s): ..." + "Run: gitcollect
              inspect <col> --user <caller>" — mirroring clone's
              existing skipped-repo messaging pattern. --json gained
              you_can_access/you_reason per repo. For PUBLIC
              collections caller is "" (loadForRead never resolves an
              identity there) — decide()'s Visibility-public check
              fires before the owner/membership checks regardless of
              username, so YOU is correctly ✓ for everyone without
              needing real auth; verified live against a hand-built
              public collection fixture (no token required) showing
              the exact REPO/ACCESS RULE/YOU table and matching --json
              output.
              cmd/show_test.go (new) — TestBuildShowRepoRows(_Empty)
              covers the table-building helper directly (rule text +
              YOU column + denied-list collection, no auth needed);
              TestToShowOutput_OwnerNotListedAsMember is the cmd-level
              regression test mirroring the access-package one above.
              docs/index.html — show's command-reference entry now
              describes the YOU column; the two-person walkthrough
              (session 7) gained a new step 5, "Your teammate checks
              what they can actually reach" with `gitcollect show`
              real output, and step 6 (clone) now notes show and
              clone are backed by the same access decision so they
              can never disagree. README.md's "Listing the repos"
              section rewritten to match.
              Did NOT test the private-collection denial path against
              the real running binary (needs a real authenticated
              client, same constraint as every other private-
              collection scenario this transcript has hit) — covered
              instead by the unit tests above plus the already-real
              public-collection smoke test.
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by five consecutive sessions (5 through 9). Strongly
consider making this its own session rather than deferring again.

Session 10 — 2026-06-29 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User asked to bring the "methodology" — the PROGRESS TRACKER, the
spec body, and docs/index.html — fully up to date with every feature
added across sessions 1-9. No new functionality requested; this was a
documentation-accuracy audit.
Completed:    Audited every illustrative example in "## Command
              behaviour — key interactions" (~line 555) against the
              actual cmd/*.go source and found it had drifted
              significantly — this section was largely still the
              ORIGINAL pre-implementation spec sketch, never updated
              as the real output format diverged during sessions
              1-9. Specific inaccuracies fixed (each verified against
              the real print/Sprintf calls, not rewritten from
              memory):
                - inspect --user's REASON column said "analysts group
                  required (not a member)"; real WhyCanAccess says
                  "no access — group analysts required".
                - member add's example showed fictional
                  "Syncing platform access..." and a multi-line "To
                  grant diana access..." paragraph that printAccess-
                  Breakdown never prints; real output is just
                  "Granted access: ..." / "Skipped: ... (reason)" +
                  one Suggestion line.
                - repo access's example showed "(4 people)" counts
                  and a live "Revoking access for N members..."
                  progress block that doesn't exist — SetRepoAccess
                  syncs before runRepoAccess prints anything, so
                  there's no progress to show.
                - clone's example said "Access verified (bob ·
                  groups: red-team)" — real string has no literal
                  "groups:" prefix, just the joined group names.
                - group add's guided error used "Add them first: ..."
                  instead of the real "Run: ..." Suggestion prefix.
                - the private-collection non-disclosure example
                  showed the collection name inside the error text;
                  access.ErrForbidden's actual message is generic
                  and never echoes the name back.
              Added brand-new example blocks for everything that
              shipped in sessions 3-9 and had NO illustration at all
              before now: `repo grant`/`repo revoke` (success + the
              ErrRepoOpen guard), `list --private`/`--public`, `show`
              with the session-9 YOU column (including the owner-
              bypass note), `remove`'s session-8 type-to-confirm
              prompt, and `activity` (session 7).
              Added a one-line warning at the top of the Command
              behaviour section telling future sessions to keep these
              blocks synced with real print statements going
              forward, since letting them drift this far took several
              sessions to accumulate unnoticed.
              Verified docs/index.html's command reference already
              covers all 20 cmd/*.go files (cross-checked by listing
              cmd/*.go and grepping every <p class="sig"> entry —
              no gaps found, this page had stayed current already).
              Did not change any Go code, tests, or docs/index.html
              this session — confirmed it didn't need it (already
              updated through session 9's show/walkthrough work) and
              focused the audit on PROMPT.md's body, which had not
              been re-verified against the implementation since the
              original spec was written.
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by SIX consecutive sessions (5 through 10).
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

cmd/root.go                                  done         +ErrUnauthorized hint in Execute(), session 6
cmd/auth.go                                  done
cmd/whoami.go                                done         +anyRejected hint, session 6
cmd/init.go                                  done
cmd/delete.go                                done
cmd/list.go                                  done         redesigned session 4 — see decisions log
cmd/show.go                                  done         +per-caller YOU column, session 9
cmd/show_test.go                             done         new, session 9
cmd/visibility.go                            done
cmd/add.go                                   done
cmd/remove.go                                done         y/N → type-name-to-confirm, session 8
cmd/repo.go                                  done         grant/revoke subcommands added session 3
cmd/member.go                                done
cmd/group.go                                 done
cmd/inspect.go                               done
cmd/audit.go                                 done
cmd/activity.go                              done         new command, session 7 — not in original spec
cmd/activity_test.go                         done         session 7; cmd/'s first ever test file (6.5% pkg cov)
cmd/clone.go                                 done
cmd/pull.go                                  done
cmd/status.go                                done
cmd/version.go                               done
(shell completion: cobra's built-in `completion` subcommand covers this
 — no separate file needed; verified with `gitcollect completion --help`)

internal/collection/collection.go            done
internal/collection/access.go                done         groupsContaining (dead code) removed
internal/collection/mutation.go              done         +GrantRepoUser/RevokeRepoUser, session 3
internal/collection/collection_test.go       done         85.9% coverage; mock +ListCommits stub, session 7

internal/access/enforce.go                   done
internal/access/sync.go                      done
internal/access/inspect.go                   done         +decide() owner-bypass fix, session 9 — see decisions log
internal/access/access_test.go               done         93.0% coverage; +TestUserAccessMap_OwnerBypass, session 9

internal/audit/audit.go                      done
internal/audit/audit_test.go                 done         82.8% coverage

internal/activity/activity.go                done         new package, session 7 — not in original spec
internal/activity/activity_test.go           done         84.4% coverage, session 7

internal/git/git.go                          done
internal/git/git_test.go                     done         91.7% coverage

internal/api/client.go                       done         +ListCommits, +CommitInfo, +RepoInfo.DefaultBranch, session 7
internal/api/github.go                       done         githubBaseURL: const → var (see decisions log); +ListCommits, session 7
internal/api/gitlab.go                       done         +ListCommits, session 7
internal/api/api_test.go                     done         85.4% coverage (was 84.8%; +ListCommits/DefaultBranch tests, session 7)

internal/config/config.go                    done         +ActivityDir(), session 7
internal/config/config_test.go               done         82.5% coverage (was 82.8%; +ActivityDir assertion, session 7)

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
- cmd/list.go semantics (REVISED session 4 — this entry describes the
  CURRENT behavior, not the original spec's): a collection is "yours" if
  the cached username for its Host equals col.Owner ("owner") or appears
  in col.Members ("member"); --all no longer exists, `list` with no flags
  now shows every collection that's "yours" regardless of visibility.
  --private/--public filter that set down to one visibility (passing both
  is the same as passing neither). Collections on a host you've never
  authenticated to (no cached username) are skipped with a warning rather
  than erroring the whole command — one bad/foreign manifest shouldn't
  break `list`. NOTE: the role switch checks `username == col.Owner`
  BEFORE `col.IsMember(username)`, so being the owner always wins —
  adding yourself as a member of your own collection does NOT change
  role from "owner" to "member". This bit a piece of advice I gave the
  user in session 3/4 before verifying it empirically; don't repeat that
  mistake — check switch/case ordering before claiming a workaround
  exists, especially for early-matching-wins constructs like this one.
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
- STILL TRUE after the session 4 redesign (not a bug, just worth knowing):
  a public collection you are NOT a member of (and don't own) is still
  invisible to `gitcollect list` regardless of any flag — the role-switch's
  default case always `continue`s past it. `list`/`list --private`/`list
  --public` are about collections you BELONG to; there's no "browse every
  public collection that exists" command, by design (gitcollect never
  scans other users' manifests). The session-1-era help text used to
  overclaim "Public collections ... are shown by default" in a way that
  implied otherwise; the session 4 rewrite of Long no longer makes that
  claim ("every collection you own or are a member of, public or
  private"), so the docs/behavior mismatch from earlier sessions is
  resolved even though the underlying limitation itself is unchanged.
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
- SESSION 3, FEATURE BEYOND THE ORIGINAL SPEC (user-requested): per-user
  repo grant/revoke. Before this, the only way to change one user's
  access to one repo was `repo access --users u1,u2,...`, which REPLACES
  the repo's entire Users list — you had to already know and retype
  everyone currently on it. Added collection.GrantRepoUser(repoName,
  username, client) and RevokeRepoUser(repoName, username, client) in
  mutation.go, following the exact same validate → mutate-in-memory →
  SyncCollaborators → rollback-on-failure → Save pattern as
  SetRepoAccess/AddToGroup/etc., plus `gitcollect repo grant <collection>
  <repo> <user>` / `repo revoke <collection> <repo> <user>` in cmd/repo.go
  (owner-only, audited as "repo.user.grant"/"repo.user.revoke", same as
  every other mutation command).
  THE FOOTGUN THIS GUARDS AGAINST: CanAccessRepo treats a repo with
  Groups=[] AND Users=[] as "open to all members" — that's the whole
  point of the empty-list convention in the spec's data model. That means:
    - GrantRepoUser on a repo that's currently open (Groups=[] Users=[])
      must refuse (ErrRepoOpen), not append to Users — appending would
      flip Users from empty to non-empty, which makes CanAccessRepo
      switch from "open to everyone" to "only check Groups/Users", and
      since Groups is still empty, that instantly narrows access down to
      JUST the one user being "granted," silently revoking every other
      member. The fix is structural, not a warning: GrantRepoUser checks
      `len(repo.Groups) == 0 && len(repo.Users) == 0` before ever
      touching the list.
    - RevokeRepoUser has the mirror-image bug: removing the last
      remaining individual user from a repo whose Groups is also empty
      would leave Groups=[] Users=[] — which CanAccessRepo reads as
      "open to all members" again. So revoking the last person who had
      restricted access would silently OPEN the repo to everyone.
      RevokeRepoUser computes what Users would become *before* committing
      and refuses (ErrRepoWouldOpen) if that result would be empty while
      Groups is also empty.
  Both are no-ops (not errors) when the user already has/lacks the
  individual grant — checked once in cmd/repo.go (mirroring member.go's
  "Already a member" pattern) and again, defensively, inside the
  mutation methods themselves.
  ALSO FOUND WHILE WRITING TESTS FOR THIS: internal/collection/
  collection_test.go's mockClient.AddCollaborator/RemoveCollaborator/
  CheckCollaborator wrote to mockClient.collaborators (a plain
  map[string]bool) with no synchronization. SyncCollaborators always
  calls these concurrently (up to maxConcurrentSyncs=4 goroutines), and
  once a test exercised more than one job at a time, Go's runtime threw
  "fatal error: concurrent map writes" — this is the Go runtime's
  always-on concurrent-map-write panic, not the optional `-race`
  detector, so it would have bitten any test (or even a `-race`-less CI
  run) the moment two jobs raced, not just under `go test -race`. Fixed
  by adding a sync.Mutex to mockClient and locking around all three
  methods' map access. Any future mock api.Client written for this
  package must assume SyncCollaborators will call it from multiple
  goroutines and lock accordingly — don't repeat this with a "simpler"
  unsynchronized mock later.
```
