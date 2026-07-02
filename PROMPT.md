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
gitcollect's local YAML is a _declaration of intent_ — it describes who should
have access. The GitHub/GitLab platform is the _enforcement point_ — it is
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
gitcollect whoami                            Show authenticated user per host
gitcollect whoami --json                     Machine-readable output (added session 11)

── Collection lifecycle ────────────────────────────────────────────────────
gitcollect init <name>                        Create collection (default: private)
                                              (no --owner flag — owner is always
                                               whoever ran "gitcollect auth"; an
                                               --owner-for-orgs flag was considered
                                               in session 11 and deliberately NOT
                                               added — see decisions log, every
                                               owner-only check is a literal
                                               caller==col.Owner string compare,
                                               which an org name could never satisfy)
gitcollect delete <collection>               Delete collection + revoke all access
                                              (requires typing the collection name
                                               to confirm)
gitcollect list                              List your collections (owned + member), any visibility
gitcollect list --private                    Filter to private collections only
gitcollect list --public                     Filter to public collections only
  (REVISED session 4, on user request — original spec's "list --all" is gone, and
   stayed gone after being reconsidered in session 11; see decisions log. "list"
   with no flags shows everything --all used to. Each row also reports staleness:
   a collection whose updated_at is >30 days old gets a "Last updated N days ago"
   warning printed below the table — added session 11.)
gitcollect show <collection>                 Summary: repos, members, groups, plus a
                                              per-repo access column. For an ordinary
                                              caller: YOU (✓/✗ + reason, added session
                                              9) and, for any denied repo, the exact
                                              fix command in the footer (added session
                                              11, via Collection.FixCmd). For the
                                              collection OWNER specifically: a WHO HAS
                                              ACCESS column instead (every member who
                                              can reach each repo) — added session 11,
                                              since a YOU column is pointless for an
                                              owner who always passes every check.
                                              Also gets the same >30-day stale warning
                                              as list, printed above the tables.
gitcollect visibility <collection> public|private   Change visibility
gitcollect transfer <collection> <new-owner>  Transfer ownership to another member
                                              (member must already be in the collection;
                                               typed ConfirmWord confirmation; previous
                                               owner becomes a regular member; new owner
                                               must not hold a group admin role)
gitcollect scale <collection> organisation|team   Switch tier:
                                              organisation = enable group admin support
                                               (allows owner to delegate per-group
                                               management via group admin add/remove);
                                              team = disable it (revokes all group admin
                                               assignments after confirmation if any exist);
                                              idempotent: switching to current tier is no-op

── Repo management ─────────────────────────────────────────────────────────
gitcollect add <collection> <repo> [repo...]  Add one or more repos (default: all
                                              members). Multi-repo support added
                                              session 12 — see decisions log. One
                                              failure doesn't abort the rest; a
                                              malformed repo name anywhere in the
                                              batch is checked up front, before
                                              anything is added.
gitcollect add <collection> <repo> --new-repo-visibility private|public
                                              On TTY: if repo doesn't exist on the
                                              platform, prompts to create it.
                                              Default visibility: private.
                                              Non-TTY: skips silently.
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
gitcollect member add <collection> <username> [username...]   Add one or more
                                                    members to a collection. Multi-
                                                    username support added session
                                                    12 — see decisions log; with more
                                                    than one username, each one's
                                                    output is printed under its own
                                                    "--- username ---" header, and one
                                                    failing doesn't abort the rest.
                                                    If any newly granted repo leaves a
                                                    member with a pending (unaccepted)
                                                    GitHub collaborator invite, warns
                                                    about it with the accept URL —
                                                    added session 11, via
                                                    Client.GetPendingInvite. GitLab
                                                    never has this state (project
                                                    membership there is immediate), so
                                                    this never fires on GitLab hosts.
gitcollect member remove <collection> <username>   Remove member + revoke all access
gitcollect member remove <collection> <username> --confirm-self   Required to remove yourself
gitcollect member list <collection>               List members + their group memberships

── Group management ────────────────────────────────────────────────────────
gitcollect group create <collection> <group>       Create a group
gitcollect group delete <collection> <group>       Delete group (blocked if repos use it)
gitcollect group add <collection> <group> <user> [user...]   Add one or more
                                                    members to a group. Multi-user
                                                    support added session 12 — see
                                                    decisions log; one failing doesn't
                                                    abort the rest. When organisation
                                                    tier enabled, group admin of that
                                                    specific group may also run this.
gitcollect group remove <collection> <group> <user> Remove member from group.
                                                    When organisation tier enabled,
                                                    group admin of that specific group
                                                    may also run this.
gitcollect group list <collection>                List groups + members
gitcollect group show <collection> <group>        Show group members + accessible repos

gitcollect group admin add <collection> <group> <user>    Grant group admin rights
                                                    (owner-only; requires org tier)
gitcollect group admin remove <collection> <group> <user> Revoke group admin rights
                                                    (owner-only, or self-removal)
gitcollect group admin list <collection>          List all group admin assignments

── Access inspection ───────────────────────────────────────────────────────
gitcollect inspect <collection> --user <username>  Show full access map for a user.
                                                    Denied rows get a "To fix:" footer
                                                    listing the exact command — added
                                                    session 11 (Collection.FixCmd).
gitcollect inspect <collection> --repo <repo>      Show who can access a repo and why
                                                    (same "To fix:" footer addition)
gitcollect inspect <collection>                    Show full collection access matrix

── Audit trail ─────────────────────────────────────────────────────────────
gitcollect audit <collection>                 Show access change log
gitcollect audit <collection> --user <u>      Filter log by user
gitcollect audit <collection> --since <dur>   Filter log by time: 1h, 24h, 7d, 30d, or
                                               90d ONLY (strict allow-list, no other
                                               duration accepted — changed session 11
                                               on user request; previously accepted
                                               anything time.ParseDuration understood
                                               plus arbitrary "Nd"; see decisions log)
gitcollect audit <collection> --json          Machine-readable output

── Code activity (added session 7; not in the original spec — see decisions
   log) ────────────────────────────────────────────────────────────────────
gitcollect activity <collection>              Show commits across accessible
                                               repos' default branch, fetched
                                               live + recorded to
                                               ~/.gitcollect/activity/<name>.log
gitcollect activity <collection> --repo <r>   Limit to one repo
gitcollect activity <collection> --since <dur>  Same strict allow-list as audit
                                                 --since (1h/24h/7d/30d/90d only)
gitcollect activity <collection> --limit <n>  Max commits fetched per repo this run (default 10)
gitcollect activity <collection> --json       Machine-readable output

── Git operations ──────────────────────────────────────────────────────────
gitcollect clone <collection>                 Clone all accessible repos
gitcollect clone <collection> --pick "r1 r2"  Clone selected repos only — value is
                                               whitespace-separated (changed session
                                               11 on user request, was comma-separated
                                               before); repeating --pick also still
                                               works (--pick r1 --pick r2)
gitcollect clone <collection> --dry-run       Preview without executing
gitcollect clone <collection> --concurrency 8 Override parallel limit (default: 4)
gitcollect clone <collection> --dest <dir>   Clone into specific directory
                                              If a skipped repo turns out to be a
                                              pending GitHub invite rather than a
                                              genuine denial, warns about it with the
                                              accept URL + retry command — added
                                              session 11.
gitcollect pull <collection>                 git pull inside all cloned repos
gitcollect status <collection>               git status inside all repos
gitcollect sync <collection>                  Clone every repo not yet present
                                               locally, pull every repo that already
                                               is — one pass instead of running clone
                                               then pull separately. Added session 11;
                                               not in the original spec.
gitcollect sync <collection> --dest <dir>    Directory to clone into / where repos
                                              were already cloned
gitcollect sync <collection> --dry-run        Preview without executing
gitcollect sync <collection> --concurrency 8  Override parallel limit (default: 4)

── System ──────────────────────────────────────────────────────────────────
gitcollect version                           Print version + platform
gitcollect completion bash|zsh|fish|powershell  Generate shell completion script
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
version: "2"
name: cybersecurity
description: "Penetration testing and security research tools"
host: github.com
owner: <platform-user-id>          # immutable platform ID (Version "2")
namespace: acme-corp                # optional; defaults to owner's cached login
visibility: private # public | private
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
    users: [] # open to all members

  - name: vuln-scanner
    groups: [red-team]
    users: [] # only red-team members (alice, bob)

  - name: threat-reports
    groups: [analysts]
    users: [diana] # analysts group OR diana individually

  - name: ops-runbooks
    groups: [ops]
    users: [] # only ops group (diana)

  - name: ctf-writeups
    groups: []
    users: [] # open to all members
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
    Owner       string              `yaml:"owner"`     // immutable platform ID (Version "2")
    Visibility  Visibility          `yaml:"visibility"`
    Members     []string            `yaml:"members"`
    Groups      map[string][]string `yaml:"groups"`    // group name → []platform-ID
    Repos       []RepoAccess        `yaml:"repos"`
    Logins      map[string]string   `yaml:"logins"`    // ID → cached login
    Namespace   string              `yaml:"namespace,omitempty"` // org/user for API paths; defaults to Logins[Owner]
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

> Every block below is the _real_ output of the current implementation
> (verified against cmd/\*.go and internal/output/output.go as of session
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

2026-01-20 14:32  alice       member.add            bob                   Added member
2026-01-20 14:35  alice       member.add_to_group   bob → red-team        Added bob to red-team
2026-01-19 09:10  alice       repo.access.set       vuln-scanner          open to all members → groups: red-team
2026-01-15 10:00  alice       init                  cybersecurity         Collection created (private)
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

### gitcollect member add / group add / add — multiple values (session 12)

`member add`, `group add`, and `add` all accept more than one trailing
positional argument now (`cobra.MinimumNArgs` instead of `ExactArgs`), so a
whole team or a batch of repos can be added in one invocation instead of one
command per item. Each item is processed independently — verified against
the real CLI by exercising `addOneMember`/`addOneToGroup`/`addOneRepo`
directly in a throwaway test and capturing actual output, not hand-derived:

```
$ gitcollect member add research alice bob charlie

--- alice ---
✓ Added alice to research

  Granted access: repo-a, repo-b, repo-c

--- bob ---
✓ Added bob to research

  Granted access: repo-a, repo-b, repo-c

--- charlie ---
✓ Added charlie to research

  Granted access: repo-a, repo-b, repo-c
```

`member add` prints a `--- username ---` header before each one's block
since that block is multiple lines (the "--- " header is only printed when
more than one username was given, so the single-username invocation's
output is byte-for-byte unchanged from before session 12). `group add` and
`add` skip the header — their per-item success output is a single line, so
there's nothing to disambiguate.

A failure partway through a batch does not abort the rest — every item is
attempted, failures are collected, and the command reports them together as
one line at the very end, exiting non-zero only if at least one item
actually failed:

```
$ gitcollect member add research alice bad_name

--- alice ---
alice is already a member of "research"

--- bad_name ---
✗ member add: 1 of 2 failed: bad_name (invalid name: username "bad_name" must match ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$)
```

`add`'s repo-name format validation is the one exception to "check each item
independently": every repo name is validated up front, before the
collection is even loaded, so a single malformed name anywhere in the batch
is a usage error (exit 2) for the whole command rather than a partial
failure — consistent with how usage errors work everywhere else in
gitcollect (see the exit-code decision in the decisions log). Once past that
gate, each repo's own already-in-collection/not-found/sync-failure outcomes
are independent, same as member add and group add.

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
groups">` — no literal word "groups:" in it. For a _public_ collection it's
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
  - Repo name: `^[a-zA-Z0-9._-]{1,100}$`
  - Username: `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`
  - Group name: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,30}$`
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

| Package                      | Test coverage required                                                                        |
| ---------------------------- | --------------------------------------------------------------------------------------------- |
| `internal/collection`        | Load, Save, Validate, AddMember, RemoveMember, AddToGroup, RemoveFromGroup, SetRepoAccess     |
| `internal/collection/access` | Full decision table (see matrix below)                                                        |
| `internal/access`            | CheckCollectionAccess, CheckRepoAccess, FilterAccessible, SyncCollaborators using mock client |
| `internal/audit`             | Append, Read, Filter by user and duration                                                     |
| `internal/api`               | GitHub + GitLab against `httptest.Server` mocks                                               |
| `internal/git`               | Correct args to git subprocess (mock exec)                                                    |
| `internal/config`            | File at 0600, directory at 0700                                                               |
| `internal/output`            | Table alignment, JSON, Confirm true/false                                                     |

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

Session 11 — 2026-06-30 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User dropped a second spec file, PROMPT_v2.md, into the project root
mid-session with its own from-scratch PROGRESS TRACKER (every file
"todo") and an instruction to generate files one at a time as if
starting over. Paused before writing any code, since the codebase was
already 10 sessions in and fully working — flagged the conflict to the
user via AskUserQuestion rather than complying literally. User
confirmed: treat PROMPT_v2.md as a delta spec against the existing
implementation, not a rewrite — see decisions log entry "PROMPT_v2.md
TREATED AS A DELTA SPEC". This happened a SECOND time later in the same
session (same framing, "previous session was interrupted, resume from
the tracker") and was met the same way — re-verified PROMPT_v2.md's
tracker was still blank (because it was never meant to be live) and
confirmed with the user again before continuing.
Completed:    Diffed PROMPT_v2.md against the real implementation and
              shipped every non-conflicting delta, each confirmed with
              the user via AskUserQuestion before implementing where
              it touched an existing deliberate decision:
                - Owner bypass moved from scattered per-caller checks
                  into CanAccessRepo/WhyCanAccess directly (user chose
                  this over keeping session 9's layered approach) —
                  see decisions log.
                - --owner flag on `init` considered and explicitly
                  rejected (architectural conflict: owner checks are
                  literal string equality against the caller's own
                  login, which an org name could never satisfy) — see
                  decisions log.
                - `list --all` stays removed (reconfirmed; see session
                  4's original decision).
                - --pick: comma-separated → space-separated
                  (StringArrayVar + splitPick), matching PROMPT_v2.md's
                  examples exactly per user's explicit choice. Caught
                  and fixed a bug this introduced in cmd/pull.go's
                  "run clone --pick ..." suggestion, which was still
                  comma-joining.
                - --since (audit + activity): flexible ParseDuration-
                  plus-"Nd" parsing → strict 1h/24h/7d/30d/90d
                  allow-list, per user's "match v2 exactly" choice.
                - `show`: owner now sees a WHO HAS ACCESS column
                  instead of YOU (trivially always true for an owner);
                  added a >30-day-stale warning shared with `list`.
                - Collection.FixCmd(username, repo) — the exact
                  gitcollect command to fix a denial — surfaced in
                  show's denied-repo footer and inspect --user/--repo's
                  new "To fix:" footer.
                - Levenshtein-distance (≤2) typo suggestion for
                  unrecognized collection names, scoped ONLY to
                  loadCollection's owner-required path — deliberately
                  NOT added to loadForRead's private-collection
                  non-disclosure path, to avoid leaking the existence
                  of private collections to non-members via typo
                  suggestions.
                - whoami --json.
                - Client.GetPendingInvite (GitHub: real GET
                  /repos/{owner}/{repo}/invitations check; GitLab:
                  stub, always false) wired into `member add` (warn
                  after granting access) and `clone` (warn when a
                  skipped repo is actually a pending invite, not a
                  genuine denial) — see decisions log.
                - `gitcollect sync` (new command, not in the original
                  spec): clone-missing + pull-existing in one pass.
                  Added git.PullWithSummary to internal/git/git.go to
                  report new-commit counts per repo. Concurrent
                  (default 4), reuses clone.go's cloneOne/
                  selectCloneTargets — see decisions log.
              Test files added: cmd/root_test.go, cmd/member_test.go,
              cmd/clone_test.go (extended), cmd/sync_test.go,
              internal/git/git_test.go (extended with a new fake-git
              harness that varies output per call, needed for
              PullWithSummary's before/after HEAD comparison — the
              existing installFakeGit always returns a fixed "ok").
              Full go build/vet/test pass: all packages green, no
              regressions. Coverage held steady or improved across
              every touched package (cmd: 12.9%, access: 93.9%, git:
              85.4%, api: 85.5%, output: 98.1%).
              Updated PROMPT.md (this file): command surface section,
              project structure tree, file completion table, and this
              session log entry — folding PROMPT_v2.md's delta in here
              rather than maintaining a second tracker.
In progress:  (none)
Blockers:     (none)
Decisions:    See decisions log entries: "PROMPT_v2.md TREATED AS A
DELTA SPEC", "OWNER BYPASS MOVED INTO CanAccessRepo/WhyCanAccess",
"--owner FLAG ON init DELIBERATELY NOT ADDED", "--pick CHANGED FROM
COMMA- TO SPACE-SEPARATED", "--since CHANGED FROM FLEXIBLE TO A STRICT
ALLOW-LIST", "PENDING-INVITE DETECTION (GitHub-only)", "gitcollect sync".
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by SEVEN consecutive sessions (5 through 11). This session
added test files for several other previously-untested cmd/ surfaces
(root.go, member.go, clone.go, sync.go) but list.go itself — now also
carrying the stale-warning logic — is still completely untested at the
cmd-package level. Strongly consider making this its own session.
README.md and docs/index.html WERE updated before this session ended
(initially deferred, then done in the same session once flagged here) —
see the "Listing the repos..." / new "Cloning and keeping repos up to
date" sections in README.md, and the new `sync` entry plus updated
flag descriptions across the command reference in docs/index.html.

Session 12 — 2026-06-30 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
User asked for one feature: `member add`, `group add`, and `add` (repo
add) should each accept multiple values in a single invocation —
several usernames, several group members, or several repos at once —
plus matching tests/coverage and an end-to-end docs/index.html update.
Completed:    `member add <collection> <username> [username...]`,
              `group add <collection> <group> <username> [username...]`,
              and `add <collection> <repo> [repo...]` all changed from
              cobra.ExactArgs to cobra.MinimumNArgs, taking the extra
              positional arguments as a trailing slice. Each command's
              per-item logic was factored out into a new helper
              (addOneMember, addOneToGroup, addOneRepo respectively) so
              the command loops over its values calling the helper,
              collecting failures rather than aborting the whole batch
              on the first one — matching the partial-failure-tolerant
              pattern `sync` already established in session 11, applied
              here for consistency rather than introduced fresh. A
              failed batch is reported as one combined error at the
              end ("member add: 1 of 3 failed: bob (...)") and only
              exits non-zero if at least one item failed; an
              already-satisfied item (already a member, already in the
              group) is still a no-op, not a failure, even inside an
              otherwise-failing batch. `member add` additionally prints
              a `--- username ---` header before each username's block
              when given more than one — its per-item output is
              multi-line (success line + granted/skipped breakdown), so
              without a header multiple users' blocks would run
              together unreadably; `group add` and `add` skip the
              header since their per-item output is a single line each.
              The single-value invocation of all three commands is
              byte-for-byte unchanged from before this session — the
              header only appears when len(values) > 1, and the
              extracted helpers contain exactly the same logic the
              inline code used to.
              `add` has one deliberate asymmetry from the other two:
              repo name format validation (collection.ValidateRepoName)
              runs over every name in the batch up front, before the
              collection is even loaded, so one malformed name is a
              usage error (exit 2) for the whole command rather than a
              partial per-item failure. This matches how usage errors
              work everywhere else in gitcollect (cmd.NewUsageError —
              see the exit-code decision in the decisions log) — a
              malformed argument is a problem with how the command was
              invoked, not a runtime outcome that should vary item by
              item.
              Tests: cmd/member_test.go gained a new shared mock,
              multiAddMock (concurrency-safe, tracks real collaborator
              state and can force one specific username's
              AddCollaborator call to fail) — used by TestAddOneMember
              there and by the two new test files, cmd/group_test.go
              (TestAddOneToGroup) and cmd/add_test.go (TestAddOneRepo).
              Each test covers the same three outcomes the batch loop
              needs to handle correctly: a fresh success, an
              already-satisfied no-op, and a sync/validation failure
              that rolls back cleanly without leaving partial state.
              cmd package coverage: 12.9% → 15.5%. Full
              `go build`/`go vet`/`go test ./...` pass: all packages
              green, no regressions.
              docs/index.html: updated the `add`, `member add`, and
              `group add` command-reference entries for the new
              MinimumNArgs signatures and partial-failure behavior;
              added a new walkthrough subsection, "Bulk operations:
              adding several repos, members, or group members at
              once," between steps 2 and 3, with example output for
              all three commands plus a partial-failure example.
              Every line in that subsection's output blocks was
              verified against real execution before being written —
              a throwaway test in cmd/ called addOneMember/
              addOneToGroup/addOneRepo directly against a real
              in-memory collection.Collection and multiAddMock, with
              its output captured via `go test -v`, then deleted —
              not hand-derived from reading the source, to honor this
              page's existing promise ("every line below is the actual
              output gitcollect prints, not an illustration").
              PROMPT.md (this file): command surface section (`add`/
              `member add`/`group add` lines), a new "gitcollect member
              add / group add / add — multiple values" example block
              in "Command behaviour," file completion table, decisions
              log, and this session log entry. Also caught and fixed a
              stale contradiction left over from session 11's own log
              entry, which said README.md/docs/index.html were "NOT
              updated this session" even though both were, in fact,
              updated later in that same session once the gap was
              flagged — see the correction above session 12's entry.
In progress:  (none)
Blockers:     (none)
Decisions:    See decisions log entry "MULTI-VALUE SUPPORT FOR member
add / group add / add (SESSION 12)".
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by EIGHT consecutive sessions (5 through 12). Still the
oldest, most consistently deferred test-coverage gap in the project.

Session 13 — 2026-07-01 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
Identity migration: replaced mutable login strings in Collection.Owner,
Members, Groups, and RepoAccess.Users with immutable platform user IDs.
GitHub/GitLab logins can be renamed by the account owner at any time —
storing them as the identity token means a rename silently breaks every
ownership and membership check. All design decisions were locked in via
an explicit confirmation exchange before any code was written; see
decisions log entry "SESSION 13, IDENTITY MIGRATION" for the full list.
Completed:    internal/api — UserInfo{ID, Login string} struct; Client
              interface changed GetAuthenticatedUser() to return UserInfo
              and gained GetUser(username string) (UserInfo, error) for
              resolving arbitrary logins to IDs; new ErrUserNotFound
              sentinel distinct from ErrNotFound. github.go: GetUser hits
              GET /users/{username}, maps 404→ErrUserNotFound. gitlab.go:
              GetUser wraps existing lookupUserID. api_test.go: all tests
              updated for UserInfo return type; new TestGitHubGetUser,
              TestGitHubGetUser_NotFound, TestGitLabGetUser,
              TestGitLabGetUser_NotFound.
              internal/collection/collection.go — CurrentVersion "1"→"2";
              new Logins map[string]string yaml field (ID→login cache,
              single source of truth for display and API calls); New()
              takes api.UserInfo instead of string for owner; Validate()
              gates Logins-completeness check on Version == CurrentVersion.
              internal/collection/access.go — new IsOwner(id bool);
              all method parameters renamed to id; FixCmd changed from
              2-param to 3-param (id, login, repo) — login passed
              explicitly because the subject may not be in col.Logins
              (non-member case).
              internal/collection/mutation.go — new helpers: cloneLogins
              (shallow map copy for rollback), resolveUsers (concurrent
              batch GetUser, max 4 in flight), IDForLogin (reverse Logins
              scan, used by remove operations), Migrate (resolves all
              legacy logins to IDs, populates Logins, bumps Version, does
              NOT save — caller decides). All add operations call GetUser
              live; all remove operations use IDForLogin (no network).
              SyncCollaborators uses col.Logins[id] for all API calls.
              internal/collection/collection_test.go — mockClient gained
              GetUser stub; New() calls → api.UserInfo{ID:x, Login:x};
              newTestCollection pre-populates col.Logins for test
              usernames; all ID-based method params updated.
              internal/config/config.go — new Config.UserIDs
              map[string]string (host→id); SaveUserID/LoadUserID; Load()
              initialises UserIDs if nil for backward compat with old
              config files.
              internal/access/enforce.go — parameters renamed to callerID;
              col.IsOwner(callerID) replaces direct string compare;
              col.Logins[col.Owner]/col.Logins[callerID] for all API calls.
              internal/access/inspect.go — UserAccessMap gains id and
              login params (now 3-param: col, id, login); RepoAccessMap/
              FullMatrix resolve IDs to logins via col.Logins for display;
              FixCmd call sites updated for 3-param signature.
              internal/access/access_test.go — GetUser stub added;
              test fixtures updated for ID-based Members/Owner + Logins.
              cmd/root.go — cachedUserID alongside cachedUser;
              currentUserInfo(client) resolves and caches both Login and
              ID, saves both to config; currentUser/currentUserID thin
              wrappers; new loadForOwner(verb, name) returning (col,
              caller, callerID, client, err) — consolidates load+resolve+
              migrate for all 10+ owner-perspective commands; new
              migrateIfNeeded(col, client) — calls Migrate then Save then
              prints a one-time notice; loadForRead→4 return values;
              loadForGit→5 return values; both call migrateIfNeeded;
              new loginsFor(col, ids) display helper (shared across all
              cmd/ files in the package).
              cmd/auth.go — saves both Login and ID to config via
              currentUserInfo. cmd/init.go — passes owner api.UserInfo to
              collection.New. cmd/whoami.go — uses user.Login for display.
              cmd/list.go's roleFor — now format-aware: Version "2" files
              compare cached platform ID (config.LoadUserID); legacy "1"
              files compare cached login (config.LoadUser). list never
              migrates a collection (must stay network-free) so must
              handle both formats indefinitely.
              All remaining cmd/ files (add, delete, visibility, member,
              group, repo, inspect, show, audit, clone, pull, status,
              sync, activity) — updated for 4/5-value loader
              destructuring, col.IsOwner(callerID), col.Logins[col.Owner]
              for display and API calls, loginsFor for member/user ID
              display. audit.go: ZERO changes — Actor/Target stay login
              strings, populated by callers; the audit package itself is
              unchanged.
              cmd/member_test.go, cmd/group_test.go, cmd/add_test.go,
              cmd/show_test.go — api.UserInfo constructors; col.Logins
              population in fixtures; GetUser stubs where needed.
              go build ./... && go test ./...: all 9 packages green,
              no regressions.
In progress:  (none)
Blockers:     (none)
Decisions:    See decisions log entry "SESSION 13, IDENTITY MIGRATION".
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by NINE consecutive sessions (5 through 13). Identity
migration also changed list.go's roleFor (now format-aware: ID
comparison for Version "2", login comparison for "1") — its branching
logic is now more important to cover than ever.

Session 14 — 2026-07-01 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
Documentation and state-audit session. No new functional code written.
Completed:    State verification on session resume: go build ./... and
              go test ./... both clean; confirmed api_test.go and
              collection_test.go were complete (not mid-edit) when the
              prior context window ended.
              PROMPT.md — wrote the missing Session 13 session log entry,
              architecture decisions entry "SESSION 13, IDENTITY
              MIGRATION", and updated 20+ file entries in the file
              completion table; also updated the "next session" pointer
              from "EIGHT" to "NINE consecutive sessions".
              PROGRESS.md — new file (was listed in project structure but
              never created); complete feature inventory from sessions 1-13:
              command groups, session-by-session summary, architecture
              highlights, what was deliberately not built, test coverage.
              README.md — replaced placeholder Installation section with
              five subsections (go install, Download binary, Homebrew,
              Verify the install, Upgrading). Download binary section uses
              exact GoReleaser v2 default archive naming sourced from
              .goreleaser.yaml (gitcollect_<version>_<os>_<arch>[.tar.gz|
              .zip]) and covers Linux/macOS/Windows with curl and
              PowerShell examples. Homebrew marked "coming soon". Added
              separate Shell completion section (cobra's built-in
              completion subcommand; bash/zsh/fish/PowerShell one-liners).
              All facts sourced from go.mod, .goreleaser.yaml,
              cmd/version.go — no invented paths or names.
In progress:  (none)
Blockers:     (none)
Next session should start with: cmd/list_test.go — unchanged pointer,
now named by TEN consecutive sessions (5 through 14). list.go's roleFor
is now format-aware (ID comparison for Version "2", login comparison
for "1") — its branching logic is more important to cover than ever.
```

Session 15 — 2026-07-01 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
PRE_SHIP_IMPROVEMENTS.md Priority 2: removed `repo grant` and
`repo revoke` commands.
Completed:    PRE_SHIP_IMPROVEMENTS Priority 1 (tests from prior
              session context) + Priority 2 in this session.
              Priority 2 changes:
              cmd/repo.go — removed repoGrantCmd, repoRevokeCmd
              var declarations; removed their AddCommand registrations
              from init(); removed runRepoGrant and runRepoRevoke
              functions; removed containsExact helper (was only used
              by those two); removed `errors` import (only used by
              those two); added Long description to repoAccessCmd
              explaining --users as the way to grant/revoke individual
              user access.
              internal/collection/mutation.go — removed ErrRepoOpen
              and ErrRepoWouldOpen sentinel errors; removed
              GrantRepoUser and RevokeRepoUser methods.
              internal/collection/collection_test.go — removed 6 test
              functions: TestGrantRepoUser, TestGrantRepoUser_Refuses
              OnOpenRepo, TestGrantRepoUser_SyncFailureRollsBack,
              TestRevokeRepoUser, TestRevokeRepoUser_RefusesIfIt
              WouldOpenRepo, TestRevokeRepoUser_SyncFailureRollsBack.
              internal/collection/access.go — FixCmd: changed
              "gitcollect repo grant %s %s %s" →
              "gitcollect repo access %s %s --users %s".
              docs/index.html — removed repo grant and repo revoke
              <div class="cmd"> entries.
              README.md — removed repo grant/revoke row from command
              reference table; replaced the paragraph explaining
              grant/revoke guardrails with a note pointing to
              `repo access --users`.
              go build ./... and go test ./... both clean.
              Coverage: 23.3% (up from 22.4%; removed dead code
              outweighed the line count reduction).
Decision:     Removed grant/revoke because they expose a footgun
              (narrowing open repos, reopening restricted ones) as
              a dedicated command pair, creating two user-visible
              failure modes that the --users flag on repo access
              handles more naturally without special-case errors.
              The "guardrails" were essentially compensating for the
              existence of the commands themselves.
In progress:  PRE_SHIP_IMPROVEMENTS Priority 3 — --namespace flag
Blockers:     (none)
Next session should start with: Priority 3 — add Namespace field
to Collection, RepoNamespace() helper, Validate() check, replace
col.Logins[col.Owner] in API calls, --namespace flag in cmd/init.go,
display in cmd/show.go, 4 tests.
```

Session 16 — 2026-07-02 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
PRE_SHIP_IMPROVEMENTS Priorities 3–8 completed. All pure-logic
changes; no new commands added.
Completed:    Priority 3 (namespace fix) — already done at session
              start, completed the one open item (PROMPT.md YAML
              example updated with namespace field and Go struct
              updated to include Logins + Namespace fields).
              Priority 4 — Added "Sharing collections with teammates"
              section to README.md between Shell completion and
              Quickstart: manual cp flow (Option A/B), why YAML
              editing doesn't grant access, coming-soon fetch command.
              Priority 5 — parseSince error message now lists all five
              valid values in duration order ("1h, 24h, 7d, 30d, 90d")
              instead of lexicographic sort; removed `sort` import from
              audit.go; --pick flag usage updated to show quoted form
              (e.g. --pick "r1 r2"); added cmd/audit_test.go with
              TestParseSince_ValidValues and TestParseSince_InvalidValue
              _ListsAllFive; --pick two-value test already covered by
              TestSplitPick (cmd/clone_test.go line 22).
              Priority 6 — Added "### Windows" subsection to README.md
              Installation section with PATH setup steps (7 numbered
              steps) and best-effort support note linking to issues.
              Priority 7 — Added "### Upgrading from an earlier version"
              to README.md Installation section explaining opportunistic
              v1→v2 migration; added gitcollect member list as a
              migration trigger to PROMPT.md architecture note.
              Priority 8 — Added [EXPERIMENTAL] prefix to
              cmd/activity.go's Long description; added [experimental]
              badge + Note to README.md activity command reference row;
              added "Stabilise gitcollect activity" roadmap item to
              README.md.
Decision:     Coverage target (50%) was not reached: final is 23.7%.
              All pure-logic helpers that don't require real auth were
              covered in Priority 1. The remaining ~76% of cmd/ code
              requires live network auth (currentClient, RunE paths for
              all 15+ commands) which cannot be unit-tested without a
              real token. The spec acknowledged this constraint ("Focus
              on pure-logic helpers that don't require real config or
              a running platform API"). 23.7% is the realistic ceiling
              without a testable auth stub or integration test harness.
go build ./... clean; go test ./... all green; go vet ./... clean.
In progress:  (none — all 8 PRE_SHIP priorities complete)
Blockers:     (none)
Next session should start with: Final review pass — verify every
PRE_SHIP_IMPROVEMENTS checklist item against the actual repo state,
run go vet ./..., confirm docs render correctly, and commit.
```

Session 17 — 2026-07-02 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
DOCS_AUDIT_AND_INSTALL.md — all Parts 1, 2, and 3 completed.
Completed:    Part 1 — 8 fixes to existing docs/index.html:
                Fix 1: Added --namespace flag row to gitcollect init
                        command table; added namespace note to init
                        description paragraph.
                Fix 2: Added Namespace row to Philosophy concepts
                        table (after Collection row).
                Fix 3: Added identity model paragraph to Philosophy
                        section (immutable platform user IDs, cached
                        login, security vs. display split).
                Fix 4: Added [experimental] badge (yellow bordered
                        inline span) and italic warning paragraph to
                        gitcollect activity command entry.
                Fix 5: Updated walkthrough step 1 comment to show
                        --namespace usage for org repos.
                Fix 6: Added "fetch coming soon" note after Options
                        A/B in walkthrough step 3.
                Fix 7: Added Windows %USERPROFILE% note below the
                        Philosophy concepts table.
                Fix 8: Fixed all stale relative links (../README.md,
                        ../PROMPT.md) — hero button + footer — to
                        absolute github.com/alby-tomy/gitcollect
                        blob/main/... URLs.
              Part 2 — Replaced thin install section with full
                        Installation section (id="install" kept):
                        Method 1: go install github.com/alby-tomy/
                                  gitcollect@latest (Go 1.26.4+)
                        Method 2: Pre-built binaries — Linux (amd64,
                                  arm64), macOS (arm64, amd64 +
                                  quarantine note), Windows (full
                                  PATH setup steps); all archive
                                  names from .goreleaser.yaml:
                                  gitcollect_${VERSION}_<os>_<arch>
                                  [.tar.gz|.zip].
                        Method 3: Build from source.
                        Method 4: Homebrew (coming soon, marked).
                        Verify: gitcollect version → format from
                                cmd/version.go.
                        Shell completion: bash, zsh (+ oh-my-zsh),
                                fish, powershell — Cobra built-in
                                completion subcommand.
              Part 3 — All 6 accuracy pass checks green:
                        no grant/revoke in HTML ✓
                        no placeholder text ✓
                        namespace in init section ✓
                        identity model paragraph present ✓
                        experimental label on activity ✓
                        footer links fixed ✓
go build ./... not re-run (HTML edits cannot break Go); all green
from Session 16 baseline.
In progress:  (none)
Blockers:     (none)
Next session should start with: commit all changes, push if desired,
and optionally open docs/index.html in a browser to visually verify
the new Installation section and philosophy updates render correctly.
```

Session 18 — 2026-07-02 — Claude Sonnet 4.6
────────────────────────────────────────────────────────────────────
Implemented FEATURE_AUTO_CREATE_REPO.md and FEATURE_SCALABILITY.md
in full, then updated docs/index.html per DOCS_STYLE_GUIDE.md.
Completed:    FEATURE_AUTO_CREATE_REPO:
                internal/api/client.go — CreateRepo(owner, name string,
                  private bool, description string) (RepoInfo, error)
                  added to Client interface; ErrNameConflict sentinel.
                internal/api/github.go — CreateRepo: GET /user to
                  resolve owner login; POST /user/repos for personal
                  or POST /orgs/{owner}/repos for org repos; 422 →
                  ErrNameConflict, 403/404 → ErrForbidden.
                internal/api/gitlab.go — CreateRepo: POST /projects
                  with namespace_path; 409 → ErrNameConflict.
                internal/api/api_test.go — 6 new tests covering
                  GitHub personal/org/conflict/forbidden and GitLab
                  ok/conflict paths.
                cmd/add.go — ensureRepoExists helper: on TTY, prompts
                  to create a missing repo (non-TTY skips silently);
                  uses ErrNameConflict as idempotent success;
                  --new-repo-visibility flag (default "private");
                  addOneRepo signature updated with callerID + private
                  params.
                cmd/add_test.go — 10 new tests: non-TTY skip, conflict
                  treated as success, create fails propagates error,
                  private/public flag variants, audit entry on create.
                All mock clients updated to implement CreateRepo stub.
              FEATURE_SCALABILITY:
                internal/collection/collection.go — GroupAdminsEnabled
                  bool field (after Visibility) + GroupAdmins
                  map[string][]string (after Groups); Validate() checks
                  every group admin ID is in Members.
                internal/collection/access.go — sentinel errors
                  ErrGroupAdminsDisabled, ErrWrongGroup, ErrSelfTransfer,
                  ErrAdminPrivilegeEscalation; IsGroupAdmin(callerID,
                  group), CanManageGroup(callerID, group),
                  GroupAdminOf(callerID) methods.
                internal/collection/mutation.go — RemoveMember also
                  removes from GroupAdmins; DeleteGroup also removes
                  from GroupAdmins.
                cmd/transfer.go (new) — full transfer command with
                  ErrSelfTransfer guard, member-only check, GroupAdminOf
                  check, typed ConfirmWord, previousOwner→Members,
                  newOwner removed from Members, Save, audit.
                cmd/transfer_test.go (new) — 8 tests covering requires-
                  owner, self-transfer, requires-member, confirm-abort,
                  typed-confirm, previous-owner-in-members, audit,
                  removeStringSlice unit test.
                cmd/scale.go (new) — scale organisation|team with
                  idempotent tier switching, admin-list confirmation
                  on downgrade, audit.
                cmd/scale_test.go (new) — 7 tests covering invalid
                  tier, requires-owner, enable, already-enabled,
                  disable-no-admins, disable-with-admins-aborts,
                  already-disabled, audit field constants.
                cmd/group.go — runGroupAdd/runGroupRemove switched
                  from requireOwner to loadForOwner + CanManageGroup
                  check (group admins of the specific group can now
                  run these); group admin add/remove/list subcommands
                  added under new groupAdminCmd.
                cmd/group_test.go — 19 new auth matrix tests covering
                  non-owner/disabled, group-admin-correct-group (✓),
                  group-admin-wrong-group (✗), regular-member (✗), and
                  all group admin add/remove/list paths.
                cmd/show.go — GROUPS table gains ADMINS column when
                  GroupAdminsEnabled=true.
                cmd/init.go — opt-in org tier prompt on TTY (uses
                  output.Confirm; no-op on non-TTY).
              STEP 3 — docs/index.html:
                init: note about opt-in org prompt.
                add: description updated for auto-create; flags table
                  added with --new-repo-visibility row.
                transfer: new cmd-block in collection lifecycle group.
                scale: new cmd-block in collection lifecycle group.
                group add/remove: descriptions updated for group admin
                  delegation.
                group admin add/remove/list: three new cmd-blocks in
                  group management group.
              STEP 4 — Final checks:
                go build ./... clean; go test ./... all green;
                go vet ./... clean.
                cmd coverage: 33.1%.
In progress:  (none)
Blockers:     (none)
Next session should start with: git commit all changes from this
session (FEATURE_AUTO_CREATE_REPO + FEATURE_SCALABILITY + docs).
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

cmd/root.go                                  done         +ErrUnauthorized hint in Execute(), session 6;
                                                           +Levenshtein typo suggestion in loadCollection
                                                           (owner-required path only, NOT loadForRead's
                                                           private-collection non-disclosure path), session 11;
                                                           +currentUserInfo (caches Login+ID, saves both);
                                                           +loadForOwner (load+resolve+migrate for all owner
                                                           commands); +migrateIfNeeded; +loginsFor; loadForRead
                                                           →4-val, loadForGit→5-val; cachedUserID, session 13
cmd/root_test.go                             done         new, session 11 — TestLevenshtein, TestSuggestCollectionName, TestStaleDays
cmd/auth.go                                  done         saves both Login and ID to config via
                                                           currentUserInfo, session 13
cmd/whoami.go                                done         +anyRejected hint, session 6; +--json flag, session 11;
                                                           uses user.Login for display, session 13
cmd/init.go                                  done         no --owner flag — considered and deliberately
                                                           rejected in session 11, see decisions log;
                                                           passes api.UserInfo to collection.New, session 13;
                                                           +opt-in org tier prompt on TTY (output.Confirm;
                                                           no-op on non-TTY), session 18
cmd/delete.go                                done
cmd/list.go                                  done         redesigned session 4 — see decisions log;
                                                           +stale-collection warning (>30 days since
                                                           updated_at), session 11; roleFor now format-aware:
                                                           Version "2" compares cached platform ID, legacy "1"
                                                           compares cached login, session 13
cmd/show.go                                  done         +per-caller YOU column, session 9; +owner-only
                                                           WHO HAS ACCESS view, +FixCmd-driven footer for
                                                           denied repos, +stale warning, session 11;
                                                           callerID-based owner check; loginsFor for
                                                           Members/Users display, session 13;
                                                           +ADMINS column in GROUPS table when
                                                           GroupAdminsEnabled=true, session 18
cmd/show_test.go                             done         new, session 9; +TestBuildOwnerShowRepoRows,
                                                           updated deniedRepo/FixCmd assertions, session 11;
                                                           api.UserInfo constructors; col.Logins, session 13
cmd/visibility.go                            done
cmd/transfer.go                              done         new, session 18 — transfer command with ErrSelfTransfer
                                                           guard, member-only check, GroupAdminOf check,
                                                           typed ConfirmWord, previousOwner→Members,
                                                           newOwner removed from Members; removeStringSlice helper
cmd/transfer_test.go                         done         new, session 18 — 8 tests
cmd/scale.go                                 done         new, session 18 — scale organisation|team with
                                                           idempotent tier switching + admin-list confirmation
                                                           on downgrade
cmd/scale_test.go                            done         new, session 18 — 7 tests
cmd/add.go                                   done         ExactArgs(2) → MinimumNArgs(2): accepts multiple
                                                           repo names, session 12 — see decisions log; logic
                                                           split into addOneRepo so a batch continues past
                                                           one repo's failure; +ensureRepoExists (TTY prompt
                                                           to create missing repo), +--new-repo-visibility
                                                           flag, session 18
cmd/add_test.go                              done         new, session 12 — TestAddOneRepo; api.UserInfo
                                                           constructor; col.Logins["bob"] fixture, session 13;
                                                           +10 auto-create tests, session 18
cmd/remove.go                                done         y/N → type-name-to-confirm, session 8
cmd/repo.go                                  done         grant/revoke subcommands added session 3
cmd/member.go                                done         +pending-invite warning after member add,
                                                           session 11 (Client.GetPendingInvite); ExactArgs(2)
                                                           → MinimumNArgs(2): accepts multiple usernames,
                                                           session 12 — see decisions log; logic split into
                                                           addOneMember so a batch continues past one
                                                           username's failure
cmd/member_test.go                           done         new, session 11 — TestHasPendingInvite; +multiAddMock
                                                           (shared with add_test.go/group_test.go) and
                                                           TestAddOneMember, session 12; GetUser stub;
                                                           api.UserInfo constructors, session 13
cmd/group.go                                 done         ExactArgs(3) → MinimumNArgs(3) on group add: accepts
                                                           multiple usernames, session 12 — see decisions log;
                                                           logic split into addOneToGroup so a batch continues
                                                           past one username's failure; runGroupAdd/runGroupRemove
                                                           auth changed from requireOwner to loadForOwner +
                                                           CanManageGroup (group admins of that specific group
                                                           can now also run these); group admin add/remove/list
                                                           subcommands added under groupAdminCmd, session 18
cmd/group_test.go                            done         new, session 12 — TestAddOneToGroup;
                                                           api.UserInfo constructor; col.Logins
                                                           pre-populated in fixture, session 13;
                                                           +19 auth matrix tests (group add/remove auth,
                                                           group admin add/remove/list), session 18
cmd/inspect.go                               done         +"To fix:" footer via Collection.FixCmd, session 11
cmd/audit.go                                 done         --since: flexible parsing → strict 1h/24h/7d/30d/90d
                                                           allow-list, session 11 — see decisions log
cmd/activity.go                              done         new command, session 7 — not in original spec;
                                                           --since help text updated to match audit.go's
                                                           strict allow-list, session 11
cmd/activity_test.go                         done         session 7; cmd/'s first ever test file (6.5% pkg cov)
cmd/clone.go                                 done         --pick: comma-separated StringSlice → space-
                                                           separated StringArray + splitPick, session 11;
                                                           +pending-invite warning on skipped repos, session 11
cmd/clone_test.go                            done         new, session 11 — TestSplitPick, TestFirstPendingInvite
cmd/pull.go                                  done         fixed --pick suggestion to space-join after
                                                           session 11's --pick flag change (was comma-join,
                                                           would have produced a non-functional suggestion)
cmd/status.go                                done
cmd/sync.go                                  done         new command, session 11 — not in original spec;
                                                           clones missing repos + pulls existing ones in one
                                                           pass; reuses clone.go's cloneOne/selectCloneTargets
cmd/sync_test.go                             done         new, session 11 — TestFormatSyncLine
cmd/version.go                               done
(shell completion: cobra's built-in `completion` subcommand covers this
 — no separate file needed; verified with `gitcollect completion --help`)

internal/collection/collection.go            done         +Logins map[string]string yaml field (ID→login cache);
                                                           CurrentVersion "1"→"2"; New() takes api.UserInfo
                                                           (not string) for owner; Validate() gates
                                                           Logins-completeness check on Version "2", session 13;
                                                           +GroupAdminsEnabled bool + GroupAdmins
                                                           map[string][]string; Validate() checks group admin
                                                           IDs are in Members, session 18
internal/collection/access.go                done         groupsContaining (dead code) removed; owner-bypass
                                                           moved INTO CanAccessRepo/WhyCanAccess directly
                                                           (was scattered across callers), session 11 — see
                                                           decisions log; +FixCmd, session 11; +IsOwner(id);
                                                           all params renamed to id; FixCmd 3-param
                                                           (id, login, repo), session 13;
                                                           +ErrGroupAdminsDisabled, ErrWrongGroup,
                                                           ErrSelfTransfer, ErrAdminPrivilegeEscalation;
                                                           +IsGroupAdmin, CanManageGroup, GroupAdminOf, session 18
internal/collection/mutation.go              done         +GrantRepoUser/RevokeRepoUser, session 3;
                                                           +Migrate, IDForLogin, resolveUsers, cloneLogins;
                                                           add ops call GetUser live; remove ops use
                                                           IDForLogin (no network); SyncCollaborators uses
                                                           col.Logins[id] for all API calls, session 13;
                                                           RemoveMember also removes from GroupAdmins;
                                                           DeleteGroup also removes from GroupAdmins, session 18
internal/collection/collection_test.go       done         83.8% coverage; mock +ListCommits stub, session 7;
                                                           +GetPendingInvite stub, session 11; WhyCanAccess
                                                           assertions updated for new reason strings, session 11;
                                                           +GetUser stub; New()→api.UserInfo{...};
                                                           newTestCollection pre-populates col.Logins;
                                                           all ID-based method params updated, session 13

internal/access/enforce.go                   done         redundant owner checks in CheckRepoAccess/
                                                           FilterAccessible removed now that CanAccessRepo
                                                           does the bypass itself, session 11; callerID params;
                                                           col.IsOwner(callerID)/col.Logins for comparisons
                                                           and API calls, session 13
internal/access/sync.go                      done
internal/access/inspect.go                   done         +decide() owner-bypass fix, session 9 — see decisions
                                                           log; decide() helper REMOVED session 11 (redundant
                                                           after the CanAccessRepo refactor); +FixCmd field on
                                                           RepoAccessDetail/MemberAccessDetail, session 11;
                                                           UserAccessMap→3-param (col, id, login); col.Logins
                                                           for all display, session 13
internal/access/access_test.go               done         93.9% coverage; +TestUserAccessMap_OwnerBypass,
                                                           session 9 (assertions updated for new owner reason
                                                           string, session 11); +GetPendingInvite stub,
                                                           session 11; +GetUser stub; fixtures updated for
                                                           ID-based Members/Owner + Logins maps, session 13

internal/audit/audit.go                      done
internal/audit/audit_test.go                 done         82.8% coverage

internal/activity/activity.go                done         new package, session 7 — not in original spec
internal/activity/activity_test.go           done         84.4% coverage, session 7

internal/git/git.go                          done         +PullWithSummary (reports new-commit count for
                                                           sync's per-repo reporting), session 11
internal/git/git_test.go                     done         85.4% coverage; +TestPullWithSummary_* (3 cases,
                                                           new fake-git harness with per-call varying output), session 11

internal/api/client.go                       done         +ListCommits, +CommitInfo, +RepoInfo.DefaultBranch,
                                                           session 7; +GetPendingInvite, +GitHubNotificationsURL
                                                           const, session 11; +UserInfo{ID,Login} struct,
                                                           GetUser(username) (UserInfo, error),
                                                           ErrUserNotFound, session 13;
                                                           +CreateRepo(owner, name, private, description)
                                                           (RepoInfo, error); +ErrNameConflict, session 18
internal/api/github.go                       done         githubBaseURL: const → var (see decisions log);
                                                           +ListCommits, session 7; +GetPendingInvite
                                                           (GET /repos/{owner}/{repo}/invitations), session 11;
                                                           +GetUser (GET /users/{username}), GetAuthenticatedUser
                                                           now returns UserInfo, session 13;
                                                           +CreateRepo (GET /user for login, then POST
                                                           /user/repos or /orgs/{owner}/repos), session 18
internal/api/gitlab.go                       done         +ListCommits, session 7; +GetPendingInvite stub
                                                           (always false — GitLab has no pending-invite state;
                                                           membership added via API is immediate), session 11;
                                                           +GetUser (wraps existing lookupUserID), session 13;
                                                           +CreateRepo (POST /projects), session 18
internal/api/api_test.go                     done         85.5% coverage; +ListCommits/DefaultBranch tests,
                                                           session 7; +TestGitHub/GitLabGetPendingInvite*,
                                                           session 11; all tests updated for UserInfo return
                                                           type; +TestGitHub/GitLabGetUser(_NotFound), session 13;
                                                           +6 CreateRepo tests (GitHub personal/org/conflict/
                                                           forbidden + GitLab ok/conflict), session 18

internal/config/config.go                    done         +ActivityDir(), session 7; +UserIDs
                                                           map[string]string (host→id), SaveUserID/LoadUserID;
                                                           Load() inits UserIDs if nil for backward compat,
                                                           session 13
internal/config/config_test.go               done         82.5% coverage (was 82.8%; +ActivityDir assertion, session 7)

internal/output/output.go                    done         Table/padRight: byte len → rune count (real bug fix);
                                                           +StaleWarning, +InviteWarning, session 11
internal/output/output_test.go               done         98.1% coverage; +TestStaleWarning, +TestInviteWarning, session 11

README.md                                    done         +Installation (go install, Download binary, Homebrew,
                                                           Verify, Upgrading) and Shell completion sections,
                                                           session 14; prior updates: activity section s7,
                                                           list/show/sync sections s9-s12
PROGRESS.md                                  done         new file, session 14 — full feature inventory
                                                           sessions 1-13
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
- SESSION 11, PROMPT_v2.md TREATED AS A DELTA SPEC, NOT A REWRITE: a
  second spec file, PROMPT_v2.md, was dropped into the project root with
  its own from-scratch PROGRESS TRACKER (every file marked "todo",
  Session 1 blank) and an instruction implying the codebase needed to be
  regenerated file-by-file from nothing. The actual codebase was already
  10 sessions in and fully working. Rather than blindly follow the
  literal instruction (which would have meant regenerating already-
  correct, already-tested files and risked silently reverting 10
  sessions of refinements), this was flagged to the user, who confirmed:
  treat PROMPT_v2.md as a delta spec against the existing implementation
  — diff it against current behavior, implement only what's genuinely
  new or changed, and keep tracking progress in PROMPT.md (this file),
  not PROMPT_v2.md's own tracker. PROMPT_v2.md's tracker was deliberately
  never updated/written to — it isn't live and was never meant to be;
  this was reconfirmed once more mid-session when the same "previous
  session was interrupted, resume from PROMPT_v2.md's tracker" framing
  came up a second time. PROMPT_v2.md itself is left in the repo as a
  historical reference for what the delta was sourced from, not as an
  active spec — if you're starting a new session and see it, this entry
  is the explanation; do not restart a from-scratch build off of it.
- SESSION 11, OWNER BYPASS MOVED INTO CanAccessRepo/WhyCanAccess: every
  caller previously implemented its own "if caller == col.Owner, allow"
  check on top of CanAccessRepo (enforce.go's CheckRepoAccess/
  FilterAccessible, inspect.go's now-removed decide() helper) — a
  layered design chosen deliberately in session 9 specifically so
  CanAccessRepo's return value stayed a pure function of the documented
  member/group/user rules, with the bypass living only at the few call
  sites that needed it. PROMPT_v2.md's spec puts the bypass inside
  CanAccessRepo itself instead. Presented both options to the user
  (keep the session-9 layered approach vs. move the bypass inboard);
  user explicitly chose the more invasive inboard refactor. CanAccessRepo
  now checks `username == c.Owner` first, unconditionally, before the
  public-visibility check; WhyCanAccess mirrors this and returns
  "owner — full access" (was bare "owner"). Every downstream caller's
  redundant copy of the bypass was then deleted — this eliminates an
  entire class of "forgot to bypass for the owner at some new call site"
  bugs at the source instead of requiring each new caller to remember to
  add it. Tradeoff accepted: CanAccessRepo is no longer a pure function
  of the member/group/user rules alone; "is this person the owner" is
  now baked into the access-control primitive itself, not layered on top.
- SESSION 11, --owner FLAG ON `init` DELIBERATELY NOT ADDED: PROMPT_v2.md
  proposes `gitcollect init <name> --owner <org>` for org-owned
  collections. Not implemented. Every owner-only check in the codebase
  (CanAccessRepo's bypass, member/group mutation guards, `show`'s
  owner-view detection) is a literal `caller == col.Owner` string
  comparison against the authenticated user's own login. An org name can
  never equal an authenticated personal user's login, so an org-owned
  collection could never pass a single owner-only check — every
  org-collection's true administrators would be permanently locked out
  of their own collection's owner-only commands (delete, repo access
  changes, member/group management, etc.) the moment --owner pointed at
  an org instead of themselves. Fixing this properly would require a
  real authorization model (e.g. "owner OR member of org's admin team"),
  which is a substantially bigger change than this flag implies. Flagged
  to the user as an architectural conflict rather than implemented as a
  flag that would silently produce an unusable collection; user agreed
  to skip it for now.
- SESSION 11, --pick CHANGED FROM COMMA- TO SPACE-SEPARATED: matching
  PROMPT_v2.md's examples (`--pick repo1 repo2`) exactly, per explicit
  user choice over keeping the original comma-separated StringSliceVar.
  Implemented as StringArrayVar (so cobra doesn't itself comma-split
  values) plus a manual splitPick() that runs strings.Fields() over each
  raw value — this supports both a single quoted value with embedded
  spaces (`--pick "r1 r2"`) and the flag repeated (`--pick r1 --pick
  r2"`), but does NOT support genuinely bare unquoted multi-token
  picks as two separate positional-looking arguments outside of pflag's
  own repeated-flag or quoted-value mechanisms — pflag has no built-in
  concept of a flag consuming a variable number of bare following
  arguments. cmd/pull.go's "repos not cloned, run clone --pick ..."
  suggestion was comma-joining the missing list before this change and
  needed a matching fix (space-join) — caught and fixed in the same
  session; comma-joined output would have silently produced a
  suggestion the new splitPick() couldn't parse correctly.
- SESSION 11, --since CHANGED FROM FLEXIBLE TO A STRICT ALLOW-LIST:
  `audit --since` and `activity --since` previously accepted anything
  time.ParseDuration understood (e.g. "2h30m") plus an ad hoc "Nd" shorthand
  for days. PROMPT_v2.md's spec only documents five exact values: 1h, 24h,
  7d, 30d, 90d. Per explicit user choice ("match v2 exactly" over keeping
  the more flexible parser), --since now rejects anything outside that
  five-value set, including previously-valid inputs like "30m" or "2h30m".
  The five values map to a fixed `sinceDurations` lookup table rather than
  being parsed, so there's no ambiguity about what's accepted.
- SESSION 11, PENDING-INVITE DETECTION (GitHub-only): GitHub's
  AddCollaborator creates a pending, unaccepted invitation unless the
  invitee already has access on the repo — CheckCollaborator returns
  false for someone with a pending invite exactly the same as it does for
  someone with no access at all, so without this, `member add` and
  `clone` had no way to tell "you're not entitled" apart from "you're
  entitled but haven't accepted the GitHub email yet." Added
  Client.GetPendingInvite(owner, repo, username) — GitHub hits
  GET /repos/{owner}/{repo}/invitations and checks invitee logins; GitLab
  always returns (false, nil) since GitLab project membership added via
  API is immediate, with no pending/unaccepted state to query. Wired into
  member.go (warns after granting access if any newly-granted repo shows
  a pending invite) and clone.go (warns when a "skipped" repo turns out to
  be a pending invite rather than a genuine local-rule denial).
- SESSION 11, `gitcollect sync` (new command, not in the original spec):
  combines clone + pull into one pass — for every accessible repo, clones
  it if absent at --dest, pulls it if already present. Reuses clone.go's
  cloneOne/selectCloneTargets rather than duplicating the clone-URL-
  resolution logic. Added git.PullWithSummary(dir) to internal/git/git.go
  specifically to support sync's per-repo "N new commits" vs "up to date"
  reporting — it compares `git rev-parse HEAD` before/after the pull and,
  if it changed, runs `git rev-list --count old..new` to report exactly
  how many commits arrived. Concurrent like clone (bounded by
  --concurrency, default 4), unlike the existing sequential pull.go —
  this was a deliberate choice for the new command, not a retrofit onto
  pull.go, since pull.go's existing sequential behavior wasn't reported as
  a problem and changing it wasn't part of this session's approved scope.
- SESSION 12, MULTI-VALUE SUPPORT FOR member add / group add / add: user
  asked for `member add`, `group add`, and `add` (repo add) to each accept
  multiple values in one invocation instead of exactly one. Implemented as
  cobra.ExactArgs → cobra.MinimumNArgs on all three, with the per-item logic
  factored into addOneMember/addOneToGroup/addOneRepo and the command itself
  reduced to a loop over its values plus failure collection — the same
  partial-failure-tolerant shape session 11's `sync` already used (process
  every item, collect failures, report them together, exit non-zero only if
  something actually failed), reused here rather than inventing a different
  batch semantics for these three commands.
  Two things worth remembering if this pattern gets extended to more
  commands later (member remove, group remove, repo grant/revoke would be
  the natural next candidates, but were NOT done this session — out of
  scope, not requested):
    - The single-value call path must stay byte-for-byte identical to
      before. This was achieved by keeping the extracted per-item helper's
      logic an exact copy of what used to be inline in the command, and by
      making any new "multiple items" UI (the `--- username ---` header in
      member add) conditional on len(values) > 1. Do NOT let a refactor like
      this change single-item output even slightly — there are three
      sessions' worth of docs/index.html example blocks whose stated
      contract is "every line is the actual output gitcollect prints, not
      an illustration," and single-item invocations are still the common
      case.
    - `add` validates every repo name's format up front, before touching
      the collection at all, so one malformed name fails the whole command
      as a usage error (exit 2) rather than as one partial failure among
      many. `member add` and `group add` do NOT do this — a malformed
      username there only fails that one username (ValidateUsername runs
      inside AddMember/AddToGroup, after the loop has already started, so
      it naturally becomes a per-item failure like any other). This
      asymmetry is intentional, not an oversight: a malformed repo name is
      knowable immediately from the argument alone, the same category of
      problem cobra's own Args validators would catch if they could; a
      malformed username is comparatively rare and this codebase already
      had no precedent for pre-validating usernames before session 12, so
      changing member add/group add's behavior here would have been an
      unrequested behavior change beyond what was asked.
  Verification note: every example output block added to docs/index.html
  for this feature was checked against a real run (a throwaway Go test
  calling the extracted helpers directly against a real in-memory
  collection.Collection and a real, if mocked, api.Client, with output
  captured via `go test -v` and the test file deleted afterward) rather
  than typed from reading the source — this page has stated since it was
  written that its output blocks are real, not illustrations, and that
  claim needed to keep holding for the new content too.
- SESSION 13, IDENTITY MIGRATION — immutable platform IDs replacing
  mutable usernames: GitHub/GitLab logins can be renamed by the account
  owner at any time. Storing them in Collection.Owner, Members, Groups,
  and RepoAccess.Users means a rename silently breaks every ownership
  and membership check. Fixed by storing the immutable numeric platform
  user ID (formatted as a decimal string) in those fields and keeping a
  separate Logins map[string]string (ID→login) as the single source of
  truth for display and API path-building (REST paths use login strings).
  Design decisions locked in before any code was written:
    - ID used ONLY for comparisons: IsOwner, IsMember, IsInGroup,
      CanAccessRepo. Login used for: all API calls, all user-facing
      display, audit log Actor/Target, FixCmd suggestions.
    - Version field bumped "1"→"2" in collection YAML to detect legacy
      files. Validate() only checks Logins-completeness on Version "2"
      files; legacy "1" files pass the old checks unchanged.
    - Migration is OPPORTUNISTIC: Migrate() is called only from
      loadForOwner, loadForGit, and loadForRead's private branch — never
      from list (must stay network-free) or loadForRead's public fast-
      path (must stay auth-free). A Version "2" file triggers no
      migration call at all. `gitcollect member list` also triggers
      migration (it uses loadForOwner when the caller is the owner),
      making it a useful manual upgrade path for owners.
    - Add operations (AddMember, AddToGroup, GrantRepoUser,
      SetRepoAccess --users) always call client.GetUser() live — the
      account must exist on the platform before access is granted.
    - Remove operations (RemoveMember, RemoveFromGroup, RevokeRepoUser)
      use IDForLogin() reverse-lookup from the cached Logins map — no
      network call; works even for renamed or deleted platform accounts.
    - FixCmd changed from 2-param to 3-param (id, login, repo): the
      subject may not be in col.Logins (non-members have no entry), so
      login is passed explicitly by callers rather than looked up.
    - loadForOwner(verb, name) consolidates the repeated load+resolve+
      migrate boilerplate from 10+ owner-perspective cmd/ files into one
      helper returning (col, caller, callerID, client, err).
    - cmd/list.go's roleFor is format-aware: Version "2" compares the
      cached platform ID (config.LoadUserID); legacy "1" compares the
      cached login (config.LoadUser). list never migrates a collection
      (no client, no network) so must handle both formats indefinitely.
    - audit.go: ZERO changes. Actor/Target stay login strings, populated
      by callers. The audit package itself is unchanged.
```

<!-- v1 -->
