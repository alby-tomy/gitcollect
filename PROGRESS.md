# gitcollect — Implementation Progress

> Last updated: Session 27 · 2026-07-08 (SCOPED_COMPLETION_CHECK.md — all 13 files verified and gaps closed)
> Build status: `go build ./...` clean · `go test ./...` all packages green

---

## What was built

**gitcollect** is a standalone Go CLI that lets developers group GitHub/GitLab repositories into named **collections** and control who can access them — at both the collection level (who is a member) and the repo level (which groups or individuals can reach which repos).

It does not replace Git. It wraps Git and the GitHub/GitLab APIs to add the grouping and access-control layer that neither platform provides natively. When gitcollect grants or revokes access, it does so by calling the real GitHub/GitLab collaborator APIs — not by maintaining a shadow list.

---

## Current state (v3.3.0, `2ebb3bf`)

| Check | Result |
|---|---|
| `go build ./...` | ✓ clean |
| `go vet ./...` | ✓ clean |
| `go test ./... -race` | ✓ all 10 packages pass |
| `gofmt -l .` | ✓ empty |
| `staticcheck ./...` | ✓ clean but for 3 deliberate ST1005s (multi-sentence CLI guidance where the trailing full stop is correct) |
| `go test -cover ./cmd/...` | 67.4% |
| Identity migration | ✓ complete — immutable platform IDs |
| cmd test files | 44 / 44 command files have a test file |
| Latest release | v3.3.0 — 7 assets, serving as `latest` |
| Open defects | 6 verified, 1 unverified — see the defect register |

---

## Complete feature list

### Authentication
| Command | Status | Added |
|---|---|---|
| `gitcollect auth` | ✓ done | Session 1 |
| `gitcollect auth --host gitlab.com` | ✓ done | Session 1 |
| `gitcollect whoami` | ✓ done | Session 2 |
| `gitcollect whoami --json` | ✓ done | Session 11 |
| `gitcollect whoami --check` | ✓ done | Session 17 (FEATURE_GAPS_2 A5) |

Token stored at `~/.gitcollect/config` (0600). Echo disabled on input. Never in logs, errors, or flags. Cached per-invocation so `GetAuthenticatedUser` is called at most once per command. Both login and platform ID are cached after first resolve (Session 13 — immutable ID migration).

---

### Collection lifecycle
| Command | Status | Added |
|---|---|---|
| `gitcollect init <name>` | ✓ done | Session 1 |
| `gitcollect init <name> --public` | ✓ done | Session 1 |
| `gitcollect init <name> --description "..."` | ✓ done | Session 1 |
| `gitcollect init <name> --namespace <org>` | ✓ done | PRE_SHIP Priority 3 |
| `gitcollect rename <old> <new>` | ✓ done | Session 17 (FEATURE_GAPS_2 A1) |
| `gitcollect copy <source> <new>` | ✓ done | Session 17 (FEATURE_GAPS_2 A2) |
| `gitcollect transfer <collection> <new-owner>` | ✓ done | Session 18 (FEATURE_SCALABILITY) |
| `gitcollect scale <collection> organisation\|team` | ✓ done | Session 18 (FEATURE_SCALABILITY) |
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
| `gitcollect repo grant <collection> <repo> <user>` | ✗ removed | Session 3 (removed PRE_SHIP) |
| `gitcollect repo revoke <collection> <repo> <user>` | ✗ removed | Session 3 (removed PRE_SHIP) |
| `gitcollect add <collection> --pattern <glob> --org <org>` | ✓ done | Session 17 (FEATURE_GAPS_2 B3) |
| `gitcollect add <collection> --topic <t> --org <org>` | ✓ done | Session 17 (FEATURE_GAPS_2 B3) |

**Notable design decisions:**
- `remove` requires typing the repo name to confirm (changed Session 8 from y/N)
- `add` validates all repo names up front before touching the collection (malformed name = usage error for the whole command, not a per-item failure)
- `repo grant`/`repo revoke` removed (PRE_SHIP Priority 2): confusing `ErrRepoOpen`/`ErrRepoWouldOpen` edge cases; `repo access --users` covers every use case
- `add` accepts multiple repo names in one invocation (Session 12); failures are collected and reported together without aborting the batch
- `add --pattern`/`--topic` uses GitHub Search API (30 req/min limit); `SearchRepos` API method added

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
| `gitcollect group admin add <collection> <group> <user>` | ✓ done | Session 18 (FEATURE_SCALABILITY) |
| `gitcollect group admin remove <collection> <group> <user>` | ✓ done | Session 18 (FEATURE_SCALABILITY) |
| `gitcollect group admin list <collection>` | ✓ done | Session 18 (FEATURE_SCALABILITY) |

**Notable design decisions:**
- `group delete` is blocked if any repo still references the group — caller must clear repo restrictions first
- `group add` of a non-member surfaces `ErrNotMember` with a guided suggestion to `member add` first
- Group admins can manage their own group's membership only; `CanManageGroup` enforces this
- Group admin feature gated behind `GroupAdminsEnabled` flag; off by default

### Dynamic scalability (FEATURE_SCALABILITY.md — complete)
| Feature | Status | Added |
|---|---|---|
| `GroupAdminsEnabled` + `GroupAdmins` fields in Collection struct | ✓ done | Session 18 / confirmed Session 26 (Step 1) |
| `IsGroupAdmin`, `CanManageGroup`, `GroupAdminOf` in access.go | ✓ done | Session 26 (Step 2) — full role helpers |
| `ErrGroupAdminsDisabled`, `ErrWrongGroup`, `ErrSelfTransfer`, `ErrAdminPrivilegeEscalation` | ✓ done | Session 26 (Step 2) — all four sentinels in internal/collection/access.go |
| `RemoveMember` cleans `GroupAdmins` entries | ✓ done | Session 26 (Step 3) — mutation.go:401 |
| `DeleteGroup` clears `GroupAdmins[group]` | ✓ done | Session 26 (Step 3) — mutation.go:569 |
| `gitcollect transfer` with typed confirmation | ✓ done | Session 26 (Step 4) — cmd/transfer.go; 7 tests |
| `gitcollect scale organisation\|team` | ✓ done | Session 26 (Step 5) — cmd/scale.go; admin revocation list; audit; tests |
| `group admin add/remove/list` nested subcommands | ✓ done | Session 26 (Step 6) — groupAdminCmd under groupCmd; group add/remove uses CanManageGroup |
| `show` ADMIN column when GroupAdminsEnabled | ✓ done | Session 26 (Step 6) — cmd/show.go GROUPS table |
| `init` opt-in group admin prompt (TTY only) | ✓ done | Session 26 (Step 7) — cmd/init.go:94; output.Confirm; non-interactive skips |
| Full authorization matrix tests | ✓ done | Session 26 (Step 8) — all role/command combinations; ErrWrongGroup path; privilege escalation guard |

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
| `gitcollect audit <collection> --from <YYYY-MM-DD>` | ✓ done | Session 25 (FEATURE_GAPS_3 B5) |
| `gitcollect audit <collection> --to <YYYY-MM-DD>` | ✓ done | Session 25 (FEATURE_GAPS_3 B5) |
| `gitcollect audit <collection> --action <action>` | ✓ done | Session 25 (FEATURE_GAPS_3 B5) |

**Notable design decisions:**
- `--since` accepts only five exact values: `1h`, `24h`, `7d`, `30d`, `90d` (changed from flexible parser in Session 11)
- `--from`/`--to` are mutually exclusive with `--since`; dates parsed as YYYY-MM-DD; `--to` is inclusive through end-of-day
- `--action` filter is case-insensitive; implemented in `audit.FilterByAction`
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
| `gitcollect pull <collection> --prune` | ✓ done | Session 17 (FEATURE_GAPS_2 B4) |
| `gitcollect pull <collection> --dry-run` | ✓ done | Session 17 (FEATURE_GAPS_2 B4) |
| `gitcollect diff <collection>` | ✓ done | Session 17 (FEATURE_GAPS_2 B1) |
| `gitcollect diff <collection> --repos-only` | ✓ done | Session 17 (FEATURE_GAPS_2 B1) |
| `gitcollect diff <collection> --members-only` | ✓ done | Session 17 (FEATURE_GAPS_2 B1) |
| `gitcollect move <src> <repo> <dest>` | ✓ done | Session 17 (FEATURE_GAPS_2 B2) |
| `gitcollect status <collection> --verbose` | ✓ done | Session 25 (FEATURE_GAPS_3 B3) |
| `gitcollect status --all` | ✓ done | Session 25 (FEATURE_GAPS_3 B4) |
| `gitcollect pull --all` | ✓ done | Session 25 (FEATURE_GAPS_3 B4) |
| `gitcollect sync --all` | ✓ done | Session 25 (FEATURE_GAPS_3 B4) |

**Notable design decisions:**
- `clone` double-checks platform collaborator status via API before cloning — local manifest alone is not sufficient
- `clone` warns when a skipped repo is a pending GitHub invite rather than a genuine denial (Session 11)
- `--pick` changed from comma-separated to space-separated in Session 11 (`--pick "r1 r2"` or `--pick r1 --pick r2`)
- `sync` clones missing repos + pulls existing ones in one pass; concurrent (default 4), reuses `cloneOne`
- `clone` and `sync` print absolute destination path after completion (FEATURE_GAPS_2 A3)
- `pull --prune` checks remote.origin.url before deleting any directory; never deletes dirs with uncommitted changes
- `diff` is read-only; exit 0 on drift, exit 1 on API error
- All git operations accessible only to users who actually have collaborator access on the platform
- `status` default mode shows REPO/STATUS/BEHIND table; `--verbose` shows raw `git status --short` per repo; `git.CommitsBehind` returns 0 (not error) when no remote
- `--all` on pull/status/sync iterates `collection.List()`, skips collections without valid credentials, runs operation sequentially with per-collection header; `syncAll()` function renamed `syncTargets()` to avoid name conflict with the `--all` bool flag

---

### Organisation import
| Command | Status | Added |
|---|---|---|
| `gitcollect import --from github\|gitlab --org <org>` | ✓ done | Session 19 (FEATURE_IMPORT) |
| `gitcollect import --dry-run` | ✓ done | Session 19 |
| `gitcollect import --team <slug>` | ✓ done | Session 19 |
| `gitcollect publish --repo <org/repo>` | ✓ done | Session 19 |
| `gitcollect pull-config --repo <org/repo>` | ✓ done | Session 19 |
| `gitcollect join --org <org> --team <team>` | ✓ done | Session 19 |
| `gitcollect sync-config [<collection>]` | ✓ done | Session 19 |

**Notable design decisions:**
- Import reads GitHub Teams API; one collection per team; concurrent (max 4)
- Conflict resolution: merge/overwrite/skip existing collections
- `publish`/`pull-config` use subprocess git calls (no git library)
- `join` is the new-hire onboarding command: fetch config + optional clone in one step
- `sync-config` re-fetches team state from GitHub and updates local YAML

### Offline and diagnostics
| Feature | Status | Added |
|---|---|---|
| `--offline` persistent flag on root command | ✓ done | Session 17 (FEATURE_GAPS_2 B5) |
| `requiresAuth` pre-flight helper | ✓ done | FEATURE_GAPS A1 |
| `printRemovalImpact` / `printDeletionImpact` / `printVisibilityImpact` | ✓ done | FEATURE_GAPS A2 — cmd/member.go:195, cmd/delete.go:37, cmd/visibility.go:27 |
| `--dry-run` on delete | ✓ done | FEATURE_GAPS A3 — cmd/delete.go:31 (deleteDryRun) |
| `--dry-run` on member remove | ✓ done | FEATURE_GAPS A3 — cmd/member.go:50 (memberRemoveDryRun) |
| Long field enrichment on 20 commands | ✓ done | Session 26 (FEATURE_GAPS B1) — clone, member add/remove, init, auth, show, group add, repo access, inspect, audit, sync, pull, add, remove, list, delete, visibility, whoami, version, root |
| `gitcollect concepts` command | ✓ done | Session 26 (FEATURE_GAPS B1) — cmd/help_concepts.go; root-level subcommand; 3 tests |
| `gitcollect doctor` | ✓ done | Session 26 (FEATURE_GAPS B2) — cmd/doctor.go; auth per host, staleness warn, token scopes, --json, exit-code; 6 tests |
| Sync suggestion after partial clone failure | ✓ done | Session 26 (FEATURE_GAPS B3) — output.Suggestion in failed>0 block in clone.go; 3 tests |
| `gitcollect verify` | ✓ done | Session 26 (FEATURE_GAPS B4) — cmd/verify.go; ok/archived/not_found/forbidden; --json/--fix; concurrent (max 4); 6 tests |
| `gitcollect export` | ✓ done | Session 26 (FEATURE_GAPS C1) — cmd/export.go; --all YAML multi-doc; --json; pure stdout; 6 tests |
| Audit Long field portability note | ✓ done | Session 26 (FEATURE_GAPS B5/C2) — cmd/audit.go Long field updated |
| GitLab walkthrough section in docs | ✓ done | Session 26 (FEATURE_GAPS D1) — docs/index.html GitLab walkthrough subsection + platform differences note box |
| `retryDo` with 429 backoff | ✓ done | FEATURE_GAPS D2 — internal/api/github.go:322 (doWithRetry with Retry-After, backoff, context cancel, 5 tests); SearchRepos uses c.do() which calls retryDo — confirmed covered |

### List and CLI improvements (FEATURE_GAPS_3 — complete)
| Feature | Status | Added |
|---|---|---|
| `gitcollect list` shows description column | ✓ done | Session 23 (FEATURE_GAPS_3 A1) — cmd/list.go:listRow.Description, truncateDesc helper |
| Archived repo warning on `gitcollect add` | ✓ done | Session 25 (FEATURE_GAPS_3 A2) — ensureRepoExists returns archived bool; addIsTerminalFn injectable |
| `gitcollect clone` skips already-cloned dirs | ✓ done | Session 25 (FEATURE_GAPS_3 A3) — os.Stat destPath check; skippedCount; sync suggestion |
| `gitcollect version --json` | ✓ done | Session 25 (FEATURE_GAPS_3 A4) — versionInfo struct; versionJSON flag |
| Completion activation hint | ✓ done | Session 25 (FEATURE_GAPS_3 A5) — cmd/completion.go; TTY vs piped hint to stderr |
| `gitcollect find <repo>` | ✓ done | Session 25 (FEATURE_GAPS_3 B1) — offline search across all collections; --json |
| `gitcollect describe <collection> [desc]` | ✓ done | Session 25 (FEATURE_GAPS_3 B2) — owner-only; empty arg clears; audit entry |
| `gitcollect status` summary mode + `--verbose` | ✓ done | Session 25 (FEATURE_GAPS_3 B3) — REPO/STATUS/BEHIND table; git.CommitsBehind |
| `--all` on pull / status / sync | ✓ done | Session 25 (FEATURE_GAPS_3 B4) — iterates collection.List(); skips inaccessible |
| `gitcollect audit --from/--to/--action` | ✓ done | Session 25 (FEATURE_GAPS_3 B5) — FilterByDate; FilterByAction; --since mutual exclusion |

### System
| Command | Status | Added |
|---|---|---|
| `gitcollect version` | ✓ done | Session 2 |
| `gitcollect version --json` | ✓ done | Session 25 (FEATURE_GAPS_3 A4) |
| `gitcollect completion bash\|zsh\|fish\|powershell` | ✓ done | Session 25 (FEATURE_GAPS_3 A5 — custom cmd/completion.go with TTY hint) |

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
| 14 | 2026-07-01 | PRE_SHIP_IMPROVEMENTS: cmd test coverage (root, clone, member, show, list, auth); removed repo grant/revoke; Namespace + RepoNamespace; --namespace on init; --since valid values error; --pick help text; [EXPERIMENTAL] on activity; Windows + upgrade docs in README |
| 15 | 2026-07-01 | FEATURE_AUTO_CREATE_REPO: CreateRepo in API layer (GitHub + GitLab); ErrNameConflict; ensureRepoExists helper; --new-repo-visibility flag; 10 cmd/add tests; API tests |
| 16 | 2026-07-01 | FEATURE_GAPS partial: requiresAuth helper + call sites; SyncCollaborators progress output; docs/index.html updates (namespace, identity model, experimental badge, install section, footer links) |
| 17 | 2026-07-02 | FEATURE_GAPS_2: rename, copy, location summary, whoami --check; diff, move, SearchRepos, add --pattern/--topic; pull --prune; --offline flag |
| 18 | 2026-07-02 | FEATURE_SCALABILITY: GroupAdminsEnabled/GroupAdmins fields; IsGroupAdmin/CanManageGroup/GroupAdminOf; transfer command; scale command; group admin subcommands; authorization matrix |
| 19 | 2026-07-03 | FEATURE_IMPORT Phase 1: ListOrgTeams/ListTeamMembers/ListTeamRepos/GetTokenScopes/paginate in API; import, publish, pull-config, join, sync-config commands |
| 20 | 2026-07-03 | FEATURE_IMPORT Phase 2: README rewrite with full command reference; docs/index.html organisation import section |
| 21 | 2026-07-04 | HasUncommittedChanges in git.go; pull --prune improvements; --offline B5 completion |
| 22 | 2026-07-05 | Audit session: verified completion across prompt files; updated PROGRESS.md; generated MANUAL.md. Session 22 incorrectly stated A2/A3 were not implemented and D2 was a passthrough — all three were already done. |
| 23 | 2026-07-05 | Full codebase audit — discovered structure from filesystem and binary. Cross-referenced all 10 prompt files against actual codebase. Corrected 3 stale Session 22 entries (A2, A3, D2). Completed FEATURE_GAPS_3 A1 (list description column). Rewrote PROGRESS.md from ground truth. Completed: 30 / Partial: 3 / Todo: 14. Agent additions found: 5. |
| 24 | 2026-07-05 | Confirmed Session 23 audit findings as accurate and complete. Rewrote PROGRESS.md with priority-ordered remaining work and annotated agent additions (all 7 kept). Completed: 30 / Partial: 3 / Todo: 14. Agent additions kept: 7. Next session: continue FEATURE_GAPS_3.md Group A item A2. |
| 25 | 2026-07-05 | FEATURE_GAPS_3.md complete — A0: output.Dim confirmed + test · A1: list description column · A2: add archived repo warning · A3: clone skip existing directories · A4: version --json flag · A5: completion hint (TTY detection) · B1: gitcollect find command · B2: gitcollect describe command · B3: status summary mode + CommitsBehind · B4: --all flag on pull/status/sync · B5: audit --from/--to/--action filters. cmd coverage: 62.8%. Next session: FEATURE_AUTO_CREATE_REPO.md |
| 27 | 2026-07-08 | SCOPED_COMPLETION_CHECK.md — all 13 prompt files verified. Gaps found and closed: B1 help_concepts command + root Long QUICK START + Long fields on 14 commands · B2 doctor command (auth/staleness/scopes/--json/exit-codes/6 tests) · B3 sync suggestion on clone failure (fixed condition from skippedCount to failed + 3 tests) · B4 verify command (ok/archived/not_found/forbidden/--json/--fix/concurrent/6 tests) · C1 export command (--all YAML multi-doc/--json/pure stdout/6 tests) · C2 audit Long field portability note · D1 GitLab walkthrough + glpat platform differences note in docs/index.html. Files 1-3/5-13 confirmed integrated with grep evidence. cmd coverage: 60.9% · 37/39 cmd files have test files. |
| 26 | 2026-07-06 | FEATURE_GAPS.md + FEATURE_SCALABILITY.md complete — Phase 1: concepts command, Long enrichment (20 commands), doctor (auth/staleness/scopes/--json/exit-code), verify (ok/archived/not_found/forbidden/--json/--fix/concurrent), export (--all YAML multi-doc/--json/pure stdout), sync suggestion on clone failure, GitLab docs walkthrough + platform differences note, audit Long field portability note · Phase 2: IsGroupAdmin/CanManageGroup/GroupAdminOf, all 4 sentinel errors (ErrGroupAdminsDisabled/ErrWrongGroup/ErrSelfTransfer/ErrAdminPrivilegeEscalation), RemoveMember/DeleteGroup GroupAdmins cleanup, transfer (typed confirm/previous-owner-as-member/audit/7 tests), scale (organisation/team/revocation list/audit), group admin subcommands (add/remove/list), show ADMIN column, init opt-in prompt (TTY only), authorization matrix tests · Agent additions: ErrNotOwnerOrGroupAdmin, doctorCheckFn, verifyCheckFn (all kept) · cmd coverage: 71.4% · Next session: FEATURE_IMPORT.md |

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
| `init --owner <org>` | Superseded by `--namespace` (PRE_SHIP Priority 3): namespace controls API path-building; owner remains the authenticated user | 11 / PRE_SHIP |
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

Measured with `go test ./... -cover` at `2ebb3bf` (v3.3.0). These replace an
earlier set of figures that had drifted: several were recorded above the
80% bar that the code no longer meets, so the table was reporting a
standard rather than a measurement.

| Package | Coverage | Against 80% bar | Notes |
|---|---|---|---|
| `internal/access` | 100.0% | pass | Full access matrix, owner bypass, inconclusive-check paths |
| `internal/output` | 95.2% | pass | Table, JSON, confirm, prompt, stale/invite warnings |
| `internal/audit` | 87.2% | pass | Append, read, Filter, FilterByDate, FilterByAction |
| `internal/activity` | 84.4% | pass | Append, read, filter, dedup |
| `internal/selfupdate` | 75.9% | **below** | Added v3.2.0; download/verify/extract/replace covered, error branches thinner |
| `internal/config` | 73.9% | **below** | Token, user, ID cache; directory paths |
| `internal/collection` | 67.7% | **below** | Mutations, access logic, rollback paths |
| `internal/git` | 66.7% | **below** | Clone, Pull, PullWithSummary, CommitsBehind (fake-git harness) |
| `internal/api` | 59.3% | **below** | GitHub + GitLab against `httptest.Server`; the largest gap |
| `cmd` | 67.4% | n/a | 44 command files, all with a test file |

`internal/api` is the one worth attention: it is the largest package, it is
where every platform call lives, and its uncovered half is mostly error and
pagination branches — exactly where the last three API defects were found.

---

## Remaining work

All 13 SCOPED_COMPLETION_CHECK.md prompt files have been verified and gaps closed as of Session 27.

### Uncovered command files (no test file)
None. All 44 files in `cmd/` have a corresponding `_test.go`. The three
previously listed here (`inspect`, `remove`, `repo`) now have test files;
the entry also miscounted, saying "2 untested" while naming three.

### Optional
- APPLY_SAMPLE_THEME.md / DOCS_THEME_REDESIGN.md — both effectively applied (lime #C8FF57 / Space Grotesk / JetBrains Mono theme is live in docs/index.html). No further visual changes pending unless explicitly requested.

---

## Agent additions (all KEEP)

These are implementation choices the agent made independently, confirmed as legitimate improvements in Session 24.

| Location | What | Decision | Reason |
|---|---|---|---|
| internal/api/github.go:322 | `doWithRetry` | ✓ KEEP | Better name than `retryDo`; fully correct — Retry-After header, backoff, context cancel, 5 tests |
| cmd/activity.go | `activity` command | ✓ KEEP | Working feature, marked `[EXPERIMENTAL]` in help text |
| internal/collection/collection.go:343 | `SetPath()` | ✓ KEEP | Used by rename and import commands; legitimate addition |
| internal/output/output.go:58 | `output.Dim` | ✓ KEEP | Was referenced in spec; agent added it correctly |
| internal/output/output.go:74 | `ClearProgress` | ✓ KEEP | Used by SyncCollaborators progress output |
| internal/collection/collection.go:418 | `MemberIDs()` | ✓ KEEP | Used by move command; legitimate |
| internal/collection/collection.go:428 | `SaveAs()` | ✓ KEEP | Used by rename and copy commands; legitimate |
| internal/collection/access.go | `ErrNotOwnerOrGroupAdmin` | ✓ KEEP | Better distinction than ErrWrongGroup for commands where caller is neither owner nor group admin of any group; improves error message clarity |
| cmd/doctor.go | `doctorCheckFn` injectable | ✓ KEEP | Correct testability pattern (same as addIsTerminalFn, addConfirmFn); allows 6 doctor tests to run without real token/network |
| cmd/verify.go | `verifyCheckFn` injectable | ✓ KEEP | Correct testability pattern; allows 6 verify tests to run against mock API responses without real collections |
| cmd/clone_test.go | `failingGetRepoMock` type | ✓ KEEP | Wraps multiAddMock to simulate per-repo GetRepo failures; enables B3 clone failure tests without real git processes |

---

## Defect register

Verified against the tree at `2ebb3bf` (v3.3.0). The previous entry read
"No currently open bugs"; that had gone stale — nine defects have been
found and fixed since, and several verified gaps remain open below.

Each closed entry names the commit, so the claim can be checked rather
than taken on trust. Every SHA below was verified to be an ancestor of
`main` and of the release named beside it.

A caution on those SHAs: this history has been rewritten more than once to
reassign authorship, and each rewrite silently invalidated every commit ID
recorded here. If it is rewritten again, re-resolve these by commit subject
rather than trusting the IDs — a stale SHA in a defect register is worse
than none, because it reads as evidence while resolving to nothing.

### Closed — fixed and released

| # | Defect | Impact | Fixed in | First release |
|---|---|---|---|---|
| D1 | Module path lacked the `/v3` suffix | `go install ...@latest` silently served **v1.0.0**; v2.0.0–v3.0.0 were never installable | `f4b0f10` | v3.1.0 |
| D2 | Release tagged on a commit reachable from no branch | Released code absent from `main` and from a fresh clone | `3d2823c` | v3.1.0 |
| D3 | `git fetch --depth=0` in the release gate | `fatal: depth 0 is not a positive number`; under `bash -e` this would have blocked **every** release | `3d2823c` | v3.1.0 |
| D4 | `ListCommits` silently capped at 100 | `activity --limit 250` returned 100 and reported success; the recorded activity log quietly omitted the rest. GitHub and GitLab *clamp* `per_page` rather than rejecting it, so nothing errored | `feda7eb` | v3.2.0 |
| D5 | No `fsync` before rename in the three atomic writers | `rename` makes the directory entry atomic, not the contents. A crash before writeback left a correctly-named file holding zero bytes — losing stored auth tokens, a whole collection manifest, or the binary itself | `439d5cf` | v3.2.0 |
| D6 | `get-update` misread a source build as a release | Go stamps a pseudo-version (`v3.0.2-0.20260914175157-...+dirty`), not `"dev"`; it parsed as 3.0.2 and compared *older*, so the guard meant to protect a developer's uncommitted build offered to overwrite it | `79040f3` | v3.2.0 |
| D7 | `scan` grouped on the first name segment only | `china-pricing`, `eu-pricing`, `us-pricing` — one team's module — became three collections named "china", "eu", "us", scattering the module the operator was assembling | `02c1c00` | v3.3.0 |
| D8 | Exhausted rate limit reported as "insufficient permissions" | GitHub answers a spent *primary* limit with **403 + `X-RateLimit-Remaining: 0`**, not 429. Users were sent to audit token scopes when the answer was to wait. Not a corner case: sync costs one call per member-repo pair | `2a878a2` | v3.3.0 |
| D9 | An unanswerable platform check was treated as a denial | Any 403 — spent quota, missing scope, refused endpoint — blocked the caller. `FilterAccessible` abandoned the whole listing on the first refusal, so one unverifiable repo made `status`, `clone`, `sync`, `activity`, `pr` and `health` report nothing. Failed asymmetrically: an owner's admin token answers fine, a read-only member's may not | `9ce9b5c` | v3.3.0 |

### Open — verified, not fixed

| # | Gap | Evidence | Notes |
|---|---|---|---|
| O1 | No hierarchy | `Collection` has no parent/child field; `Groups` maps a name to *people*, not repos | `e-commerce → pricing → china-pricing` cannot be represented. Needs a schema change plus a YAML migration, touching sync and access resolution |
| O2 | Sync is O(members × repos) | `Collection.SyncCollaborators` builds one job per member-repo pair | 30 members × 20 repos = 600 calls per sync against a 5000/hr limit. D8 exists because of this |
| O3 | Grants are individual collaborators, not teams | every grant is `AddCollaborator` | Inside an org this sits outside the team structure, costs one call per pair, and sends an invite per person per repo. `PUT /orgs/{org}/teams/{team}/repos/...` would be one call regardless of member count |
| O4 | One namespace per collection | `RepoNamespace()` returns `Namespace` or the owner's login | A collection cannot span a personal account *and* an org, which limits "sort all my repos by project" |
| O5 | Re-pushing an existing tag fails the release workflow | runs `36784953053` / `36784986666`: `verify tag` passed, goreleaser failed with `422 ... Code:already_exists` | Harmless — both releases were already published intact — but it reports a red run for a no-op. The workflow should treat an existing release for the tag as a clean skip |
| O7 | Hard-coded version samples in README and docs go stale at every release | `README.md` and `docs/index.html` both showed `gitcollect v3.1.0` two releases after v3.1.0; the same entry previously showed `v1.0.0`, which was worse, since v1.0.0 is the symptom of installing without the `/v3` suffix | Refreshed by hand each time so far. A release step that rewrites the samples, or samples written as an explicit placeholder, would stop the recurrence |
| O6 | Five packages below the stated 80% coverage bar | see the coverage table above | `internal/api` at 59.3% is the notable one: its uncovered half is mostly error and pagination branches, which is where D4, D8 and D9 all lived |

### Open — unverified

| # | Suspected | Why it is still open |
|---|---|---|
| U1 | GitHub's collaborator-check endpoint may require more than the read access gitcollect grants members | Confirming it needs a second account and a private repo; `docs.github.com` was unreachable from the environment used. D9 makes the failure survivable either way — members now get a warning and working commands instead of a lockout — but whether it fires in practice is untested. **To test:** add a second account as a read-only member and run `gitcollect clone` as them; a "Could not confirm platform access" warning means it is real |

### Release history note

`v3.2.0` was tagged at `caa546c` while later work was already on `main`, so
D7, D8, D9 and the `scan`/`move` features missed it and shipped in `v3.3.0`
instead. `v3.2.0` itself is intact and published. The tag was not re-pointed:
moving a tag detaches its GitHub release and drafts it, which had already
happened once to `v3.0.0` earlier in the same session.
