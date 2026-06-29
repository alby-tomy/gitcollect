# gitcollect — Master Build Prompt

> Paste this entire file into Claude (or any capable LLM) to generate the
> complete gitcollect codebase. Every section is intentional — do not skip,
> summarise, or reorder any part. Read the PROGRESS TRACKER at the bottom
> before writing a single line of code, and update it before ending the session.

---

## Role & mindset

You are a senior Go engineer with 8+ years shipping production CLI tools used
by thousands of developers. You prioritise correctness over cleverness,
explicit over implicit, and user clarity over code brevity. You think about
failure modes before happy paths.

The most important principle in this codebase:
gitcollect's local YAML is a declaration of intent — it describes who should
have access. The GitHub/GitLab platform is the enforcement point — it is
where access is actually granted or revoked. These two must never diverge.
Every access mutation must drive the platform API to completion before the
local YAML is written. If the API call fails, the YAML does not change.
There is no in-between state. No shadow permission system.

---

## What this tool is

gitcollect is a standalone Go CLI that lets developers group GitHub/GitLab
repositories into named collections and control who can access them — at
both the collection level (who is a member) and the repo level (which groups
or individuals can reach which repos).

It does not replace Git. It wraps Git and the GitHub/GitLab APIs to add the
grouping and access-control layer that neither platform provides natively.
When gitcollect grants or revokes access, it does so by calling the real
GitHub/GitLab collaborator APIs — not by maintaining a shadow list.

---

## Complete command surface

```
── Authentication ──────────────────────────────────────────────────────────
gitcollect auth                               Store GitHub token (hidden prompt)
gitcollect auth --host gitlab.com             Authenticate a GitLab instance
gitcollect whoami                             Show authenticated user per host
gitcollect whoami --json                      Machine-readable output

── Collection lifecycle ────────────────────────────────────────────────────
gitcollect init <name>                        Create collection (default: private)
gitcollect init <name> --owner <org>          Create for a GitHub org (not just self)
gitcollect init <name> --public               Create as public
gitcollect init <name> --description "..."    Add description
gitcollect delete <collection>                Delete collection + revoke all access
gitcollect list                               List collections (owned + member)
gitcollect list --all                         Include all private if owner
gitcollect list --public                      Show only public collections
gitcollect list --private                     Show only private collections
gitcollect list --json                        Machine-readable output
gitcollect show <collection>                  Full summary with per-user access column
gitcollect show <collection> --json           Machine-readable output
gitcollect visibility <collection> public|private   Change visibility

── Repo management ─────────────────────────────────────────────────────────
gitcollect add <collection> <repo>            Add repo (default: open to all members)
gitcollect remove <collection> <repo>         Remove repo + revoke platform access
gitcollect repo access <collection> <repo> --groups g1,g2   Restrict to groups
gitcollect repo access <collection> <repo> --users u1,u2    Restrict to individuals
gitcollect repo access <collection> <repo> --open           Open to all members
gitcollect repo show <collection> <repo>      Show who can access this repo and why

── Member management ───────────────────────────────────────────────────────
gitcollect member add <collection> <username>             Add member
gitcollect member remove <collection> <username>          Remove member + revoke access
gitcollect member remove <collection> <username> --confirm-self  Remove yourself
gitcollect member list <collection>                       List members + group memberships

── Group management ────────────────────────────────────────────────────────
gitcollect group create <collection> <group>              Create a group
gitcollect group delete <collection> <group>              Delete (blocked if repos use it)
gitcollect group add <collection> <group> <user>          Add member to group
gitcollect group remove <collection> <group> <user>       Remove member from group
gitcollect group list <collection>                        List groups + members
gitcollect group show <collection> <group>                Show group members + repos

── Access inspection ───────────────────────────────────────────────────────
gitcollect inspect <collection>                           Full member x repo matrix
gitcollect inspect <collection> --user <username>         Full access map for one user
gitcollect inspect <collection> --repo <repo>             Who can access one repo + why
gitcollect inspect <collection> --json                    Machine-readable output

── Audit trail ─────────────────────────────────────────────────────────────
gitcollect audit <collection>                 Show access change log (newest first)
gitcollect audit <collection> --user <u>      Filter by actor or target username
gitcollect audit <collection> --since 7d      Filter by time: 1h | 24h | 7d | 30d | 90d
gitcollect audit <collection> --json          Machine-readable output

── Git operations ──────────────────────────────────────────────────────────
gitcollect clone <collection>                 Clone all accessible repos
gitcollect clone <collection> --pick r1 r2   Clone selected repos (space-separated)
gitcollect clone <collection> --dry-run       Preview without executing
gitcollect clone <collection> --concurrency 8 Parallel limit (default: 4)
gitcollect clone <collection> --dest <dir>   Clone into specific directory
gitcollect pull <collection>                  git pull inside all cloned repos
gitcollect pull <collection> --dest <dir>    Directory repos were cloned into
gitcollect status <collection>                git status inside all repos
gitcollect status <collection> --dest <dir>  Directory repos were cloned into
gitcollect sync <collection>                  Clone missing + pull existing in one pass
gitcollect sync <collection> --dest <dir>    Directory to clone/pull into
gitcollect sync <collection> --dry-run        Preview without executing
gitcollect sync <collection> --concurrency 8  Parallel limit (default: 4)

── System ──────────────────────────────────────────────────────────────────
gitcollect version                            Print version + platform
gitcollect completion bash|zsh|fish|powershell  Shell completion script
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
│   ├── sync.go              # sync = clone missing + pull existing
│   └── version.go
├── internal/
│   ├── collection/
│   │   ├── collection.go        # Collection struct, Load, Save, Validate
│   │   ├── access.go            # IsMember, IsOwner, CanAccessRepo, WhyCanAccess
│   │   ├── mutation.go          # AddMember, RemoveMember, AddToGroup, etc.
│   │   └── collection_test.go
│   ├── access/
│   │   ├── enforce.go           # CheckCollectionAccess, CheckRepoAccess
│   │   ├── sync.go              # SyncCollaborators — drives platform API
│   │   ├── inspect.go           # UserAccessMap, RepoAccessMap, FullMatrix
│   │   └── access_test.go
│   ├── audit/
│   │   ├── audit.go             # AuditLog, Append, Read, Filter
│   │   └── audit_test.go
│   ├── git/
│   │   ├── git.go               # Clone, Pull, Status, Sync wrappers
│   │   └── git_test.go
│   ├── api/
│   │   ├── client.go            # Client interface + NewClient factory
│   │   ├── github.go            # GitHub implementation
│   │   ├── gitlab.go            # GitLab implementation
│   │   └── api_test.go
│   ├── config/
│   │   ├── config.go            # Token storage, paths via os.UserHomeDir()
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

## CRITICAL: Windows compatibility

All file paths MUST use os.UserHomeDir() — never hardcode Unix-style paths.

```go
// CORRECT — works on Windows, Mac, Linux
func CollectionsDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("cannot find home directory: %w", err)
    }
    return filepath.Join(home, ".gitcollect", "collections"), nil
}

// WRONG — breaks on Windows
func CollectionsDir() string {
    return "~/.gitcollect/collections"
}
```

Use filepath.Join() everywhere. Never string-concatenate paths with "/".

```
Config directory:   filepath.Join(home, ".gitcollect")
Collections:        filepath.Join(home, ".gitcollect", "collections")
Audit logs:         filepath.Join(home, ".gitcollect", "audit")
Auth config:        filepath.Join(home, ".gitcollect", "config")
```

---

## Data model — complete YAML format

```yaml
# <home>/.gitcollect/collections/cybersecurity.yaml
version: "1"
name: cybersecurity
description: "Penetration testing and security research tools"
host: github.com
owner: yourusername        # set on init, defaults to authenticated user
visibility: private        # public | private
created_at: "2025-01-15T10:00:00Z"
updated_at: "2025-01-20T14:32:00Z"

# Members: everyone who belongs to this collection.
# The owner is NOT automatically a member — they must add themselves
# explicitly with "gitcollect member add <collection> <owner-username>"
# if they want to appear in the members list.
# The owner always has full access regardless of membership status.
# This is intentional — owner and membership are separate concepts.
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

# Repos with per-repo access control.
# Access rules are UNIONED — user needs to satisfy ANY ONE of:
#   groups=[] AND users=[] → open to all members
#   user in any listed group → allowed
#   username in users list → allowed individually
repos:
  - name: pen-test-tools
    groups: []
    users: []               # open to all members

  - name: vuln-scanner
    groups: [red-team]
    users: []               # only red-team (alice, bob)

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

type RepoAccess struct {
    Name   string   `yaml:"name"`
    Groups []string `yaml:"groups"`
    Users  []string `yaml:"users"`
}

type Collection struct {
    Version     string              `yaml:"version"`
    Name        string              `yaml:"name"`
    Description string              `yaml:"description"`
    Host        string              `yaml:"host"`
    Owner       string              `yaml:"owner"`
    Visibility  Visibility          `yaml:"visibility"`
    Members     []string            `yaml:"members"`
    Groups      map[string][]string `yaml:"groups"`
    Repos       []RepoAccess        `yaml:"repos"`
    CreatedAt   time.Time           `yaml:"created_at"`
    UpdatedAt   time.Time           `yaml:"updated_at"`

    path string // not serialised — absolute path on disk
}
```

---

## Access logic — internal/collection/access.go

```go
// IsOwner returns true if username matches c.Owner exactly.
func (c *Collection) IsOwner(username string) bool

// IsMember returns true if:
//   - collection is public (everyone is implicitly a member), OR
//   - username is in the Members list, OR
//   - username is the owner (owner always has access)
func (c *Collection) IsMember(username string) bool

// IsInGroup returns true if username belongs to the named group.
func (c *Collection) IsInGroup(username, group string) bool

// CanAccessRepo returns true if username can clone/pull the named repo.
//
// Decision table (evaluated in order, stop at first match):
//   1. caller is owner                          → true always
//   2. collection is public                     → true
//   3. user not a member                        → false
//   4. repo.Groups=[] AND repo.Users=[]         → true (open to all members)
//   5. user in any repo.Groups                  → true
//   6. user in repo.Users                       → true
//   7. none of the above                        → false
func (c *Collection) CanAccessRepo(username, repoName string) bool

// AccessibleRepos returns repos the username can access, preserving order.
func (c *Collection) AccessibleRepos(username string) []RepoAccess

// AllRepos returns all repos regardless of access.
// Used by show command so members can see what exists even if denied.
func (c *Collection) AllRepos() []RepoAccess

// WhyCanAccess returns a human-readable reason for access decisions.
// Used by inspect and show commands.
//   "owner — full access"
//   "open to all members"
//   "member of group red-team"
//   "individually granted"
//   "no access — not a member"
//   "no access — group red-team required"
//   "no access — group red-team or individual grant required"
func (c *Collection) WhyCanAccess(username, repoName string) string

// FixCmd returns the exact gitcollect command the owner must run
// to grant username access to repoName. Used in denied-access messages.
// Example: "gitcollect group add cybersecurity red-team alice"
func (c *Collection) FixCmd(username, repoName string) string
```

---

## Mutation layer — internal/collection/mutation.go

Every mutation follows this exact four-step pattern:
  1. Validate inputs (regex + logic checks)
  2. Call platform API
  3. Only if API succeeds: update struct and save atomically
  4. Append to audit log (always — even on failure)

```go
func (c *Collection) AddMember(username string, client api.Client, actor string) error
func (c *Collection) RemoveMember(username string, client api.Client, actor string) error
func (c *Collection) AddToGroup(username, group string, client api.Client, actor string) error
func (c *Collection) RemoveFromGroup(username, group string, client api.Client, actor string) error
func (c *Collection) SetRepoAccess(repoName string, groups, users []string, client api.Client, actor string) error
func (c *Collection) CreateGroup(group, actor string) error
func (c *Collection) DeleteGroup(group, actor string) error
```

The actor parameter is the authenticated username — always stored in audit entries.

---

## CRITICAL: GitHub collaborator invite handling

When gitcollect calls AddCollaborator, GitHub sends the invited user an email.
They must ACCEPT the invite on GitHub before they can clone anything.
gitcollect must detect and explain this explicitly — never let it appear as
a silent 403.

When a clone attempt returns HTTP 403, gitcollect must:
  1. Call GetPendingInvite to check if there is an unaccepted invite
  2. If pending invite exists, print:

```
✗ clone: GitHub returned 403 for pen-test-tools
  You have a pending collaborator invite from <owner>.
  Accept it at: https://github.com/notifications
  Then retry: gitcollect clone <collection>
```

  3. If no pending invite, print the generic access denied error.

Add this method to the API client interface:
```go
// GetPendingInvite returns true if username has a pending (unaccepted)
// collaborator invite on owner/repo.
GetPendingInvite(owner, repo, username string) (bool, error)
```

Also print the invite warning immediately after member add:
```
⚠ <username> will receive a GitHub collaborator invite email.
  They must accept it before gitcollect clone will work.
  Invite URL: https://github.com/notifications
```

---

## gitcollect show — member view vs owner view

The show command automatically detects whether the caller is the owner
or a member by comparing the authenticated username to c.Owner.
No flags needed. Two different output formats.

### show: ALL repos visible to members — accessible and denied

Members see every repo in the collection including restricted ones.
This is intentional — members must know what repos exist so they can
request access. The YOU column reflects the caller specifically.

```
$ gitcollect show cybersecurity

Collection:   cybersecurity
Host:         github.com
Owner:        alby-tomy
Visibility:   private
Members:      4
Groups:       red-team (2) · analysts (2) · ops (1)

MEMBERS
alice · bob · charlie · diana

REPO              ACCESS RULE           YOU
pen-test-tools    open to all members   ✓ can clone
ctf-writeups      open to all members   ✓ can clone
vuln-scanner      red-team              ✗ no access — group red-team required
threat-reports    analysts              ✗ no access — analysts group required
ops-runbooks      ops                   ✗ no access — ops group required

2 repos accessible · 3 repos restricted
To request access, ask alby-tomy to run the relevant group add command.
Run: gitcollect inspect cybersecurity --user alice  for full detail
```

For each denied repo, include the exact fix command on the next line:
```
vuln-scanner   red-team   ✗ no access — group red-team required
                            Ask alby-tomy: gitcollect group add cybersecurity red-team alice
```

### show: owner view — WHO HAS ACCESS column

Owner sees the same repo list plus a WHO HAS ACCESS column showing
exactly which members can reach each repo. Owner does not need to be
in the members list to get this view.

```
$ gitcollect show cybersecurity

Collection:   cybersecurity
Host:         github.com
Owner:        alby-tomy (you)
Visibility:   private
Members:      4
Groups:       red-team (2) · analysts (2) · ops (1)

MEMBERS
alice · bob · charlie · diana

REPO              ACCESS RULE           WHO HAS ACCESS
pen-test-tools    open to all members   alice, bob, charlie, diana (4)
ctf-writeups      open to all members   alice, bob, charlie, diana (4)
vuln-scanner      red-team              alice, bob (2)
threat-reports    analysts              charlie, alice (2)
ops-runbooks      ops                   diana (1)
```

### show: stale YAML warning

After loading the collection, check updated_at. If older than 30 days:
```
⚠ This collection was last updated 45 days ago.
  If you are not the owner, ask alby-tomy for the latest collection file.
```
Print this warning at the top of show output, before the table.

### show: non-member sees nothing

A non-member asking about a private collection gets the same error
as "not found" — never confirm the collection exists:
```
✗ show: collection "cybersecurity" not found or access denied
```

---

## gitcollect member add — full output

```
$ gitcollect member add cybersecurity bob

✓ Added bob to cybersecurity
  Syncing platform access...
  ✓ Granted pull access: pen-test-tools, ctf-writeups (2 repos)
  ✗ Skipped: vuln-scanner (red-team group required)
  ✗ Skipped: ops-runbooks (ops group required)
  ✗ Skipped: threat-reports (analysts group required)

⚠ bob will receive a GitHub collaborator invite email.
  They must accept it before gitcollect clone will work.
  Invite URL: https://github.com/notifications

To grant bob access to restricted repos:
  gitcollect group add cybersecurity red-team bob
```

---

## gitcollect sync — new command

Clone repos not yet present locally, pull repos already cloned.
One command to bring a full collection up to date after any time away.

```
$ gitcollect sync cybersecurity

✓ Access verified (alice · groups: analysts)
  3 repos accessible · 2 skipped (no access)

[1/3] pen-test-tools    already cloned → pulling...   ✓ up to date
[2/3] ctf-writeups      already cloned → pulling...   ✓ 3 new commits
[3/3] threat-reports    not cloned     → cloning...   ✓ done (1.4s)
      vuln-scanner      skipped (no access — group red-team required)
      ops-runbooks      skipped (no access — ops group required)

✓ Synced 3 repos · 2 skipped (no access)
Run: gitcollect inspect cybersecurity --user alice  to see access details
```

---

## gitcollect list — stale YAML warning

Check updated_at on each collection after loading. Warn if over 30 days:

```
$ gitcollect list

  cybersecurity      12 repos   private   github.com
  ⚠ Last updated 45 days ago — ask the collection owner for the latest file
  machine-learning    8 repos   private   github.com
  frontend-libs       6 repos   public    github.com
```

Warning does not block the command — it informs only.

---

## gitcollect init — auto-detect owner

```go
func runInit(cmd *cobra.Command, args []string) error {
    name := args[0]
    owner, _ := cmd.Flags().GetString("owner")
    if owner == "" {
        // Default to authenticated user — resolve via API
        client := api.NewClient(host, token)
        user, err := client.GetAuthenticatedUser()
        if err != nil {
            return fmt.Errorf("init: cannot determine owner: %w", err)
        }
        owner = user
    }
    // ... create collection with owner set
}
```

---

## Access enforcement — internal/access/enforce.go

```go
// CheckCollectionAccess verifies the caller can use this collection.
// Owner always passes. Public collections always pass.
// Private: identical error for "not found" and "not a member"
// to prevent private collection existence disclosure.
func CheckCollectionAccess(col *collection.Collection, caller string) error

// CheckRepoAccess verifies three things in order:
//   1. Caller is a member or owner (local check — fast)
//   2. Caller passes CanAccessRepo (local check — fast)
//   3. Caller is an actual collaborator on the platform (API call)
//      If step 3 returns 403, call GetPendingInvite and return
//      ErrPendingInvite with the accept URL if invite is pending.
func CheckRepoAccess(
    col *collection.Collection,
    repoName, caller string,
    client api.Client,
) error

// FilterAccessible returns repos accessible to caller.
// Uses local rules first (fast path) then platform verification.
func FilterAccessible(
    col *collection.Collection,
    caller string,
    client api.Client,
) ([]collection.RepoAccess, error)
```

---

## Platform sync — internal/access/sync.go

```go
// SyncCollaborators computes the correct collaborator state for every
// (member, repo) pair and drives the platform API to match it.
// Runs concurrently (max 4 parallel API calls via semaphore).
// Owner is never added or removed as a collaborator — they own the repos.
// Returns counts of added/removed and any errors encountered.
// Partial failures are collected and returned as a joined error —
// successful pairs are applied even when others fail.
func SyncCollaborators(col *collection.Collection, client api.Client) (added, removed int, err error)
```

---

## Inspect — internal/access/inspect.go

```go
type RepoAccessDetail struct {
    RepoName  string
    CanAccess bool
    Reason    string
    FixCmd    string  // exact command owner must run to grant access
}
func UserAccessMap(col *collection.Collection, username string) []RepoAccessDetail

type MemberAccessDetail struct {
    Username  string
    CanAccess bool
    Reason    string
}
func RepoAccessMap(col *collection.Collection, repoName string) []MemberAccessDetail

type AccessMatrix struct {
    Members []string
    Repos   []string
    Grid    [][]bool
    Reasons [][]string
}
func FullMatrix(col *collection.Collection) AccessMatrix
```

---

## Audit trail — internal/audit/audit.go

Every mutation appended to:
  filepath.Join(home, ".gitcollect", "audit", "<collection>.log")
as newline-delimited JSON. One object per line.

```go
type AuditEntry struct {
    Timestamp  time.Time `json:"timestamp"`
    Collection string    `json:"collection"`
    Actor      string    `json:"actor"`       // authenticated username who ran the command
    Action     string    `json:"action"`      // "member.add" | "member.remove" |
                                              // "group.add" | "group.remove" |
                                              // "repo.access.set" | "visibility.change" |
                                              // "collection.delete" | "collection.create"
    Target     string    `json:"target"`      // username, group name, or repo name
    Detail     string    `json:"detail"`      // human-readable summary
    Result     string    `json:"result"`      // "ok" | "error: <message>"
}

// --since flag accepts ONLY these values — no raw Go duration strings.
// Parse with a lookup map, not time.ParseDuration directly.
var sinceDurations = map[string]time.Duration{
    "1h":  time.Hour,
    "24h": 24 * time.Hour,
    "7d":  7 * 24 * time.Hour,
    "30d": 30 * 24 * time.Hour,
    "90d": 90 * 24 * time.Hour,
}

func Append(entry AuditEntry) error
func Read(collection string) ([]AuditEntry, error)
func Filter(entries []AuditEntry, user string, since time.Duration) []AuditEntry
```

Collection delete must write a final audit entry before removing the log:
```json
{"action":"collection.delete","detail":"Collection deleted. Final audit entry.","result":"ok"}
```

---

## API client interface — internal/api/client.go

```go
type Client interface {
    GetRepo(owner, repo string) (RepoInfo, error)
    GetAuthenticatedUser() (string, error)
    AddCollaborator(owner, repo, username, permission string) error
    RemoveCollaborator(owner, repo, username string) error
    CheckCollaborator(owner, repo, username string) (bool, error)
    GetPendingInvite(owner, repo, username string) (bool, error)
    Host() string
}

type RepoInfo struct {
    Name     string
    CloneURL string  // always HTTPS — never SSH
    Private  bool
    Archived bool
}

func NewClient(host, token string) Client

var (
    ErrNotFound      = errors.New("repository not found")
    ErrUnauthorized  = errors.New("invalid or missing token")
    ErrForbidden     = errors.New("insufficient permissions")
    ErrRateLimit     = errors.New("API rate limit exceeded")
    ErrPendingInvite = errors.New("collaborator invite not yet accepted")
)
```

---

## Config — internal/config/config.go

```go
// ALL paths use os.UserHomeDir() + filepath.Join. Never hardcode.

func HomeDir() (string, error)           // os.UserHomeDir()
func GitcollectDir() (string, error)     // <home>/.gitcollect
func CollectionsDir() (string, error)    // <home>/.gitcollect/collections
func AuditDir() (string, error)          // <home>/.gitcollect/audit
func ConfigFile() (string, error)        // <home>/.gitcollect/config

// SaveToken writes token to config file with permission 0600.
// Creates parent directories with 0700 if they do not exist.
func SaveToken(host, token string) error

// LoadToken reads the token for the given host.
// Returns ErrNotAuthenticated if no token exists for that host.
func LoadToken(host string) (string, error)

// ListHosts returns all hosts that have stored tokens.
func ListHosts() ([]string, error)

var ErrNotAuthenticated = errors.New("not authenticated — run: gitcollect auth")
```

---

## Output package — internal/output/output.go

```go
func Success(format string, args ...any)                       // green ✓  → stdout
func Error(format string, args ...any)                         // red ✗    → stderr
func Warn(format string, args ...any)                          // yellow ⚠ → stderr
func Info(format string, args ...any)                          // cyan →   → stderr
func Dim(format string, args ...any)                           // muted    → stderr
func Progress(current, total int, label string)                // \r overwrite → stderr
func Table(headers []string, rows [][]string)                  // aligned columns → stdout
func JSON(v any) error                                         // marshal → stdout
func Confirm(prompt string) bool                               // "prompt [y/N]: " → bool
func ConfirmWord(prompt, word string) bool                     // require typing exact word
func Suggestion(cmd string)                                    // "Run: <cmd>" → stderr
func InviteWarning(username, owner, notifURL, retryCmd string) // pending invite → stderr
func StaleWarning(collectionName string, daysSince int)        // stale YAML warning → stderr
```

---

## Non-negotiable engineering constraints

### 1. Efficiency

- Concurrent clone/pull/sync with semaphore — default 4, --concurrency to override.
- SyncCollaborators runs API calls concurrently (max 4 in parallel).
- Cache GetAuthenticatedUser() result per command invocation. One API call only.
- Zero network calls on startup. gitcollect list reads local YAML only.
- strings.Builder not + in loops. One http.Client per command invocation.
- Audit log append runs in a goroutine (non-blocking) but errors go to stderr.

### 2. Security

- Token: stored at 0600. Echo disabled via golang.org/x/term.
  Never in logs, errors, flags, or shell history. Truncated in debug: ghp_xxxx...
- API-first: YAML written only AFTER platform API call succeeds.
  If API fails, YAML is unchanged. There is no in-between state.
- Non-disclosure: private collections return identical errors for "not found"
  and "access denied" — never confirm existence to non-members.
- Double-check on clone: local manifest + platform collaborator check.
  Detect pending invite and print clear error with accept URL.
- Validation regexes (compile once at package init, reuse):
  Collection name: ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$
  Repo name:       ^[a-zA-Z0-9._-]{1,100}$
  Username:        ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$
  Group name:      ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,30}$
  Reject all inputs containing ../ \ or null bytes.
- HTTPS only: use clone_url from API. Reject SSH patterns.
- File permissions: all files 0600, directories 0700.
- Destructive operations require confirmation before executing.
- Self-removal requires --confirm-self flag.

### 3. Reliability

- Atomic YAML writes: temp file + os.Rename. Never write directly to target.
- API-then-YAML order: must succeed in that sequence or operation is rolled back.
- Partial clone/sync: all repos attempted, failures collected and reported at end.
- SyncCollaborators partial failure: apply successful pairs, return joined error.
- git prerequisite: exec.LookPath("git") before any git command.
- API timeout: context.WithTimeout of 15 seconds on all HTTP requests.
- Idempotent mutations: AddMember, AddToGroup, SetRepoAccess safe to run twice.
- Audit on failure: log every failed operation with result: "error: <message>".
- SyncCollaborators called after every membership mutation before returning.

### 4. Consistency

- Error format: <command>: <what happened>: <why> — never raw Go error strings.
- stdout = data and success. stderr = errors, warnings, progress, prompts.
- Exit codes: 0 = success · 1 = operational error · 2 = usage/argument error.
- Colours: green = success · red = error · yellow = warning · cyan = info.
  Respect NO_COLOR env var. Auto-disable when not a TTY.
- Flag names: kebab-case always.
- Timestamps: stored RFC3339 UTC · displayed in local time.
- --pick flag is space-separated: --pick repo1 repo2
- --since accepts only: 1h, 24h, 7d, 30d, 90d
- After every mutation: print what changed, what was skipped, exact fix commands.
- Every denied access message includes the exact gitcollect command to fix it.

### 5. Ease of use

- Not authenticated: "Not authenticated. Run: gitcollect auth"
- Collection not found (private or missing): identical error — never disclose existence
- Typo detection: suggest closest match if Levenshtein distance ≤ 2
- Group add of non-member: guide to member add first with exact command
- Group delete blocked: list exactly which repos block it
- Every access denial: reason + exact command to fix it
- Pending GitHub invite: detect 403 + print accept URL + retry command
- Stale YAML (>30 days): warn on list and show
- Owner view vs member view: auto-detected, no flags needed
- --dry-run on clone, pull, sync: show exactly what would happen, nothing executes
- --json on list, show, inspect, audit, whoami: machine-readable output
- Shell completion for bash, zsh, fish, powershell via Cobra built-ins

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

Use stdlib testing only. No external assertion libraries.
Use t.TempDir() for all file tests. Never touch real home directory paths.
Use os.UserHomeDir() pattern in tests — never hardcode paths.

| Package                    | What to test                                                         |
|----------------------------|----------------------------------------------------------------------|
| internal/collection        | Load, Save, Validate, all mutations, IsOwner, IsMember, CanAccessRepo, AllRepos, WhyCanAccess, FixCmd |
| internal/collection/access | Full decision table — all rows including owner, public, union cases  |
| internal/access            | CheckCollectionAccess, CheckRepoAccess with pending invite mock, FilterAccessible, SyncCollaborators |
| internal/audit             | Append, Read, Filter by user and all --since values                  |
| internal/api               | GitHub + GitLab against httptest.Server mocks + GetPendingInvite     |
| internal/git               | Correct args to git subprocess for Clone, Pull, Status, Sync         |
| internal/config            | File at 0600, directory at 0700, paths via os.UserHomeDir()          |
| internal/output            | Table alignment, JSON, Confirm, InviteWarning, StaleWarning          |

Minimum 80% coverage on all internal/ packages.

Access control test matrix — every row must be covered:

```
Scenario                                                  Expected
────────────────────────────────────────────────────────  ─────────────────────
owner + any repo                                        → allowed (owner rule)
public collection + any caller                          → allowed
private + member + repo open (groups=[] users=[])       → allowed
private + member + repo groups=[G] + member in G        → allowed (group match)
private + member + repo users=[U] + caller == U         → allowed (user match)
private + member + groups=[G] AND users=[U]
  + caller in G (union test)                            → allowed
private + member + groups=[G] AND users=[U]
  + caller not in G but caller == U                     → allowed (user match)
private + member + groups=[G]
  + member NOT in G + NOT in users                      → ErrGroupDenied
private + non-member                                    → ErrForbidden (non-disclosure)
private + non-member + any repo                         → ErrForbidden (non-disclosure)
clone + 403 + pending invite = true                     → ErrPendingInvite with URL
clone + 403 + pending invite = false                    → ErrForbidden
owner not in members + owner runs show                  → owner view renders correctly
member runs show                                        → sees ALL repos inc denied
```

---

## Scope boundary — do NOT build

- No GUI or TUI
- No web server or daemon
- No database (YAML + newline-delimited JSON audit log only)
- No SSH clone (HTTPS only)
- No Bitbucket (GitHub + GitLab only)
- No automatic git installation
- No telemetry or analytics
- No email or webhook notifications
- No encryption of collection YAML
- No gitcollect admin or super-user mode
- No gitcollect fetch command (future roadmap item)
- No dashboard or web UI (future roadmap item)

---

## PROGRESS TRACKER

Read this section FIRST at the start of every agent session.
Do not write a single line of code before reading it.
Update the file completion table and session log before ending the session.

---

### How to continue a session

Say this at the start of every new session:

"Read the PROGRESS TRACKER in this file. Do not regenerate any file already
marked done. Continue from where the previous session stopped. Generate one
file at a time, complete and compilable — no stubs or placeholders. After
finishing each file, update its STATUS to done and ask me before moving to
the next file."

---

### Session log

```
Session 1 — [DATE] — [MODEL]
────────────────────────────────────────────────────────────────────
Completed:    (none yet)
In progress:  (none yet)
Blockers:     (none yet)
Decisions:    (none yet)
Next session: Start with main.go → go.mod → cmd/root.go → internal/output/output.go
```

---

### File completion table

STATUS values: todo | in-progress | done | blocked:<reason>

```
FILE                                         STATUS       NOTES
───────────────────────────────────────────  ───────────  ─────────────────────
main.go                                      todo
go.mod                                       todo
Makefile                                     todo
.goreleaser.yaml                             todo

cmd/root.go                                  todo
cmd/auth.go                                  todo
cmd/whoami.go                                todo
cmd/init.go                                  todo
cmd/delete.go                                todo
cmd/list.go                                  todo
cmd/show.go                                  todo
cmd/visibility.go                            todo
cmd/add.go                                   todo
cmd/remove.go                                todo
cmd/repo.go                                  todo
cmd/member.go                                todo
cmd/group.go                                 todo
cmd/inspect.go                               todo
cmd/audit.go                                 todo
cmd/clone.go                                 todo
cmd/pull.go                                  todo
cmd/status.go                                todo
cmd/sync.go                                  todo
cmd/version.go                               todo

internal/collection/collection.go            todo
internal/collection/access.go                todo
internal/collection/mutation.go              todo
internal/collection/collection_test.go       todo

internal/access/enforce.go                   todo
internal/access/sync.go                      todo
internal/access/inspect.go                   todo
internal/access/access_test.go               todo

internal/audit/audit.go                      todo
internal/audit/audit_test.go                 todo

internal/git/git.go                          todo
internal/git/git_test.go                     todo

internal/api/client.go                       todo
internal/api/github.go                       todo
internal/api/gitlab.go                       todo
internal/api/api_test.go                     todo

internal/config/config.go                    todo
internal/config/config_test.go               todo

internal/output/output.go                    todo
```

---

### Architecture decisions log

Record decisions made during implementation so future sessions
do not re-debate them. Add entries as: "DECISION: <what> — <why>"

```
(none yet)
```
