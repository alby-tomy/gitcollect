# gitcollect — Feature: Organisation Import + Docs Update

> Read this file completely before touching any code.
> This is a large feature. Read every section before writing anything.
> Verify baseline first. Work through the implementation order strictly.
> After every file: go build ./... and go test ./...
> Flag any deviation before implementing it, not after.
> Update PROMPT.md progress tracker before ending each session.
>
> This prompt covers two distinct phases:
> PHASE 1 — gitcollect import (and companion commands)
> PHASE 2 — Docs and README update to reflect all current features
> Complete Phase 1 fully before starting Phase 2.

---

## Context — why this feature exists

gitcollect currently assumes you start from scratch. Every collection
is created manually with gitcollect init, repos added one by one with
gitcollect add, members added one by one with gitcollect member add.

This works for individuals and small teams (2-20 people).

It is completely impractical for any existing organisation that already
has structure on GitHub or GitLab:
- 20-30 teams → 20-30 manual gitcollect init calls
- 100+ repos → 100+ gitcollect add calls
- 300+ members → 300+ gitcollect member add calls
  (each triggering a GitHub API call)

That setup would take days. Nobody does it. The tool gets evaluated
and abandoned before proving its value.

The import command reads the existing structure from GitHub/GitLab
via their APIs and creates gitcollect collections automatically.
An org with 30 teams, 100 repos, and 300 members can be fully
imported in under 2 minutes.

---

## Step 0 — Baseline verification

```bash
go build ./...
go test ./...
go vet ./...
```

All must be clean before writing a single line. Report output.

Also verify these specific things exist from prior sessions:
```bash
# Confirm namespace support exists (Priority 3 from pre-ship)
grep -n "RepoNamespace\|Namespace" internal/collection/collection.go | head -5

# Confirm identity model (Session 13)
grep -n "UserInfo\|GetUser\b" internal/api/client.go | head -5

# Confirm retryDo exists (Gap D2) or note if still to be built
grep -n "retryDo\|paginate" internal/api/github.go | head -5

# Check module path
head -3 go.mod
```

Report every finding. The import feature builds on all of these.

---

## PHASE 1 — Implementation

### New commands overview

```
gitcollect import     Read GitHub/GitLab org structure, create collections
gitcollect publish    Push collection files to a shared git repo
gitcollect pull-config  Fetch collection files from a shared git repo
gitcollect join       New-hire onboarding: fetch config + clone in one step
```

These four commands form a complete set. An org admin runs import
once and publish once. Each employee runs join once. After that,
pull-config keeps everyone in sync when the org structure changes.

---

### New API methods — internal/api/client.go

Add these to the Client interface. Read the existing interface first
to understand the existing patterns before adding.

```go
// ListOrgTeams returns all teams in the org, handling pagination.
// Requires read:org scope on GitHub.
// On GitLab: returns subgroups of the group.
ListOrgTeams(org string) ([]TeamInfo, error)

// ListTeamMembers returns members of a team.
// role: "" = all, "maintainer" = maintainers only, "member" = non-maintainers
// On GitLab: role maps to "owner"/"maintainer"/"developer"/etc.
ListTeamMembers(org, teamSlug, role string) ([]UserInfo, error)

// ListTeamRepos returns repos a team has access to, handling pagination.
// On GitLab: returns projects the subgroup has access to.
ListTeamRepos(org, teamSlug string) ([]RepoInfo, error)

// GetTokenScopes returns the OAuth scopes the current token has.
// GitHub: reads X-OAuth-Scopes response header from /user endpoint.
// GitLab: reads scope from /oauth/token/info endpoint.
// Used for pre-flight scope checking before import.
GetTokenScopes() ([]string, error)

// New type
type TeamInfo struct {
    ID          int64
    Name        string   // display name e.g. "Payments Team"
    Slug        string   // URL-safe e.g. "payments-team"
    Description string
    Privacy     string   // "closed" | "secret" (GitHub) | "private" (GitLab)
    ParentSlug  string   // non-empty for nested teams
}
```

### Pagination helper — internal/api/github.go

GitHub paginates all list endpoints at 100 items per page.
The next page URL is in the Link response header.
This helper must be used by all three list methods above.

```go
// paginate calls startURL repeatedly, following Link: rel="next" headers,
// until all pages are fetched. Calls fn with each page's raw response body.
// Applies the same 15-second timeout and retryDo logic as all other calls.
func (c *githubClient) paginate(startURL string, fn func([]byte) error) error {
    url := startURL
    for url != "" {
        req, err := http.NewRequestWithContext(
            context.WithTimeout(context.Background(), 15*time.Second),
            http.MethodGet, url, nil,
        )
        if err != nil {
            return err
        }
        req.Header.Set("Authorization", "Bearer "+c.token)
        req.Header.Set("Accept", "application/vnd.github.v3+json")

        resp, err := c.retryDo(req)
        if err != nil {
            return err
        }
        body, err := io.ReadAll(resp.Body)
        resp.Body.Close()
        if err != nil {
            return err
        }
        if resp.StatusCode != http.StatusOK {
            return classifyStatus(resp.StatusCode)
        }
        if err := fn(body); err != nil {
            return err
        }
        url = nextPageURL(resp.Header.Get("Link"))
    }
    return nil
}

// nextPageURL parses GitHub's Link header and returns the "next" URL.
// Returns "" when there is no next page.
// Link header format:
//   <https://api.github.com/...?page=2>; rel="next", <...>; rel="last"
func nextPageURL(link string) string {
    for _, part := range strings.Split(link, ",") {
        part = strings.TrimSpace(part)
        if strings.Contains(part, `rel="next"`) {
            parts := strings.SplitN(part, ";", 2)
            if len(parts) > 0 {
                return strings.Trim(strings.TrimSpace(parts[0]), "<>")
            }
        }
    }
    return ""
}
```

GitLab uses different pagination headers (`X-Next-Page`, `X-Total-Pages`).
Add an equivalent `paginateGitLab` helper in gitlab.go.

### GitHub implementation — internal/api/github.go

```go
func (c *githubClient) ListOrgTeams(org string) ([]TeamInfo, error) {
    url := fmt.Sprintf("%s/orgs/%s/teams?per_page=100", c.baseURL, org)
    var teams []TeamInfo
    err := c.paginate(url, func(body []byte) error {
        var page []struct {
            ID          int64  `json:"id"`
            Name        string `json:"name"`
            Slug        string `json:"slug"`
            Description string `json:"description"`
            Privacy     string `json:"privacy"`
            Parent      *struct {
                Slug string `json:"slug"`
            } `json:"parent"`
        }
        if err := json.Unmarshal(body, &page); err != nil {
            return err
        }
        for _, t := range page {
            ti := TeamInfo{
                ID:          t.ID,
                Name:        t.Name,
                Slug:        t.Slug,
                Description: t.Description,
                Privacy:     t.Privacy,
            }
            if t.Parent != nil {
                ti.ParentSlug = t.Parent.Slug
            }
            teams = append(teams, ti)
        }
        return nil
    })
    return teams, err
}

func (c *githubClient) ListTeamMembers(org, teamSlug, role string) ([]UserInfo, error) {
    url := fmt.Sprintf("%s/orgs/%s/teams/%s/members?per_page=100",
        c.baseURL, org, teamSlug)
    if role != "" {
        url += "&role=" + role
    }
    var members []UserInfo
    err := c.paginate(url, func(body []byte) error {
        var page []struct {
            ID    int64  `json:"id"`
            Login string `json:"login"`
        }
        if err := json.Unmarshal(body, &page); err != nil {
            return err
        }
        for _, m := range page {
            members = append(members, UserInfo{
                ID:    strconv.FormatInt(m.ID, 10),
                Login: m.Login,
            })
        }
        return nil
    })
    return members, err
}

func (c *githubClient) ListTeamRepos(org, teamSlug string) ([]RepoInfo, error) {
    url := fmt.Sprintf("%s/orgs/%s/teams/%s/repos?per_page=100",
        c.baseURL, org, teamSlug)
    var repos []RepoInfo
    err := c.paginate(url, func(body []byte) error {
        var page []struct {
            Name     string `json:"name"`
            CloneURL string `json:"clone_url"`
            Private  bool   `json:"private"`
            Archived bool   `json:"archived"`
        }
        if err := json.Unmarshal(body, &page); err != nil {
            return err
        }
        for _, r := range page {
            repos = append(repos, RepoInfo{
                Name:     r.Name,
                CloneURL: r.CloneURL,
                Private:  r.Private,
                Archived: r.Archived,
            })
        }
        return nil
    })
    return repos, err
}

func (c *githubClient) GetTokenScopes() ([]string, error) {
    // Make a lightweight call to /user and read the X-OAuth-Scopes header
    req, err := http.NewRequestWithContext(
        context.WithTimeout(context.Background(), 15*time.Second),
        http.MethodGet, c.baseURL+"/user", nil,
    )
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    resp, err := c.retryDo(req)
    if err != nil {
        return nil, err
    }
    resp.Body.Close()
    scopeHeader := resp.Header.Get("X-OAuth-Scopes")
    if scopeHeader == "" {
        return []string{}, nil
    }
    var scopes []string
    for _, s := range strings.Split(scopeHeader, ",") {
        scopes = append(scopes, strings.TrimSpace(s))
    }
    return scopes, nil
}
```

### Scope pre-flight check helper — cmd/import.go

```go
// checkImportScopes verifies the token has the scopes needed for import.
// Returns a clear, actionable error if any required scope is missing.
func checkImportScopes(client api.Client) error {
    scopes, err := client.GetTokenScopes()
    if err != nil {
        return fmt.Errorf("import: could not check token scopes: %w", err)
    }

    scopeSet := make(map[string]bool)
    for _, s := range scopes {
        scopeSet[s] = true
    }

    var missing []string
    // read:org is required to list teams and their members
    if !scopeSet["read:org"] {
        missing = append(missing, "read:org")
    }
    // repo is required for private repo access
    // public_repo is sufficient if only importing public repos
    if !scopeSet["repo"] && !scopeSet["public_repo"] {
        missing = append(missing, "repo (or public_repo for public-only orgs)")
    }

    if len(missing) > 0 {
        return fmt.Errorf(
            "import: GitHub token is missing required scopes: %s\n\n"+
                "  Generate a new token at:\n"+
                "  https://github.com/settings/tokens/new?scopes=read:org,repo\n\n"+
                "  Then run: gitcollect auth",
            strings.Join(missing, ", "),
        )
    }
    return nil
}
```

---

### cmd/import.go — the main import command

```
gitcollect import --from github|gitlab --org <orgname> [flags]
```

Flags:
```
--from github|gitlab     platform to import from (required)
--org <orgname>          organisation or group name (required)
--team <teamslug>        import only this team (optional, default: all)
--dry-run                preview what would be imported, no files written
--flatten                treat nested teams as top-level (default: true)
--owner-from-maintainer  set collection owner from team maintainer
                         (default: true, falls back to importer if no maintainer)
--namespace <name>       override namespace for all collections
                         (default: org name)
```

Full behaviour:

```
$ gitcollect import --from github --org acme-corp

Pre-flight checks...
✓ Authenticated as alby-tomy (github.com)
✓ Token scopes: read:org, repo
✓ Organisation acme-corp found

Fetching org structure...
  Teams:   30
  Repos:   147
  Members: 312 unique

Importing collections (30):

  [1/30]  payments-team      8 repos · 25 members · owner: payments-lead
  [2/30]  mobile-team       12 repos · 18 members · owner: mobile-lead
  [3/30]  devops-team       20 repos · 12 members · owner: devops-lead
  ...

✓ Imported 30 collections
  Collections written to ~/.gitcollect/collections/
  312 unique members across all collections
  147 repos across all collections

Next steps:
  gitcollect list                          see all imported collections
  gitcollect clone payments-team           clone your team's repos
  gitcollect publish --repo acme-corp/gitcollect-config
                                           share with your team
```

Dry-run output (no files written, shows exactly what would happen):

```
$ gitcollect import --from github --org acme-corp --dry-run

[dry-run] Would create 30 collections:

  payments-team      8 repos · 25 members · owner: payments-lead
  mobile-team       12 repos · 18 members · owner: mobile-lead
  ...

[dry-run] No files written.
Run without --dry-run to apply.
```

Single team import:

```
$ gitcollect import --from github --org acme-corp --team payments-team

✓ Imported payments-team
  8 repos · 25 members · owner: payments-lead
  Written to ~/.gitcollect/collections/payments-team.yaml
```

### Nested team handling

When `--flatten=true` (default):
- Import each team as a separate top-level collection
- Use the team slug as the collection name
- Ignore the parent-child relationship
- A team named "backend/payments" becomes collection "payments"
  (use the slug, not the full path)

When `--flatten=false`:
- Prefix the collection name with the parent slug
- "payments" team under "backend" parent → collection "backend-payments"
- Print a warning: nested hierarchy is preserved but
  gitcollect does not model parent-child collection relationships

Default is `--flatten=true`. Most orgs use team structure for
access grouping, not for hierarchy the way gitcollect would model it.

### Collection building from import data

```go
// buildCollectionFromTeam creates a Collection struct from the imported
// GitHub team data. Called once per team during import.
func buildCollectionFromTeam(
    team    api.TeamInfo,
    members []api.UserInfo,
    maintainers []api.UserInfo,
    repos   []api.RepoInfo,
    org     string,
    ownerFromMaintainer bool,
    callerID, callerLogin string,
) (*collection.Collection, error) {

    // Determine owner: first maintainer if available and flag is set,
    // otherwise fall back to the authenticated user running the import
    ownerID    := callerID
    ownerLogin := callerLogin
    if ownerFromMaintainer && len(maintainers) > 0 {
        ownerID    = maintainers[0].ID
        ownerLogin = maintainers[0].Login
    }

    // Build Logins cache: ID → login for all members + owner
    logins := make(map[string]string)
    logins[ownerID] = ownerLogin
    for _, m := range members {
        logins[m.ID] = m.Login
    }

    // Build member ID list (exclude the owner — owner is not also a member)
    var memberIDs []string
    for _, m := range members {
        if m.ID != ownerID {
            memberIDs = append(memberIDs, m.ID)
        }
    }

    // Build repos list (all open to all members by default on import)
    var repoAccess []collection.RepoAccess
    for _, r := range repos {
        if r.Archived {
            // Include archived repos but note them — still cloneable
            repoAccess = append(repoAccess, collection.RepoAccess{
                Name:   r.Name,
                Groups: []string{},
                Users:  []string{},
            })
        } else {
            repoAccess = append(repoAccess, collection.RepoAccess{
                Name:   r.Name,
                Groups: []string{},
                Users:  []string{},
            })
        }
    }

    col := &collection.Collection{
        Version:     collection.CurrentVersion,
        Name:        team.Slug,
        Description: team.Description,
        Host:        "github.com",
        Namespace:   org,
        Owner:       ownerID,
        Visibility:  collection.VisibilityPrivate,
        Members:     memberIDs,
        Groups:      map[string][]string{},
        Repos:       repoAccess,
        Logins:      logins,
        CreatedAt:   time.Now().UTC(),
        UpdatedAt:   time.Now().UTC(),
    }
    col.SetPath(org + "-" + team.Slug) // internal path setter

    return col, col.Validate()
}
```

### Concurrency during import

Import fetches data for multiple teams concurrently.
Use the existing semaphore pattern (max 4 parallel API calls):

```go
type importResult struct {
    team api.TeamInfo
    col  *collection.Collection
    err  error
}

results := make([]importResult, len(teams))
var wg sync.WaitGroup
sem := make(chan struct{}, 4)

for i, team := range teams {
    wg.Add(1)
    go func(i int, team api.TeamInfo) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()

        members, err := client.ListTeamMembers(org, team.Slug, "")
        if err != nil {
            results[i] = importResult{team: team, err: err}
            return
        }
        maintainers, err := client.ListTeamMembers(org, team.Slug, "maintainer")
        if err != nil {
            results[i] = importResult{team: team, err: err}
            return
        }
        repos, err := client.ListTeamRepos(org, team.Slug)
        if err != nil {
            results[i] = importResult{team: team, err: err}
            return
        }

        col, err := buildCollectionFromTeam(
            team, members, maintainers, repos,
            org, ownerFromMaintainer, callerID, callerLogin,
        )
        results[i] = importResult{team: team, col: col, err: err}
    }(i, team)
}
wg.Wait()
```

### Conflict handling during import

If a collection with the same name already exists locally:

```
⚠ Collection "payments-team" already exists locally.
  Local:  8 repos · 25 members · updated 3 days ago
  Import: 9 repos · 26 members (GitHub current state)

  [o] Overwrite local with imported data
  [s] Skip this collection
  [m] Merge — add new repos/members, keep existing ones
  Choice [o/s/m]: m
```

Default is merge when running non-interactively (`--merge` flag).
`--overwrite` flag skips the prompt and always overwrites.
`--skip-existing` flag skips the prompt and always skips.

---

### cmd/publish.go — share collections with your team

```
gitcollect publish --repo <org/repo> [flags]
```

Flags:
```
--repo <org/repo>        GitHub/GitLab repo to publish to (required)
--collection <name>      publish only this collection (default: all)
--branch <name>          branch to push to (default: main)
--path <dir>             directory in the repo (default: collections/)
--message <msg>          commit message (default: "update gitcollect collections")
```

What this does:
1. Reads all local collection YAML files (or just the named one)
2. Clones the target repo into a temp directory
3. Copies collection files to `collections/` in the repo
4. Commits and pushes

The target repo (e.g. `acme-corp/gitcollect-config`) is a regular
GitHub repo that the team admin has created. It can be private —
team members only need read access to fetch from it.

```
$ gitcollect publish --repo acme-corp/gitcollect-config

Publishing 30 collections to acme-corp/gitcollect-config...
  ✓ payments-team.yaml
  ✓ mobile-team.yaml
  ✓ devops-team.yaml
  ... 27 more

✓ Published 30 collections to acme-corp/gitcollect-config (main)
  Commit: "update gitcollect collections"

Share this with your team:
  gitcollect pull-config --repo acme-corp/gitcollect-config
```

Implementation note: use `git clone`, file copy, `git add`, `git commit`,
`git push` as subprocess calls — the same pattern already used in git.go.
Do not implement a git library. This keeps the implementation simple and
means it works with any git credential helper already set up on the system.

---

### cmd/pull_config.go — fetch collections from shared repo

```
gitcollect pull-config --repo <org/repo> [flags]
```

Flags:
```
--repo <org/repo>        shared config repo to fetch from (required)
--collection <name>      fetch only this collection (default: all)
--branch <name>          branch to fetch from (default: main)
--path <dir>             directory in repo (default: collections/)
--overwrite              overwrite existing local collections
```

What this does:
1. Clones or shallow-fetches the config repo into a temp directory
2. Copies collection YAML files to `~/.gitcollect/collections/`
3. Reports what was fetched

```
$ gitcollect pull-config --repo acme-corp/gitcollect-config

Fetching collections from acme-corp/gitcollect-config...
  ✓ payments-team     (new)
  ✓ mobile-team       (new)
  ✓ devops-team       (new)
  ... 27 more

✓ Fetched 30 collections

Your collections are ready. Clone your team's repos:
  gitcollect clone payments-team
```

---

### cmd/join.go — new hire onboarding in one command

```
gitcollect join --org <orgname> --team <teamname> [flags]
```

Flags:
```
--org <orgname>          GitHub org or GitLab group (required)
--team <teamname>        your team slug (required)
--repo <org/repo>        config repo to fetch from (optional,
                         if not set: imports directly from GitHub API)
--clone                  clone repos immediately after joining (default: false)
--dest <dir>             clone destination (default: current directory)
```

This is the new hire command. One command to go from nothing to
fully set up with the right repos cloned.

```
$ gitcollect join --org acme-corp --team payments-team --clone

Setting up gitcollect for acme-corp/payments-team...

✓ Authenticated as new-hire (github.com)

Fetching payments-team configuration...
✓ payments-team fetched (8 repos · 25 members)
  Written to ~/.gitcollect/collections/payments-team.yaml

Cloning repos...
[1/8] Cloning payments-api...          ✓ done (2.1s)
[2/8] Cloning payments-frontend...     ✓ done (1.4s)
[3/8] Cloning payments-db-migrations...✓ done (0.8s)
...

✓ Joined acme-corp/payments-team
  8 repos cloned to ./

Welcome to the team. Next steps:
  gitcollect show payments-team    see your full repo access
  gitcollect pull payments-team    pull updates any time
```

If `--repo` is provided, fetch the collection YAML from the shared
config repo. If not, import directly from the GitHub teams API
(requires the new member to have read:org scope on their token).

---

### cmd/sync_config.go — refresh collections from GitHub

```
gitcollect sync-config [<collection>] [flags]
```

Flags:
```
--all                    sync all collections (default if no collection given)
--from github|gitlab     platform to sync from (default: collection's host)
--org <orgname>          org to sync from (default: collection's namespace)
--dry-run                show what would change without applying
```

What this does:
1. For each local collection that has a namespace set (imported from an org)
2. Re-fetch the current team state from GitHub/GitLab
3. Compare with local collection
4. Print what changed (new members, removed members, new repos, removed repos)
5. Update the local collection YAML with current state

```
$ gitcollect sync-config payments-team

Syncing payments-team from github.com/acme-corp...

Changes detected:
  + new member: new-hire (id: 9988776)
  + new repo:   payments-notifications
  - removed member: former-employee (no longer in GitHub team)

Applying changes...
✓ payments-team synced

Run: gitcollect sync payments-team   to clone new repos and pull existing
```

No changes:
```
$ gitcollect sync-config payments-team

✓ payments-team is up to date (last synced: 2 minutes ago)
```

---

### New files

```
cmd/import.go           gitcollect import
cmd/import_test.go
cmd/publish.go          gitcollect publish
cmd/publish_test.go
cmd/pull_config.go      gitcollect pull-config
cmd/pull_config_test.go
cmd/join.go             gitcollect join
cmd/join_test.go
cmd/sync_config.go      gitcollect sync-config
cmd/sync_config_test.go
```

### Modified files

```
internal/api/client.go    ListOrgTeams, ListTeamMembers, ListTeamRepos,
                          GetTokenScopes added to interface + TeamInfo type
internal/api/github.go    All four implementations + paginate + nextPageURL
internal/api/gitlab.go    All four implementations + paginateGitLab
internal/api/api_test.go  Tests for all new API methods
internal/collection/
  collection.go           SetPath helper method for import path building
```

---

### Implementation order — strict

```
Step 1  internal/api/client.go    — add TeamInfo type + 4 new methods to interface
Step 2  internal/api/github.go    — paginate + nextPageURL + all 4 implementations
Step 3  internal/api/gitlab.go    — paginateGitLab + all 4 GitLab implementations
Step 4  internal/api/api_test.go  — tests for all new methods (mock httptest servers)
Step 5  internal/collection/collection.go — SetPath helper
Step 6  cmd/import.go             — full import command + buildCollectionFromTeam
Step 7  cmd/import_test.go        — tests listed below
Step 8  cmd/publish.go            — publish command (uses subprocess git calls)
Step 9  cmd/publish_test.go
Step 10 cmd/pull_config.go        — pull-config command
Step 11 cmd/pull_config_test.go
Step 12 cmd/join.go               — join command
Step 13 cmd/join_test.go
Step 14 cmd/sync_config.go        — sync-config command
Step 15 cmd/sync_config_test.go
```

go build ./... and go test ./... after every step.

---

### Tests — what to cover

**internal/api/api_test.go (new tests):**
```
TestGitHubListOrgTeams_SinglePage        — one page of teams
TestGitHubListOrgTeams_MultiPage         — follows Link: rel="next"
TestGitHubListOrgTeams_Unauthorized      — 401 → ErrUnauthorized
TestGitHubListOrgTeams_NotFound          — 404 → clear error
TestGitHubListTeamMembers_AllRoles       — role="" returns all
TestGitHubListTeamMembers_MaintainerOnly — role="maintainer" filtered
TestGitHubListTeamRepos_Paginated        — follows pagination
TestGitHubGetTokenScopes_ParsesHeader    — parses X-OAuth-Scopes correctly
TestGitHubGetTokenScopes_Empty           — empty header returns []string{}
TestNextPageURL_WithNext                 — returns next URL
TestNextPageURL_NoNext                   — returns ""
TestNextPageURL_EmptyHeader              — returns ""
TestGitLabListOrgTeams_SinglePage        — GitLab subgroups
TestGitLabListTeamMembers               — GitLab group members
```

**cmd/import_test.go:**
```
TestImport_DryRun_NoFilesWritten         — dry-run writes nothing
TestImport_CreatesCollectionFiles        — files written to collections dir
TestImport_SetsOwnerFromMaintainer       — maintainer becomes owner
TestImport_FallsBackToCallerOwner        — no maintainer → caller is owner
TestImport_SingleTeam                    — --team flag imports one team
TestImport_MissingScope_ReturnsError     — missing read:org → clear error
TestImport_ConflictMerge                 — merge adds new, keeps existing
TestImport_ConflictOverwrite             — overwrite replaces local
TestImport_ConflictSkip                  — skip leaves local unchanged
TestImport_FlattenNestedTeams            — nested → top-level collection name
TestImport_BuildCollectionFromTeam_Valid — correct Collection struct built
TestImport_BuildCollectionFromTeam_OwnerNotInMembers — owner excluded
TestImport_LoginsMapComplete             — all members in Logins cache
```

**cmd/sync_config_test.go:**
```
TestSyncConfig_NoChanges                 — prints "up to date"
TestSyncConfig_NewMember                 — new member added to collection
TestSyncConfig_RemovedMember             — removed member removed
TestSyncConfig_NewRepo                   — new repo added
TestSyncConfig_DryRun                    — shows diff, no file change
TestSyncConfig_NoNamespace               — error if collection has no namespace
```

---

## PHASE 2 — Docs and README update

Complete Phase 1 fully and run go test ./... clean before starting Phase 2.

---

### Part A — README.md

Read the current README.md completely before writing.
Then rewrite it from scratch to reflect every feature currently
implemented as of the latest session. Do not preserve any section
that is no longer accurate. Do not document features that are
not yet built.

The README must cover these sections in this order:

**1. Badge line**
Go version (from go.mod), license (check LICENSE file), platform support.

**2. One-line description**
What gitcollect does in one sentence. Accurate to current state.

**3. The problem**
Three to four sentences. The flat repo list problem, no native grouping
on GitHub/GitLab, manual clone-one-by-one friction.

**4. Quick demo**
Real terminal output from the actual binary. Run:
```bash
./bin/gitcollect --help
./bin/gitcollect clone --help
./bin/gitcollect show --help
```
Use real output, not invented examples.

**5. How is this different from ghorg?**
Honest comparison. ghorg clones existing org structures.
gitcollect creates custom groupings with access control.
Use the existing framing from prior sessions — do not oversell.

**6. Installation**
Read .goreleaser.yaml for binary names and targets.
Read go.mod for module path and Go version.
Cover: go install, pre-built binaries (Linux/Mac/Windows),
build from source, Homebrew (coming soon).

**7. Quickstart**
Five to seven commands. Real output from the binary.
auth → init → add → member add → clone.

**8. Complete command reference**
Grouped by category. Read the actual --help output for every
command to get accurate flag names and descriptions.
Do not copy from PROMPT.md or prior sessions — read the real
binary output.

Categories:
- Authentication (auth, whoami)
- Collection lifecycle (init, delete, list, show, visibility)
- Repo management (add, remove, repo access, repo show)
- Member management (member add/remove/list)
- Group management (group create/delete/add/remove/list/show)
- Access inspection (inspect, audit)
- Git operations (clone, pull, sync, status)
- Diagnostics (doctor, verify) — if built
- Organisation import (import, publish, pull-config, join, sync-config)
- System (version, completion, concepts)

For each command: one line description + real flags with defaults.

**9. How access control works**
The two-person walkthrough. Keep it concise — the full detail
is in the docs website.

**10. Security model**
Bullet list: token storage, HTTPS-only, input validation,
non-disclosure, atomic writes, immutable IDs.

**11. Architecture**
Brief: folder tree of cmd/ and internal/ as it actually exists.
Not the aspirational structure — the real files that exist right now.
```bash
ls cmd/*.go internal/*/*.go
```
Use that output to build the tree.

**12. Organisation import**
Separate section for the new import commands.
Cover the enterprise onboarding flow:
admin runs import once → publish once →
employees run join once → sync-config keeps everyone in sync.

**13. Roadmap**
Honest list of what is planned but not yet built:
gitcollect fetch (collection sharing by URL),
Homebrew tap,
Winget/Scoop for Windows,
Shared audit log,
Dashboard.

**14. Contributing**
One paragraph. Link to issues. Mention the AI-assisted build
process and PROMPT.md for anyone who wants to contribute.

**15. License**
Read the actual LICENSE file. Do not assume MIT — verify.

---

### Part B — docs/index.html

Read the current docs/index.html completely before writing.
Then update it to reflect the current state. Do not do a full
redesign — that is handled by APPLY_SAMPLE_THEME.md separately.
Only update content that is out of date or missing.

Specifically update:

**1. Command reference sections**
Add the new commands from Phase 1:
- import, publish, pull-config, join, sync-config
  → add to a new "Organisation import" section
  → follow DOCS_STYLE_GUIDE.md exactly for HTML structure

**2. Installation section**
Make sure the install commands match the real binary name
and module path from .goreleaser.yaml and go.mod.
If any URL still says "yourusername", replace with "alby-tomy".

**3. Organisation import walkthrough**
Add a new walkthrough section showing the enterprise onboarding flow:
admin import → publish → employee join.
Use the exact same walkthrough step structure as the existing steps.

**4. Any commands added since the last docs session**
Check the current binary's --help output and compare to what
is documented. Add any missing commands.

```bash
# Get the current full command list
./bin/gitcollect --help

# Compare against what is in docs
grep -o 'gitcollect [a-z-]*' docs/index.html | sort -u
```

Any command in the binary but not in the docs needs to be added.

**5. Accuracy pass**
```bash
# No yourusername placeholders
grep -n "yourusername" docs/index.html

# No TODO or FIXME
grep -n "TODO\|FIXME" docs/index.html

# Key content still present
grep -c "declaration of intent" docs/index.html
```

---

### Commit messages (two separate commits)

After Phase 1 is complete and tests pass:
```
feat: org import — import, publish, pull-config, join, sync-config

- gitcollect import: read GitHub/GitLab org teams → create collections
- gitcollect publish: push collection files to shared config repo
- gitcollect pull-config: fetch collections from shared config repo
- gitcollect join: new-hire onboarding in one command
- gitcollect sync-config: refresh local collections from GitHub state
- Pagination helper for GitHub and GitLab list endpoints
- Token scope pre-flight check with actionable error message
- Conflict resolution: merge, overwrite, or skip existing collections
- Concurrent team data fetching (max 4 parallel, semaphore pattern)
- Full test coverage for all new API methods and commands
```

After Phase 2 is complete:
```
docs: update README and index.html to reflect all current features

- README rewritten to reflect all commands implemented through Session N
- Added organisation import section with enterprise onboarding flow
- Fixed all yourusername placeholder URLs to alby-tomy
- Added new commands from import feature to docs/index.html
- Added organisation import walkthrough to docs
- Accuracy pass: all commands verified against binary --help output
```

---

## Delivery checklist

```
PHASE 1 — BASELINE
  [ ] go build ./... clean
  [ ] go test ./... all green
  [ ] go vet ./... clean
  [ ] Existing features verified (namespace, UserInfo, retryDo)

PHASE 1 — API LAYER
  [ ] client.go: TeamInfo type + 4 new interface methods
  [ ] github.go: paginate + nextPageURL + ListOrgTeams
  [ ] github.go: ListTeamMembers (both role="" and role="maintainer")
  [ ] github.go: ListTeamRepos + GetTokenScopes
  [ ] gitlab.go: paginateGitLab + all 4 GitLab implementations
  [ ] api_test.go: all 14 new API tests pass

PHASE 1 — COLLECTION
  [ ] collection.go: SetPath helper

PHASE 1 — COMMANDS
  [ ] cmd/import.go: full import + checkImportScopes + buildCollectionFromTeam
  [ ] cmd/import_test.go: all 13 tests pass
  [ ] cmd/publish.go: publish command using subprocess git
  [ ] cmd/publish_test.go
  [ ] cmd/pull_config.go: pull-config command
  [ ] cmd/pull_config_test.go
  [ ] cmd/join.go: join command
  [ ] cmd/join_test.go
  [ ] cmd/sync_config.go: sync-config command
  [ ] cmd/sync_config_test.go: all 6 tests pass

PHASE 1 — FINAL
  [ ] go build ./... clean
  [ ] go test ./... all green
  [ ] go vet ./... clean
  [ ] go test -cover ./cmd/... — report percentage
  [ ] PROMPT.md progress tracker updated

PHASE 2 — README
  [ ] Binary name verified from .goreleaser.yaml
  [ ] Module path verified from go.mod
  [ ] Go version verified from go.mod
  [ ] LICENSE file read — correct license in README
  [ ] All --help output read before writing command reference
  [ ] Real folder structure from ls used for architecture section
  [ ] Organisation import section added
  [ ] All links use alby-tomy not yourusername
  [ ] No feature documented that is not yet built

PHASE 2 — DOCS
  [ ] Import commands added to docs/index.html
  [ ] yourusername replaced with alby-tomy everywhere
  [ ] Organisation import walkthrough added
  [ ] Binary --help compared to docs — no missing commands
  [ ] Accuracy pass: no placeholders, content present
  [ ] DOCS_STYLE_GUIDE.md followed for all new HTML

PHASE 2 — FINAL
  [ ] README.md committed separately from docs/index.html
  [ ] Both commits use messages above
  [ ] PROMPT.md progress tracker updated with session log entry
```
