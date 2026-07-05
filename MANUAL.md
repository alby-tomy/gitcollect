# gitcollect — Command Manual

> Generated from binary output on 2026-07-05.
> Source of truth for all commands and parameters.
> For design rationale see PROMPT_v2.md.
> For the docs website see https://alby-tomy.github.io/gitcollect/

---

## Quick reference

| Command | Description | Auth required | Owner only |
|---------|-------------|---------------|------------|
| auth | Authenticate with GitHub or GitLab and store the access token | no | no |
| whoami | Show the authenticated user for each host you've run gitcollect auth on | no | no |
| init | Create a new collection | yes | — |
| delete | Delete a collection and revoke all access to its repos | yes | yes |
| list | List your collections (owned + member) | no | no |
| show | Show a summary of a collection: repos, members, and groups | no | no |
| visibility | Change a collection's visibility | yes | yes |
| rename | Rename a collection | yes | yes |
| copy | Copy a collection to a new name | yes | no (read) |
| transfer | Transfer collection ownership to another member | yes | yes |
| scale | Switch a collection between TEAM and ORGANISATION tiers | yes | yes |
| add | Add repos to a collection, open to all members by default | yes | yes |
| remove | Remove a repo from a collection and revoke everyone's access to it | yes | yes |
| repo access | Restrict or open up who can access a repo | yes | yes |
| repo show | Show who can access a repo and why | yes | no |
| member add | Add one or more members to a collection | yes | yes |
| member remove | Remove a member from a collection and revoke all their access | yes | yes |
| member list | List members and their group memberships | yes | no |
| group create | Create a group | yes | yes |
| group delete | Delete a group (blocked if any repo still uses it) | yes | yes |
| group add | Add one or more members to a group | yes | owner or group admin |
| group remove | Remove a member from a group | yes | owner or group admin |
| group list | List groups and their members | yes | no |
| group show | Show a group's members and the repos they can reach | yes | no |
| group admin add | Grant a member group admin rights for a specific group (owner-only) | yes | yes |
| group admin remove | Revoke group admin rights for a specific group | yes | yes or self |
| group admin list | List all group admin assignments in a collection | yes | no |
| inspect | Show access decisions for a user, a repo, or the full collection matrix | yes | no |
| audit | Show the access change log for a collection | no | no |
| activity | Show commits across a collection's repos, fetched live from the platform | yes | no |
| clone | Clone every repo you can access in a collection | yes | no |
| pull | git pull inside every accessible repo that's already cloned | yes | no |
| sync | Clone every repo not yet present locally, pull every repo that already is | yes | no |
| status | git status inside every accessible repo that's already cloned | yes | no |
| diff | Compare a local collection against GitHub/GitLab reality | yes | no |
| move | Move a repo from one collection to another | yes | yes (both) |
| import | Import GitHub/GitLab org structure as gitcollect collections | yes | — |
| publish | Push collection files to a shared git repository | yes | — |
| pull-config | Fetch collection files from a shared git repository | yes | — |
| join | New-hire onboarding: fetch your team's config and clone repos in one step | yes | — |
| sync-config | Refresh local collections from the platform org state | yes | — |
| version | Print version and platform information | no | no |
| completion | Generate the autocompletion script for the specified shell | no | no |

---

## Authentication

### gitcollect auth

**What it does:** Prompts for a personal access token (input is hidden) and verifies it against the platform API before storing it under `~/.gitcollect/config`.

**Usage:**
```
gitcollect auth [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--host` | `github.com` | platform host to authenticate (e.g. github.com, gitlab.com) |

**Example:**
```bash
gitcollect auth
gitcollect auth --host gitlab.com
gitcollect auth --host gitlab.company.com
```

**Notes:** Token is stored at `~/.gitcollect/config` (mode 0600). Run again with `--host` to add more platforms. Re-run to replace a revoked token.

**See also:** whoami

---

### gitcollect whoami

**What it does:** Shows the authenticated user for each host you have run `gitcollect auth` on.

**Usage:**
```
gitcollect whoami [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--check` | false | exit with code 1 if any stored token is rejected by the platform |
| `--json` | false | machine-readable output |

**Example:**
```bash
gitcollect whoami
gitcollect whoami --check
gitcollect whoami --json
```

**Notes:** Without `--check`, reads only the local config cache — no network call. With `--check`, makes a live API call to verify each stored token is still valid.

**See also:** auth

---

## Collection lifecycle

### gitcollect init

**What it does:** Creates a new collection. Private by default. The collection owner is always the authenticated user.

**Usage:**
```
gitcollect init <name> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--description` | `""` | human-readable description |
| `--host` | `github.com` | platform host the repos live on |
| `--namespace` | *(your login)* | org or username under which the repos live |
| `--public` | false | create as public instead of private |

**Example:**
```bash
gitcollect init cybersecurity
gitcollect init cybersecurity --namespace acme-corp
gitcollect init cybersecurity --host gitlab.com --description "pen-test tools"
```

**Notes:** Use `--namespace acme-corp` when the repos live under an org rather than your personal account. The collection owner (who administers it) is always the authenticated user regardless of namespace.

**See also:** delete, list, show

---

### gitcollect delete

**What it does:** Deletes a collection and revokes all platform collaborator access for every member on every repo in the collection. Requires typing the collection name to confirm.

**Usage:**
```
gitcollect delete <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | preview impact without deleting |

**Example:**
```bash
gitcollect delete cybersecurity
gitcollect delete cybersecurity --dry-run
```

**Notes:** Irreversible. Removes the local YAML file permanently. All platform access revocations are made before the file is deleted. Use `--dry-run` to see which members would have access revoked before committing.

**See also:** visibility, rename

---

### gitcollect list

**What it does:** Lists every collection you own or are a member of, reading only local manifests — no network calls.

**Usage:**
```
gitcollect list [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | machine-readable output |
| `--private` | false | show only private collections |
| `--public` | false | show only public collections |

**Example:**
```bash
gitcollect list
gitcollect list --private
gitcollect list --json
```

**Notes:** Works fully offline. Prints a stale warning for collections whose local YAML is more than 30 days old.

**See also:** show

---

### gitcollect show

**What it does:** Shows a summary of a collection: repos, members, and groups. The `YOU` column shows whether you can access each repo and why; the collection owner sees `WHO HAS ACCESS` instead.

**Usage:**
```
gitcollect show <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | machine-readable output |

**Example:**
```bash
gitcollect show cybersecurity
gitcollect show cybersecurity --json
```

**See also:** inspect, list

---

### gitcollect visibility

**What it does:** Changes a collection's visibility between public and private.

**Usage:**
```
gitcollect visibility <collection> <public|private> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect visibility cybersecurity public
gitcollect visibility cybersecurity private
```

**Notes:** Changing to public makes the collection's repo list visible to any gitcollect user. Changing back to private immediately re-restricts it.

---

### gitcollect rename

**What it does:** Renames a collection. Pure local operation — no API calls. Members, repos, and groups are unchanged.

**Usage:**
```
gitcollect rename <collection> <new-name> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect rename cybersecurity security-research
```

**Notes:** The new name must match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$` and must not already exist. An audit entry is written.

**See also:** copy

---

### gitcollect copy

**What it does:** Creates a new collection as a copy of an existing one. The caller becomes the owner of the new collection. No platform API calls — members need to be re-synced after copying.

**Usage:**
```
gitcollect copy <collection> <new-name> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect copy payments-team mobile-team
```

**Notes:** The new collection is always private regardless of the source's visibility. Platform collaborator access is not synced automatically — run `gitcollect member list <new-name>` to trigger sync.

**See also:** rename

---

### gitcollect transfer

**What it does:** Transfers ownership of a collection to another member. The previous owner becomes a regular member and retains access. Requires typing the new owner's username to confirm.

**Usage:**
```
gitcollect transfer <collection> <new-owner-username> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect transfer cybersecurity alice
```

**Notes:** The new owner must already be a collection member. Cannot transfer to yourself. An audit entry is written.

**See also:** scale

---

### gitcollect scale

**What it does:** Switches a collection between TEAM mode (owner manages everything) and ORGANISATION mode (group admins can manage their own groups independently).

**Usage:**
```
gitcollect scale <collection> organisation|team [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect scale cybersecurity organisation
gitcollect scale cybersecurity team
```

**Notes:** Switching to `team` while group admins are assigned shows who loses admin rights and requires confirmation. Switching to `organisation` does not assign any group admins — use `group admin add` afterwards.

**See also:** group admin add, transfer

---

## Repo management

### gitcollect add

**What it does:** Adds repos to a collection, open to all members by default. Supports individual repo names, bulk pattern search, or topic search.

**Usage:**
```
gitcollect add <collection> [repo...] [--pattern glob | --topic name] [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | show which repos would be added without adding them |
| `--limit` | 50 | max repos to add via search (max 100) |
| `--new-repo-visibility` | `"private"` | visibility for auto-created repos: `"public"` or `"private"` |
| `--org` | *(collection namespace)* | org to search in |
| `--pattern` | `""` | add all repos matching this name pattern (supports `*` wildcard) |
| `--topic` | `""` | add all repos with this GitHub topic |

**Example:**
```bash
gitcollect add cybersecurity pen-test-tools vuln-scanner
gitcollect add cybersecurity --pattern "payments-*" --org acme-corp
gitcollect add cybersecurity --topic security --dry-run
```

**Notes:** If a repo does not exist on the platform and the terminal is interactive, you will be prompted to create it. `--new-repo-visibility` controls the visibility of auto-created repos and is ignored for repos that already exist. `--pattern` and `--topic` use the GitHub Search API (rate limit: 30 req/min).

**See also:** remove, repo access

---

### gitcollect remove

**What it does:** Removes a repo from a collection and revokes everyone's platform collaborator access to it. Requires typing the repo name to confirm.

**Usage:**
```
gitcollect remove <collection> <repo> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect remove cybersecurity old-tool
```

**See also:** add, repo access

---

### gitcollect repo access

**What it does:** Restricts or opens up who can access a repo within a collection. Sets the complete list — does not append.

**Usage:**
```
gitcollect repo access <collection> <repo> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--groups` | *(none)* | restrict access to these groups (comma-separated) |
| `--open` | false | open the repo to all members |
| `--users` | *(none)* | restrict access to these individual users (comma-separated) |

**Example:**
```bash
gitcollect repo access cybersecurity vuln-scanner --groups red-team,sre
gitcollect repo access cybersecurity vuln-scanner --users alice,bob
gitcollect repo access cybersecurity vuln-scanner --open
```

**Notes:** `--groups` and `--users` are comma-separated. `--open` clears all restrictions. To set both groups and users, pass both flags together.

**See also:** repo show, inspect

---

### gitcollect repo show

**What it does:** Shows who can access a repo and why.

**Usage:**
```
gitcollect repo show <collection> <repo> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect repo show cybersecurity vuln-scanner
```

**See also:** repo access, inspect

---

## Member management

### gitcollect member add

**What it does:** Adds one or more members to a collection, syncing platform collaborator access across every repo they are entitled to.

**Usage:**
```
gitcollect member add <collection> <username> [username...] [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect member add cybersecurity alice
gitcollect member add cybersecurity alice bob charlie
```

**Notes:** On GitHub, newly granted repos send a collaborator invite email that must be accepted before cloning works. GitLab access is immediate. If any username in a batch fails, the others still proceed; all failures are reported at the end.

**See also:** member remove, member list

---

### gitcollect member remove

**What it does:** Removes a member from a collection and revokes all their platform access across every repo.

**Usage:**
```
gitcollect member remove <collection> <username> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--confirm-self` | false | required when removing your own username |
| `--dry-run` | false | preview impact without removing |

**Example:**
```bash
gitcollect member remove cybersecurity alice
gitcollect member remove cybersecurity alice --dry-run
gitcollect member remove cybersecurity alice --confirm-self
```

**See also:** member add, member list

---

### gitcollect member list

**What it does:** Lists members and their group memberships.

**Usage:**
```
gitcollect member list <collection> [flags]
```

**Flags:** none (beyond `-h`)

**Example:**
```bash
gitcollect member list cybersecurity
```

**See also:** member add, group list

---

## Group management

### gitcollect group create

**What it does:** Creates a new empty group in a collection.

**Usage:**
```
gitcollect group create <collection> <group> [flags]
```

**Example:**
```bash
gitcollect group create cybersecurity red-team
```

---

### gitcollect group delete

**What it does:** Deletes a group. Blocked if any repo still uses this group for access restrictions — clear those first with `repo access`.

**Usage:**
```
gitcollect group delete <collection> <group> [flags]
```

**Example:**
```bash
gitcollect group delete cybersecurity red-team
```

---

### gitcollect group add

**What it does:** Adds one or more members to a group. The user must already be a collection member.

**Usage:**
```
gitcollect group add <collection> <group> <username> [username...] [flags]
```

**Example:**
```bash
gitcollect group add cybersecurity red-team alice
gitcollect group add cybersecurity red-team alice bob
```

**Notes:** In ORGANISATION mode, a group admin of this group can run this command without being the collection owner.

---

### gitcollect group remove

**What it does:** Removes a member from a group (does not remove them from the collection).

**Usage:**
```
gitcollect group remove <collection> <group> <username> [flags]
```

**Example:**
```bash
gitcollect group remove cybersecurity red-team alice
```

---

### gitcollect group list

**What it does:** Lists all groups and their members.

**Usage:**
```
gitcollect group list <collection> [flags]
```

**Example:**
```bash
gitcollect group list cybersecurity
```

---

### gitcollect group show

**What it does:** Shows a group's members and the repos they can reach through that group.

**Usage:**
```
gitcollect group show <collection> <group> [flags]
```

**Example:**
```bash
gitcollect group show cybersecurity red-team
```

---

### gitcollect group admin add

**What it does:** Grants a member group admin rights for a specific group (owner-only). Group admins can add and remove members of their group independently.

**Usage:**
```
gitcollect group admin add <collection> <group> <username> [flags]
```

**Example:**
```bash
gitcollect group admin add cybersecurity red-team alice
```

**Notes:** Requires `gitcollect scale cybersecurity organisation` to be run first. Group admins can only manage their own group — not other groups, not collection members, not repo access.

**See also:** group admin remove, group admin list, scale

---

### gitcollect group admin remove

**What it does:** Revokes group admin rights for a specific group. The owner can remove any group admin; a group admin can remove themselves.

**Usage:**
```
gitcollect group admin remove <collection> <group> <username> [flags]
```

**Example:**
```bash
gitcollect group admin remove cybersecurity red-team alice
```

---

### gitcollect group admin list

**What it does:** Lists all group admin assignments in a collection.

**Usage:**
```
gitcollect group admin list <collection> [flags]
```

**Example:**
```bash
gitcollect group admin list cybersecurity
```

---

## Access inspection

### gitcollect inspect

**What it does:** Shows access decisions for the whole collection (member × repo matrix), one user, or one repo. Every denied entry includes the exact fix command.

**Usage:**
```
gitcollect inspect <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | machine-readable output |
| `--repo` | `""` | show who can access this repo and why |
| `--user` | `""` | show the full access map for this user |

**Example:**
```bash
gitcollect inspect cybersecurity
gitcollect inspect cybersecurity --user alice
gitcollect inspect cybersecurity --repo vuln-scanner
```

**Notes:** `--user` and `--repo` are mutually exclusive.

**See also:** show, audit

---

### gitcollect audit

**What it does:** Shows the access change log for a collection. Records every add, remove, and visibility change gitcollect has made.

**Usage:**
```
gitcollect audit <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | machine-readable output |
| `--since` | `""` | filter to entries within this duration: `1h`, `24h`, `7d`, `30d`, or `90d` |
| `--user` | `""` | filter to entries involving this user |

**Example:**
```bash
gitcollect audit cybersecurity
gitcollect audit cybersecurity --since 7d
gitcollect audit cybersecurity --user alice --json
```

**Notes:** Log is stored locally at `~/.gitcollect/audit/<collection>.log` (newline-delimited JSON, append-only). Only records changes gitcollect made — not changes made directly on GitHub/GitLab.

**See also:** inspect, diff

---

### gitcollect activity

**What it does:** Shows recent git commit activity across all accessible repos in a collection. Fetches commits from the platform API and caches results locally.

**Usage:**
```
gitcollect activity <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | machine-readable output |
| `--limit` | 10 | max commits to fetch per repo this run |
| `--repo` | `""` | show activity for only this repo |
| `--since` | `""` | filter to commits within this duration: `1h`, `24h`, `7d`, `30d`, or `90d` |

**Example:**
```bash
gitcollect activity cybersecurity
gitcollect activity cybersecurity --since 24h
gitcollect activity cybersecurity --repo pen-test-tools --limit 20
```

**Notes:** `[EXPERIMENTAL]` — output format and flag names may change in a future release. Caches fetched commits at `~/.gitcollect/activity/<collection>.log`; `--limit` bounds the live fetch only, not the cached history displayed.

**See also:** audit

---

## Git operations

### gitcollect clone

**What it does:** Clones every repo you can access in a collection. Verifies platform collaborator status before cloning — the local manifest alone is not sufficient.

**Usage:**
```
gitcollect clone <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--concurrency` | 4 | max repos to clone in parallel |
| `--dest` | `.` | directory to clone repos into |
| `--dry-run` | false | preview what would be cloned without doing it |
| `--pick` | *(none)* | clone only these repos, space-separated (e.g. `--pick "r1 r2"`) |

**Example:**
```bash
gitcollect clone cybersecurity
gitcollect clone cybersecurity --pick "pen-test-tools vuln-scanner"
gitcollect clone cybersecurity --dest ~/projects --dry-run
gitcollect clone cybersecurity --concurrency 8
```

**Notes:** `--pick` takes space-separated repo names in quotes, or can be repeated: `--pick r1 --pick r2`. Prints the absolute destination path after completion. On partial failure, suggests running `gitcollect sync` to retry.

**See also:** pull, sync

---

### gitcollect pull

**What it does:** Runs `git pull` inside every accessible repo that is already cloned locally.

**Usage:**
```
gitcollect pull <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--dest` | `.` | directory repos were cloned into |
| `--dry-run` | false | preview prune operations without executing |
| `--prune` | false | prompt to delete local clones of repos no longer in this collection |

**Example:**
```bash
gitcollect pull cybersecurity
gitcollect pull cybersecurity --prune
gitcollect pull cybersecurity --prune --dry-run
```

**Notes:** `--prune` only considers directories that (a) are git repos, (b) have a `remote.origin.url` matching this collection's namespace, and (c) are no longer in the collection. Never deletes a repo with uncommitted changes.

**See also:** clone, sync

---

### gitcollect sync

**What it does:** Clones every repo not yet present locally, and pulls every repo that already is — in a single pass and single access check.

**Usage:**
```
gitcollect sync <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--concurrency` | 4 | max repos to sync in parallel |
| `--dest` | `.` | directory to clone into, or where repos were already cloned |
| `--dry-run` | false | preview what would be cloned/pulled without doing it |

**Example:**
```bash
gitcollect sync cybersecurity
gitcollect sync cybersecurity --dest ~/projects
```

**See also:** clone, pull

---

### gitcollect status

**What it does:** Runs `git status` inside every accessible repo that is already cloned locally.

**Usage:**
```
gitcollect status <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--dest` | `.` | directory repos were cloned into |

**Example:**
```bash
gitcollect status cybersecurity
```

**See also:** pull, sync

---

### gitcollect diff

**What it does:** Compares the local collection manifest against the current state on GitHub/GitLab. Reports missing repos, archived repos, and members removed directly on the platform. Read-only — no changes are made.

**Usage:**
```
gitcollect diff <collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | machine-readable JSON output |
| `--members-only` | false | only compare members, skip repo check |
| `--repos-only` | false | only compare repos, skip member check |

**Example:**
```bash
gitcollect diff cybersecurity
gitcollect diff cybersecurity --repos-only
gitcollect diff cybersecurity --json
```

**Notes:** Exit 0 on drift (drift is informational), exit 1 on API error. Use `gitcollect sync-config` to apply the platform's current state back to the local collection.

**See also:** sync-config, audit

---

### gitcollect move

**What it does:** Moves a repo from one collection to another atomically. Grants access to members of the destination collection, revokes access from members of the source collection who are not in the destination.

**Usage:**
```
gitcollect move <source-collection> <repo> <dest-collection> [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | preview access changes without executing |

**Example:**
```bash
gitcollect move cybersecurity vuln-scanner red-team
gitcollect move cybersecurity vuln-scanner red-team --dry-run
```

**Notes:** Caller must be owner of both collections. Always shows the access diff before executing. Writes to the destination collection first; if that fails, nothing is changed.

**See also:** add, remove

---

## Organisation import

### gitcollect import

**What it does:** Reads an organisation's team structure from GitHub or GitLab and creates gitcollect collection files automatically — one per team.

**Usage:**
```
gitcollect import [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | preview what would be imported without writing files |
| `--flatten` | true | treat nested teams as top-level collections |
| `--from` | *(required)* | platform to import from: `github` or `gitlab` |
| `--merge` | false | merge imported data into existing collections without prompting |
| `--namespace` | *(org name)* | override namespace for all collections |
| `--org` | *(required)* | organisation or group name |
| `--overwrite` | false | overwrite existing collections without prompting |
| `--owner-from-maintainer` | true | set collection owner from team maintainer |
| `--skip-existing` | false | skip teams whose collection already exists |
| `--team` | `""` | import only this team slug (default: all teams) |

**Example:**
```bash
gitcollect import --from github --org acme-corp
gitcollect import --from github --org acme-corp --dry-run
gitcollect import --from github --org acme-corp --team payments-team
```

**Notes:** Requires `read:org` and `repo` (or `public_repo`) token scopes. Runs team data fetches concurrently (max 4). On conflict with an existing local collection, prompts for merge/overwrite/skip unless `--merge`, `--overwrite`, or `--skip-existing` is set.

**See also:** publish, sync-config

---

### gitcollect publish

**What it does:** Copies collection YAML files into a shared git repository so team members can fetch them with `gitcollect pull-config`.

**Usage:**
```
gitcollect publish [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--branch` | `main` | branch to push to |
| `--collection` | `""` | publish only this collection (default: all) |
| `--message` | `"update gitcollect collections"` | commit message |
| `--path` | `collections/` | directory in the repo to write collection files to |
| `--repo` | *(required)* | GitHub/GitLab repo to publish to (e.g. `acme-corp/gitcollect-config`) |

**Example:**
```bash
gitcollect publish --repo acme-corp/gitcollect-config
gitcollect publish --repo acme-corp/gitcollect-config --collection payments-team
```

**Notes:** The target repo must already exist. Uses subprocess git calls (clone, commit, push) — respects whatever git credential helper is configured on the system.

**See also:** pull-config, import

---

### gitcollect pull-config

**What it does:** Fetches collection files from a shared git repository and copies them to `~/.gitcollect/collections/`.

**Usage:**
```
gitcollect pull-config [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--branch` | `main` | branch to fetch from |
| `--collection` | `""` | fetch only this collection (default: all) |
| `--overwrite` | false | overwrite existing local collections |
| `--path` | `collections/` | directory in the repo that contains collection files |
| `--repo` | *(required)* | shared config repo to fetch from (e.g. `acme-corp/gitcollect-config`) |

**Example:**
```bash
gitcollect pull-config --repo acme-corp/gitcollect-config
gitcollect pull-config --repo acme-corp/gitcollect-config --collection payments-team
```

**See also:** publish, join

---

### gitcollect join

**What it does:** New-hire onboarding in one command: fetches your team's collection from a shared config repo (or directly from the GitHub API) and optionally clones every accessible repo.

**Usage:**
```
gitcollect join [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--clone` | false | clone repos immediately after joining |
| `--dest` | `.` | clone destination directory (used with `--clone`) |
| `--from` | `github` | platform to import from when `--repo` is not set |
| `--org` | *(required)* | GitHub org or GitLab group |
| `--repo` | `""` | shared config repo to fetch from; if not set, imports directly from the platform API |
| `--team` | *(required)* | your team slug |

**Example:**
```bash
gitcollect join --org acme-corp --team payments-team --clone
gitcollect join --org acme-corp --team payments-team --repo acme-corp/gitcollect-config --clone
```

**See also:** pull-config, clone

---

### gitcollect sync-config

**What it does:** For each local collection that has a namespace set, re-fetches the current team state from GitHub or GitLab, prints what changed, and updates the local YAML.

**Usage:**
```
gitcollect sync-config [<collection>] [flags]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--all` | false | sync all collections (default when no collection name is given) |
| `--dry-run` | false | show what would change without applying |
| `--from` | *(collection's host)* | platform to sync from: `github` or `gitlab` |
| `--org` | *(collection's namespace)* | org to sync from |

**Example:**
```bash
gitcollect sync-config payments-team
gitcollect sync-config payments-team --dry-run
gitcollect sync-config --all
```

**Notes:** Only works for collections that have a `namespace` field (collections created via `import` or `init --namespace`). After sync, run `gitcollect sync <collection>` to clone new repos.

**See also:** diff, import

---

## System

### gitcollect version

**What it does:** Prints version and platform information.

**Usage:**
```
gitcollect version [flags]
```

**Example:**
```bash
gitcollect version
```

**Output example:**
```
gitcollect dev windows/amd64
```

---

### gitcollect completion

**What it does:** Generates shell autocompletion scripts for bash, zsh, fish, or powershell.

**Usage:**
```
gitcollect completion [command]
```

**Subcommands:** bash, fish, powershell, zsh

**Example:**
```bash
# Bash
gitcollect completion bash >> ~/.bashrc && source ~/.bashrc

# Zsh
gitcollect completion zsh >> ~/.zshrc && source ~/.zshrc

# Fish
gitcollect completion fish > ~/.config/fish/completions/gitcollect.fish

# PowerShell
gitcollect completion powershell >> $PROFILE; . $PROFILE
```

---

## Commands not yet implemented

| Command | Planned in |
|---------|-----------|
| `gitcollect doctor` | FEATURE_GAPS.md (B2) |
| `gitcollect verify` | FEATURE_GAPS.md (B4) |
| `gitcollect export` | FEATURE_GAPS.md (C1) |
| `gitcollect concepts` | FEATURE_GAPS.md (B1) |
| `gitcollect find` | FEATURE_GAPS.md |
| `gitcollect describe` | FEATURE_GAPS.md |
