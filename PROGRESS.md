# gitcollect — Implementation Progress

> Last updated: Session 13 · 2026-07-01  
> Build status: `go build ./...` clean · `go test ./...` all 9 packages green

---

## What was built

**gitcollect** is a standalone Go CLI that lets developers group GitHub/GitLab repositories into named **collections** and control who can access them — at both the collection level (who is a member) and the repo level (which groups or individuals can reach which repos).

It does not replace Git. It wraps Git and the GitHub/GitLab APIs to add the grouping and access-control layer that neither platform provides natively. When gitcollect grants or revokes access, it does so by calling the real GitHub/GitLab collaborator APIs — not by maintaining a shadow list.

---

## Current state (as of Session 13)

| Check | Result |
|---|---|
| `go build ./...` | ✓ clean |
| `go test ./...` | ✓ all 9 packages pass |
| `go vet ./...` | ✓ clean |
| Identity migration | ✓ complete — immutable platform IDs |
| Test coverage | ✓ all internal/ packages above 80% |

---

## Complete feature list

### Authentication
| Command | Status | Added |
|---|---|---|
| `gitcollect auth` | ✓ done | Session 1 |
| `gitcollect auth --host gitlab.com` | ✓ done | Session 1 |
| `gitcollect whoami` | ✓ done | Session 2 |
| `gitcollect whoami --json` | ✓ done | Session 11 |

Token stored at `~/.gitcollect/config` (0600). Echo disabled on input. Never in logs, errors, or flags. Cached per-invocation so `GetAuthenticatedUser` is called at most once per command. Both login and platform ID are cached after first resolve (Session 13 — immutable ID migration).

---

### Collection lifecycle
| Command | Status | Added |
|---|---|---|
| `gitcollect init <name>` | ✓ done | Session 1 |
| `gitcollect init <name> --public` | ✓ done | Session 1 |
| `gitcollect init <name> --description "..."` | ✓ done | Session 1 |
| `gitcollect delete <collection>` | ✓ done | Session 2 |
| `gitcollect list` | ✓ done | Session 1 (redesigned Session 4) |
| `gitcollect list --private` | ✓ done | Session 4 |
| `gitcollect list --public` | ✓ done | Session 4 |
| `gitcollect list --json` | ✓ done | Session 2 |
| `gitcollect show <collection>` | ✓ done | Session 2 |
| `gitcollect show <collection> --json` | ✓ done | Session 2 |
| `gitcollect visibility <collection> public\|private` | ✓ done | Session 2 |

**Notable design decisions:**
- `list` reads local YAML only — zero network calls, works fully offline
- `list --all` was deliberately removed in Session 4; `list` with no flags now shows all collections you own or are a member of
- `delete` requires typing the collection name to confirm (not just y/N)
- `show` displays a per-repo YOU column (✓/✗ + reason) for members; owner sees WHO HAS ACCESS instead (Session 11)
- Stale warning printed when a collection's `updated_at` is >30 days old (Session 11)
- `list`'s role detection is format-aware from Session 13: compares platform ID for Version "2" files, cached login for legacy "1" files

---

### Repo management
| Command | Status | Added |
|---|---|---|
| `gitcollect add <collection> <repo> [repo...]` | ✓ done | Session 1 (multi-value Session 12) |
| `gitcollect remove <collection> <repo>` | ✓ done | Session 2 |
| `gitcollect repo access <collection> <repo> --groups g1,g2` | ✓ done | Session 2 |
| `gitcollect repo access <collection> <repo> --users u1,u2` | ✓ done | Session 2 |
| `gitcollect repo access <collection> <repo> --open` | ✓ done | Session 2 |
| `gitcollect repo show <collection> <repo>` | ✓ done | Session 2 |
| `gitcollect repo grant <collection> <repo> <user>` | ✓ done | Session 3 |
| `gitcollect repo revoke <collection> <repo> <user>` | ✓ done | Session 3 |

**Notable design decisions:**
- `remove` requires typing the repo name to confirm (changed Session 8 from y/N)
- `add` validates all repo names up front before touching the collection (malformed name = usage error for the whole command, not a per-item failure)
- `repo grant` refuses with `ErrRepoOpen` if the repo is currently open to all members — appending a Users entry would silently revoke every other member's access
- `repo revoke` refuses with `ErrRepoWouldOpen` if revoking the last individually-granted user would leave Groups=[] and Users=[] (which means "open to all members")
- `add` accepts multiple repo names in one invocation (Session 12); failures are collected and reported together without aborting the batch

---

### Member management
| Command | Status | Added |
|---|---|---|
| `gitcollect member add <collection> <username> [username...]` | ✓ done | Session 1 (multi-value Session 12) |
| `gitcollect member remove <collection> <username>` | ✓ done | Session 2 |
| `gitcollect member remove ... --confirm-self` | ✓ done | Session 2 |
| `gitcollect member list <collection>` | ✓ done | Session 2 |

**Notable design decisions:**
- `member add` warns if any newly-granted repo has a pending unaccepted GitHub collaborator invite (Session 11); GitLab is immediate, never has this state
- `member add` with multiple usernames prints a `--- username ---` header between blocks; single-username invocation is byte-for-byte unchanged
- One batch failure does not abort the rest; all items are attempted

---

### Group management
| Command | Status | Added |
|---|---|---|
| `gitcollect group create <collection> <group>` | ✓ done | Session 2 |
| `gitcollect group delete <collection> <group>` | ✓ done | Session 2 |
| `gitcollect group add <collection> <group> <user> [user...]` | ✓ done | Session 2 (multi-value Session 12) |
| `gitcollect group remove <collection> <group> <user>` | ✓ done | Session 2 |
| `gitcollect group list <collection>` | ✓ done | Session 2 |
| `gitcollect group show <collection> <group>` | ✓ done | Session 2 |

**Notable design decisions:**
- `group delete` is blocked if any repo still references the group — caller must clear repo restrictions first
- `group add` of a non-member surfaces `ErrNotMember` with a guided suggestion to `member add` first

---

### Access inspection
| Command | Status | Added |
|---|---|---|
| `gitcollect inspect <collection> --user <username>` | ✓ done | Session 2 |
| `gitcollect inspect <collection> --repo <repo>` | ✓ done | Session 2 |
| `gitcollect inspect <collection>` | ✓ done | Session 2 |

**Notable design decisions:**
- Every denied row includes the exact fix command (Session 11 via `Collection.FixCmd`)
- Owner bypass in `CanAccessRepo`/`WhyCanAccess` (Session 11): `inspect --user <owner>` correctly reports ✓ even if the owner is not separately listed as a member

---

### Audit trail
| Command | Status | Added |
|---|---|---|
| `gitcollect audit <collection>` | ✓ done | Session 2 |
| `gitcollect audit <collection> --user <u>` | ✓ done | Session 2 |
| `gitcollect audit <collection> --since <dur>` | ✓ done | Session 2 (strict allow-list Session 11) |
| `gitcollect audit <collection> --json` | ✓ done | Session 2 |

**Notable design decisions:**
- `--since` accepts only five exact values: `1h`, `24h`, `7d`, `30d`, `90d` (changed from flexible parser in Session 11)
- Audit log is newline-delimited JSON at `~/.gitcollect/audit/<collection>.log`
- Failed operations are logged too (`result: "error: ..."`) — auditability requires seeing what was attempted
- `audit.go` received zero changes in Session 13 — Actor/Target stay login strings, populated by callers

---

### Code activity
| Command | Status | Added |
|---|---|---|
| `gitcollect activity <collection>` | ✓ done | Session 7 |
| `gitcollect activity <collection> --repo <r>` | ✓ done | Session 7 |
| `gitcollect activity <collection> --since <dur>` | ✓ done | Session 7 (strict allow-list Session 11) |
| `gitcollect activity <collection> --limit <n>` | ✓ done | Session 7 |
| `gitcollect activity <collection> --json` | ✓ done | Session 7 |

**Notable design decisions:**
- Persists fetched commits to `~/.gitcollect/activity/<collection>.log` (dedup by repo+SHA)
- Display always shows full known history (this run's fetch + all prior records); `--limit` only bounds the live fetch
- Default branch fetched per repo via `GetRepo` before `ListCommits`; falls back to "main" if empty

---

### Git operations
| Command | Status | Added |
|---|---|---|
| `gitcollect clone <collection>` | ✓ done | Session 2 |
| `gitcollect clone <collection> --pick "r1 r2"` | ✓ done | Session 2 (space-separated Session 11) |
| `gitcollect clone <collection> --dry-run` | ✓ done | Session 2 |
| `gitcollect clone <collection> --concurrency 8` | ✓ done | Session 2 |
| `gitcollect clone <collection> --dest <dir>` | ✓ done | Session 2 |
| `gitcollect pull <collection>` | ✓ done | Session 2 |
| `gitcollect status <collection>` | ✓ done | Session 2 |
| `gitcollect sync <collection>` | ✓ done | Session 11 |
| `gitcollect sync <collection> --dest <dir>` | ✓ done | Session 11 |
| `gitcollect sync <collection> --dry-run` | ✓ done | Session 11 |
| `gitcollect sync <collection> --concurrency 8` | ✓ done | Session 11 |

**Notable design decisions:**
- `clone` double-checks platform collaborator status via API before cloning — local manifest alone is not sufficient
- `clone` warns when a skipped repo is a pending GitHub invite rather than a genuine denial (Session 11)
- `--pick` changed from comma-separated to space-separated in Session 11 (`--pick "r1 r2"` or `--pick r1 --pick r2`)
- `sync` clones missing repos + pulls existing ones in one pass; concurrent (default 4), reuses `cloneOne`
- All git operations accessible only to users who actually have collaborator access on the platform

---

### System
| Command | Status | Added |
|---|---|---|
| `gitcollect version` | ✓ done | Session 2 |
| `gitcollect completion bash\|zsh\|fish\|powershell` | ✓ done | Session 2 (cobra built-in) |

---

## Session-by-session summary

| Session | Date | Key work |
|---|---|---|
| 1 | 2026-06-29 | `main.go`, `go.mod`, `cmd/root.go`, `cmd/auth.go` |
| 2 | 2026-06-29 | All remaining cmd/ files; all internal/ packages; full test suite at 80%+ coverage; fixed Table padding bug (byte len → rune count) |
| 3 | 2026-06-29 | `repo grant` / `repo revoke` (beyond original spec); `ErrRepoOpen`/`ErrRepoWouldOpen` sentinels; fixed concurrent map write in mockClient |
| 4 | 2026-06-29 | `list` redesign: removed `--all`, added `--private`/`--public` filters |
| 5 | 2026-06-29 | Progress tracker housekeeping only |
| 6 | 2026-06-29 | `ErrUnauthorized` hint in `Execute()`; `whoami`'s `anyRejected` hint |
| 7 | 2026-06-29 | `gitcollect activity` command (new package `internal/activity`); `ListCommits` + `DefaultBranch` in API client |
| 8 | 2026-06-29 | `remove` changed from y/N to type-name-to-confirm |
| 9 | 2026-06-29 | `show` per-caller YOU column; owner-bypass bug fixed in `inspect` |
| 10 | 2026-06-29 | Documentation accuracy audit — synced PROMPT.md's example blocks to real implementation output |
| 11 | 2026-06-30 | PROMPT_v2.md delta absorbed: `sync` command, `whoami --json`, pending-invite detection, `FixCmd`, Levenshtein typo suggestions, WHO HAS ACCESS owner view, stale warnings, strict `--since` allow-list, `--pick` space-separated |
| 12 | 2026-06-30 | Multi-value support: `member add`, `group add`, `add` each accept multiple targets in one invocation; `multiAddMock` and three new test files |
| 13 | 2026-07-01 | Identity migration: immutable platform IDs replace mutable usernames in all ownership/membership fields; `UserInfo`, `GetUser`, `Logins` cache, `Migrate`, `loadForOwner`, `migrateIfNeeded`, format-aware `roleFor` in list |

---

## Architecture highlights

### Core principle
gitcollect's local YAML is a *declaration of intent* — it describes who should have access. The GitHub/GitLab platform is the *enforcement point* — it is where access is actually granted or revoked. Every mutation calls the platform API to completion before the local YAML is written. If the API call fails, the YAML does not change.

### Data storage
```
~/.gitcollect/
  config                          # YAML: tokens + cached logins + cached IDs (0600)
  collections/<name>.yaml         # one file per collection (0600)
  audit/<name>.log                # newline-delimited JSON, append-only
  activity/<name>.log             # newline-delimited JSON, append-only
```

### Collection YAML format (Version "2" — current)
```yaml
version: "2"
name: cybersecurity
host: github.com
owner: "583231"          # immutable platform user ID (not a login)
visibility: private
members:
  - "99"                 # IDs, not logins
groups:
  red-team:
    - "99"
repos:
  - name: vuln-scanner
    groups: [red-team]
    users: []
logins:                  # ID → login cache (single source of truth for display)
  "583231": alice
  "99": bob
```

### Identity model (Session 13)
- **Owner / Members / Groups / RepoAccess.Users** — store immutable platform user IDs (GitHub: numeric int64 as decimal string; GitLab: same). These never change when a user renames their account.
- **`col.Logins[id]`** — the single source of truth for the login string, used for all API path-building, display, audit log, and fix-command suggestions.
- **Add operations** always call `GetUser()` live — the account must exist before access is granted.
- **Remove operations** use `IDForLogin()` reverse-lookup from the cache — no network call; survives account renames.
- **Opportunistic migration** — triggered on first write-capable load of a legacy Version "1" file; never from `list` (network-free) or public collection reads (auth-free).

### Access control model
```
collection public                       → allowed for any caller
private + owner                         → allowed (CanAccessRepo has owner bypass built in)
private + member + repo open            → allowed (Groups=[] AND Users=[])
private + member + repo groups=[G]      → allowed if member is in G
private + member + repo users=[U]       → allowed if member is in U
private + non-member                    → ErrForbidden (same error as "not found", no disclosure)
```

### Key packages
| Package | Role |
|---|---|
| `internal/api` | GitHub + GitLab REST client; `UserInfo`; `GetUser`; sentinels |
| `internal/collection` | YAML struct; access logic; all mutations; `Migrate` |
| `internal/access` | Access enforcement; inspect (user/repo/matrix views) |
| `internal/config` | Token + login + ID cache; directory layout |
| `internal/audit` | Append-only audit log (access mutations) |
| `internal/activity` | Append-only activity log (git commit history) |
| `internal/git` | `Clone`, `Pull`, `PullWithSummary`, `Status` wrappers |
| `internal/output` | Coloured output; table (rune-width-aware); prompts; JSON |
| `cmd/` | Cobra command tree; `loadForOwner`/`loadForRead`/`loadForGit` |

### Security properties
- Tokens stored at 0600; never logged, never in error messages
- Private collections return identical errors for "not found" and "access denied" (non-disclosure)
- Typo suggestions for unrecognised collection names only on owner-required paths — never on the non-disclosure path (to avoid leaking existence of private collections)
- All files under `~/.gitcollect/` written with 0600; directories 0700
- Atomic YAML writes via temp file + `os.Rename`
- HTTPS only — SSH clone URLs are never used

---

## What was deliberately NOT built

These were considered and explicitly rejected:

| Feature | Decision | Session |
|---|---|---|
| `init --owner <org>` | Architectural conflict: owner checks are literal string equality against the caller's own login; an org name can never satisfy them | 11 |
| `list --all` | Removed; `list` with no flags now shows everything | 4 |
| Flexible `--since` parser | Replaced with strict 5-value allow-list (`1h`/`24h`/`7d`/`30d`/`90d`) | 11 |
| Comma-separated `--pick` | Changed to space-separated (`--pick "r1 r2"`) | 11 |
| GUI / TUI | Out of scope |  |
| Database | YAML + newline-delimited JSON only |  |
| SSH clone | HTTPS only |  |
| Bitbucket | GitHub + GitLab only in v1 |  |
| Telemetry | Out of scope |  |

---

## Test coverage

| Package | Coverage | Notes |
|---|---|---|
| `internal/collection` | 83.8% | All mutations, access logic, rollback paths |
| `internal/access` | 93.9% | Full access matrix, owner bypass, inspect views |
| `internal/api` | 85.5% | GitHub + GitLab against `httptest.Server` |
| `internal/audit` | 82.8% | Append, read, filter |
| `internal/activity` | 84.4% | Append, read, filter, dedup |
| `internal/git` | 85.4% | Clone, Pull, PullWithSummary (fake-git harness) |
| `internal/config` | 82.5% | Token, user, ID cache; directory paths |
| `internal/output` | 98.1% | Table, JSON, confirm, stale/invite warnings |
| `cmd` | ~15.5% | Pure-logic helpers only; command integration not tested |

The `cmd` package's low coverage is a known, consistently deferred gap (see PROMPT.md's session log for nine consecutive "Next session should start with: cmd/list_test.go" entries). All `internal/` packages are above the 80% requirement.
