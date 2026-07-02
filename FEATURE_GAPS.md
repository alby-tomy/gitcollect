# gitcollect — Gap Fixes & Missing Features

> Read this file completely before touching any code or HTML.
> Work through groups in strict order A → B → C → D.
> Within each group, work through items in the listed order.
> After every single file changed: go build ./... and go test ./...
> Flag any deviation before implementing it, not after.
> Update PROMPT.md progress tracker before ending the session.

---

## Context — why these gaps matter

A senior engineering review and first-time user analysis identified
11 gaps that affect either user trust (things that feel broken or
surprising) or team adoption (things that block real team use at
scale). They are grouped by what they touch in the codebase so each
group can be implemented cleanly without interfering with the next.

---

## GROUP A — Core UX fixes (existing commands only)

These touch only existing command files. No new files needed.
Implement these first because they affect every user on every command.

---

### A1 — Pre-flight auth check on every authenticated command

**The problem**

If a token has expired or was never set, the user gets a confusing
API error mid-command rather than a clear "you need to authenticate"
message at the start. The error arrives after the command has already
loaded the collection and done local work — wasted time and a bad
first impression.

**What to build**

Add a `requiresAuth(host string) error` helper in `cmd/root.go`:

```go
// requiresAuth checks that a token exists for the given host before
// any network call is made. Called at the top of every RunE function
// that needs authentication. Returns a clear, actionable error if
// no token is found.
func requiresAuth(host string) error {
    _, err := config.LoadToken(host)
    if err != nil {
        return fmt.Errorf(
            "not authenticated with %s\n"+
            "  Run: gitcollect auth --host %s",
            host, host,
        )
    }
    return nil
}
```

Call `requiresAuth(col.Host)` at the very top of `RunE` in every
command that makes API calls, before loading the collection or
resolving the caller identity. The commands that need this:

```
cmd/add.go           cmd/remove.go        cmd/member.go
cmd/group.go         cmd/clone.go         cmd/pull.go
cmd/sync.go          cmd/inspect.go       cmd/visibility.go
cmd/delete.go        cmd/repo.go          cmd/show.go (private only)
cmd/transfer.go      cmd/scale.go         cmd/activity.go
```

`cmd/list.go` does NOT need this — list is explicitly network-free.
`cmd/show.go` for public collections does NOT need this — public
show is auth-free by design.

**Tests to add in cmd/root_test.go:**

```
TestRequiresAuth_NoToken          — returns error with auth command hint
TestRequiresAuth_TokenPresent     — returns nil
TestRequiresAuth_WrongHost        — returns error mentioning the right host
```

---

### A2 — Impact summary before destructive commands

**The problem**

`member remove`, `delete`, and `visibility → public` currently show
a bare confirmation prompt. The user types "y" or a username without
knowing the full scope of what they are about to change. A member
removal that revokes access to 12 repos and removes from 3 groups
should show that before asking.

**What to build**

Add `printRemovalImpact` helper in `cmd/member.go`:

```go
// printRemovalImpact shows the full scope of what removing username
// will do, before the confirmation prompt is shown.
func printRemovalImpact(col *collection.Collection, username, userID string) {
    groups := groupsForMember(col, userID)
    repos  := col.AccessibleRepos(userID)

    fmt.Printf("\n⚠ Removing %s from %s will:\n", username, col.Name)
    if len(groups) > 0 {
        fmt.Printf("  Remove from groups: %s\n", strings.Join(groups, ", "))
    }
    if len(repos) > 0 {
        names := make([]string, len(repos))
        for i, r := range repos { names[i] = r.Name }
        fmt.Printf("  Revoke access to %d repo(s): %s\n",
            len(repos), strings.Join(names, ", "))
    }
    fmt.Printf("  Re-adding %s later requires gitcollect member add\n\n",
        username)
}
```

Call this immediately before the `output.ConfirmWord(...)` prompt
in `runMemberRemove`. Do not call it after — the user must see the
impact before they type the confirmation.

Add `printDeletionImpact` helper in `cmd/delete.go`:

```go
func printDeletionImpact(col *collection.Collection) {
    fmt.Printf("\n⚠ Deleting collection %q will:\n", col.Name)
    fmt.Printf("  Revoke platform access for %d member(s)\n", len(col.Members))
    fmt.Printf("  Remove collaborator access from %d repo(s)\n", len(col.Repos))
    fmt.Printf("  Delete ~/.gitcollect/collections/%s.yaml permanently\n\n",
        col.Name)
}
```

Call before the confirmation prompt in `runDelete`.

Add `printVisibilityImpact` in `cmd/visibility.go` for the
private→public direction only:

```go
func printVisibilityImpact(col *collection.Collection) {
    fmt.Printf("\n⚠ Making %q public will:\n", col.Name)
    fmt.Printf("  Allow any gitcollect user to discover this collection\n")
    fmt.Printf("  Expose the repo list (%d repos) to anyone\n\n",
        len(col.Repos))
}
```

**Tests:**

```
TestPrintRemovalImpact_ShowsGroups    — groups listed in output
TestPrintRemovalImpact_ShowsRepos     — repos listed in output
TestPrintRemovalImpact_NoGroups       — graceful when member has no groups
TestPrintDeletionImpact_ShowsCounts   — member and repo counts correct
```

---

### A3 — --dry-run on delete and member remove

**The problem**

Clone and pull have `--dry-run`. Delete and member remove do not.
These are the two most destructive commands in the tool — a senior
engineer evaluating gitcollect for team use will expect `--dry-run`
on anything that revokes platform access.

**What to build**

Add `--dry-run` flag to `cmd/delete.go`:

```go
var deleteDryRun bool
deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false,
    "preview what would be deleted without executing")
```

In `runDelete`, after printing the deletion impact, check the flag:

```go
if deleteDryRun {
    output.Info("dry-run: no changes made")
    return nil
}
// ... proceed with actual deletion
```

Add `--dry-run` flag to `cmd/member.go` for the remove subcommand:

```go
var memberRemoveDryRun bool
memberRemoveCmd.Flags().BoolVar(&memberRemoveDryRun, "dry-run", false,
    "preview what would be revoked without executing")
```

In `runMemberRemove`, after printing the removal impact:

```go
if memberRemoveDryRun {
    output.Info("dry-run: no changes made")
    return nil
}
```

**Tests:**

```
TestDeleteDryRun_PrintsImpactNoChange     — no API calls, no file changes
TestMemberRemoveDryRun_PrintsImpact       — no API calls, no YAML changes
TestDeleteDryRun_ExitCodeZero             — dry-run exits 0, not an error
```

---

## GROUP B — New diagnostic commands

New files only. These add capability without changing existing commands
(except the sync suggestion in B3 which adds one line to clone output).

---

### B1 — gitcollect help (enriched help system)

**The problem**

Cobra auto-generates `--help` output that lists flags but gives no
examples, no common error guidance, and no mental model. New users
cannot orient themselves from this output alone.

**What to build**

**Part 1: Enrich Long descriptions on every command**

The `Long` field of every `cobra.Command` struct currently has either
an empty string or a one-line description. Expand every command's
`Long` field to include:

- What the command does (one paragraph, plain prose)
- Usage line (already in Use field, but repeat for clarity)
- At least one concrete example using realistic collection/repo names
- For commands with non-obvious behaviour: a "note" paragraph
- For commands that have common failure modes: a brief "if this fails"

Example for clone (update cmd/clone.go Long field):

```go
Long: `Clone every repo you can access in a collection.

Access is verified before cloning using two independent checks:
the local collection manifest (your ID must be listed as a member)
and the platform API (GitHub/GitLab must have you as a collaborator).
Repos you cannot reach are skipped and reported, not treated as errors.

Examples:
  gitcollect clone cybersecurity
  gitcollect clone cybersecurity --pick "pen-test-tools vuln-scanner"
  gitcollect clone cybersecurity --dest ~/projects --dry-run
  gitcollect clone cybersecurity --concurrency 8

Note: --pick takes space-separated repo names in quotes.
      --groups and --users on other commands are comma-separated.
      These two flags intentionally behave differently.

If cloning fails with "pending collaborator invite":
  Accept the invite at https://github.com/notifications
  Then retry: gitcollect clone cybersecurity

See also:
  gitcollect sync   — clone missing repos + pull existing in one pass
  gitcollect pull   — pull updates in already-cloned repos`,
```

Do this for every command. Priority order (most-used first):
clone, member add, member remove, group add, repo access,
show, inspect, init, auth, sync, pull, add, remove, audit,
activity, list, delete, visibility, whoami, version.

**Part 2: `gitcollect help concepts` subcommand**

Create `cmd/help_concepts.go`:

```go
var helpConceptsCmd = &cobra.Command{
    Use:   "concepts",
    Short: "Explain core gitcollect concepts and mental model",
    RunE:  runHelpConcepts,
}

func runHelpConcepts(cmd *cobra.Command, args []string) error {
    fmt.Print(`
GITCOLLECT CONCEPTS

Collection
  A named group of repos stored at ~/.gitcollect/collections/<name>.yaml.
  Collections are local — share them by copying the YAML file to a teammate.
  Each collection has one owner (the person who created it).

Visibility
  private  Only listed members can use the collection. Non-members get
           the same error as "not found" — existence is never confirmed.
  public   Any gitcollect user can clone without authentication.

Access control — two independent layers
  Collection level  who is a member (can use the collection at all)
  Repo level        which groups or individual users can reach each repo
  Both checks must pass. Passing one is not enough.

Identity
  gitcollect stores your platform's permanent numeric user ID, not your
  username. Renaming your GitHub or GitLab account never breaks access.
  Cached login strings are used for display and API paths only.

Platform enforcement
  gitcollect does not maintain its own permission system.
  Every access grant drives a real GitHub/GitLab collaborator API call.
  The local YAML is a declaration of intent — the platform enforces it.
  Editing the YAML by hand grants nothing without the API call.

GitHub collaborator invites
  When you add a member, GitHub sends them a collaborator invite email.
  They must accept it before cloning works — usually within minutes.
  gitcollect detects unaccepted invites and tells you explicitly.
  GitLab access is immediate — no invite step.

Config file locations
  Tokens:      ~/.gitcollect/config           (permissions: 0600)
  Collections: ~/.gitcollect/collections/     (one YAML per collection)
  Audit logs:  ~/.gitcollect/audit/           (newline-delimited JSON)
  Activity:    ~/.gitcollect/activity/        (newline-delimited JSON)
  On Windows:  %USERPROFILE%\.gitcollect\

`)
    return nil
}
```

Register `helpConceptsCmd` as a subcommand of the root command,
not under a `help` parent — so it is accessible as:
`gitcollect concepts` (short form) or via `gitcollect help concepts`.

**Part 3: Improve the root command's help output**

Update rootCmd's Long field to show a quick-start section that
appears when the user runs `gitcollect` with no arguments:

```go
Long: `gitcollect — group GitHub and GitLab repos into collections

QUICK START
  gitcollect auth                         authenticate with GitHub/GitLab
  gitcollect init my-project              create a collection
  gitcollect add my-project repo-name     add a repo to it
  gitcollect member add my-project alice  share with a teammate
  gitcollect clone my-project             clone everything you can access

LEARN MORE
  gitcollect concepts                     how collections and access work
  gitcollect <command> --help             flags and options for any command
  https://alby-tomy.github.io/gitcollect/`,
```

**Tests:**

```
TestHelpConcepts_Runs            — command runs without error
TestHelpConcepts_ContainsKeywords — output contains "Collection", "Identity",
                                   "Platform enforcement", config paths
TestRootCmd_LongContainsQuickStart — root Long field has QUICK START section
```

---

### B2 — gitcollect doctor

**The problem**

Users have no way to verify the tool is configured correctly, check
token validity, or detect drift between their collection files and
the actual state on GitHub/GitLab.

**What to build**

New file: `cmd/doctor.go`

```
gitcollect doctor
```

Behaviour:

```
$ gitcollect doctor

Checking gitcollect configuration...

AUTH
✓ github.com — authenticated as alby-tomy
  Token type: personal access token (never expires)
✗ gitlab.com — no token stored
  Run: gitcollect auth --host gitlab.com

COLLECTIONS
✓ cybersecurity     12 repos · 4 members · github.com
⚠ machine-learning   8 repos · 2 members · github.com
  Last updated 52 days ago — consider sharing the latest YAML
✓ frontend-libs      6 repos · 1 member  · github.com

PLATFORM CHECK
  Checking github.com token scopes...
✓ repo scope present (required for private repos)
⚠ admin:org scope missing (required for org namespace operations)

gitcollect doctor complete · 1 warning, 1 error
Run gitcollect doctor --fix to attempt automatic repairs (where possible)
```

Implementation:

```go
type doctorCheck struct {
    Label   string
    Status  string  // "ok" | "warn" | "error"
    Message string
    Fix     string  // optional: command to fix this
}

func runDoctor(cmd *cobra.Command, args []string) error {
    var checks []doctorCheck

    // 1. Check auth for each configured host
    // 2. List collections and check updated_at staleness (>30 days = warn)
    // 3. For each authenticated host, verify token scopes via API
    //    (GitHub: GET /user — check scopes header)
    // 4. Print summary with counts of ok/warn/error
    // 5. Exit code: 0 if no errors, 1 if any errors, 0 if only warnings
}
```

Flags:
```
--json    machine-readable output (one JSON object per check)
```

Token scope check for GitHub:
GitHub returns token scopes in the `X-OAuth-Scopes` response header
on any API call. Read this header after `GetAuthenticatedUser()` and
check for `repo` (private repo access) and optionally `admin:org`.

**Tests:**

```
TestDoctor_NoAuth_ReportsError        — missing token shows error check
TestDoctor_ValidToken_ReportsOk       — valid token shows ok
TestDoctor_StaleCollection_ReportsWarn — >30 days shows warning
TestDoctor_JSON_ValidOutput           — --json produces parseable JSON
TestDoctor_ExitCode_ZeroOnlyWarns     — exit 0 when only warnings
TestDoctor_ExitCode_OneOnErrors       — exit 1 when any check is error
```

New file: `cmd/doctor_test.go`

---

### B3 — Suggest gitcollect sync after partial clone failure

**The problem**

When clone fails on some repos, the error output tells the user what
failed but not what to do next. Users either re-clone everything
(wasteful) or manually work out which repos to retry.

**What to build**

This is a one-line change in `cmd/clone.go`. After the failure
summary is printed at the end of `runClone`, add:

```go
if len(failed) > 0 {
    output.Suggestion(
        fmt.Sprintf("gitcollect sync %s --dest %s", name, destDir))
}
```

The `output.Suggestion()` function already exists and prints
"Run: <cmd>" in the right style. This is the only change needed.

`gitcollect sync` already does clone-missing + pull-existing, so it
is the correct recovery command — it will clone the repos that failed
without touching the ones that succeeded.

**Tests:**

```
TestClone_PartialFailure_SuggestsSync  — suggestion line appears when
                                         at least one clone fails and
                                         at least one succeeds
TestClone_AllFail_SuggestsSync         — suggestion also appears when all fail
TestClone_AllSucceed_NoSuggestion      — no suggestion when nothing failed
```

---

### B4 — gitcollect verify

**The problem**

Repos in a collection can silently drift from reality. Repos get
renamed, archived, deleted, or made private on GitHub after being
added to a collection. gitcollect has no way to detect this until
a user tries to clone and gets a confusing 404.

**What to build**

New file: `cmd/verify.go`

```
gitcollect verify <collection>
```

Behaviour:

```
$ gitcollect verify cybersecurity

Verifying 6 repos against github.com...

✓ pen-test-tools     — accessible
✓ vuln-scanner       — accessible
✗ ctf-writeups       — not found (renamed or deleted on GitHub)
⚠ nmap-scripts       — archived (cloneable but read-only)
✓ exploit-db         — accessible
✓ burp-extensions    — accessible

1 repo missing · 1 archived · 4 accessible

To remove the missing repo:
  gitcollect remove cybersecurity ctf-writeups
```

Implementation:

```go
type verifyResult struct {
    Repo     string
    Status   string  // "ok" | "archived" | "not_found" | "forbidden"
    Message  string
}

func runVerify(cmd *cobra.Command, args []string) error {
    name := args[0]
    // 1. loadForRead (no owner required — any member can verify)
    // 2. For each repo: client.GetRepo(col.RepoNamespace(), repo.Name)
    //    - nil error, not archived → ok
    //    - nil error, archived → warn
    //    - ErrNotFound → not_found
    //    - ErrForbidden → forbidden (made private, no longer accessible)
    // 3. Run concurrently (same semaphore pattern, max 4)
    // 4. Print results in collection order (not sorted — preserve intent)
    // 5. Print summary line and fix commands for not_found repos
    // 6. Exit code: 0 if all ok or only archived, 1 if any not_found/forbidden
}
```

Flags:
```
--json    machine-readable output
--fix     after verifying, prompt to remove not_found repos one by one
```

The `--fix` flag:
After showing results, for each `not_found` repo prompt:
```
Remove ctf-writeups from cybersecurity? [y/N]:
```
If y: call the existing remove mutation. If n: skip. After all prompts,
print a final summary of what was removed.

**Tests:**

```
TestVerify_AllAccessible_ExitsZero     — all ok → exit 0
TestVerify_OneNotFound_ExitsOne        — not_found → exit 1
TestVerify_ArchivedIsWarn_ExitsZero    — archived → exit 0 (warning only)
TestVerify_Concurrent_AllChecked       — all repos checked, order preserved
TestVerify_JSON_ValidOutput            — --json parseable
TestVerify_Fix_RemovesNotFound         — --fix calls remove mutation on y
```

New file: `cmd/verify_test.go`

---

## GROUP C — Data portability

These add new capability without changing existing command behaviour.

---

### C1 — gitcollect export

**The problem**

Collections are YAML files in `~/.gitcollect/collections/`. If a
machine dies or is replaced, collections are lost. There is no
built-in way to back up or share collection config as structured output.

**What to build**

New file: `cmd/export.go`

```
gitcollect export <collection>           print one collection as YAML
gitcollect export <collection> --json    print as JSON
gitcollect export --all                  print all collections as YAML
gitcollect export --all --json           print all as JSON array
```

Behaviour:

```bash
# Backup one collection
gitcollect export cybersecurity > cybersecurity-backup.yaml

# Backup all collections
gitcollect export --all > all-collections.yaml

# Share with a teammate (they pipe it to their collections dir)
gitcollect export cybersecurity | ssh teammate "cat > ~/.gitcollect/collections/cybersecurity.yaml"

# Machine-readable for scripting
gitcollect export cybersecurity --json | jq '.repos[].name'
```

Implementation:

```go
func runExport(cmd *cobra.Command, args []string) error {
    exportAll, _ := cmd.Flags().GetBool("all")
    exportJSON, _ := cmd.Flags().GetBool("json")

    if exportAll {
        // Load all collections from CollectionsDir()
        // Marshal each to YAML or JSON
        // Print to stdout (no headers, no decoration — pure data)
        // Separate multiple collections with --- (YAML multi-doc)
    } else {
        name := args[0]
        col, err := collection.Load(name)
        // ... marshal and print
    }
}
```

Rules:
- Output goes to stdout only. No file path argument — let the shell
  handle redirection. This is the Unix way and keeps the command simple.
- Output is pure data — no color, no progress indicators, no headers.
  This is one of the rare commands where output.Success() is NOT used.
- JSON output for a single collection: the collection struct as JSON.
- JSON output for --all: a JSON array of collection objects.
- YAML output for --all: multiple YAML documents separated by `---`.
- The exported YAML is valid input for manual placement in
  ~/.gitcollect/collections/ — it round-trips cleanly.

**Tests:**

```
TestExport_SingleCollection_ValidYAML    — output is parseable YAML
TestExport_SingleCollection_JSON         — output is parseable JSON
TestExport_All_MultiDocYAML             — three-dash separator between docs
TestExport_All_JSONArray                 — valid JSON array
TestExport_NoColor_NoDecoration          — stdout is pure data, no ANSI codes
TestExport_NonExistent_Error             — clear error for missing collection
```

New file: `cmd/export_test.go`

---

### C2 — Audit log portability note

**The problem**

The audit log lives at `~/.gitcollect/audit/<collection>.log` on
the owner's machine only. If the owner changes machines, the audit
history is lost. For team accountability ("who added alice to
red-team three months ago?"), this is a real limitation.

**What to build — documentation only (no code change)**

This gap does not have a clean code-only solution in v1. A full
solution (syncing audit logs to a shared repo or API) is a v2 feature.
What can be done now is making the limitation visible and providing
a manual workaround:

1. Add a note to `gitcollect audit --help` (Long field):

```
Note: The audit log is stored locally at ~/.gitcollect/audit/<collection>.log.
To preserve audit history across machines, commit this file to a shared
repository or include it in your gitcollect-collections backup.

To export the audit log:
  gitcollect audit cybersecurity --json > cybersecurity-audit-backup.json
```

2. Add a note to the `gitcollect doctor` output:

```
⚠ Audit logs are stored locally only — not backed up or shared
  Export with: gitcollect audit <collection> --json > backup.json
```

3. Add to docs/index.html in the audit command section (follow
   DOCS_STYLE_GUIDE.md for HTML style):

```html
<p class="note">Audit logs are stored locally at
<code>~/.gitcollect/audit/&lt;collection&gt;.log</code> and are not
automatically shared or backed up. Export them with
<code>gitcollect audit &lt;collection&gt; --json</code> for off-machine
storage. A shared audit log is planned for a future release.</p>
```

No new Go files needed for this item. Update:
- `cmd/audit.go` Long field
- `cmd/doctor.go` output (from C1 above)
- `docs/index.html` audit section

---

## GROUP D — Platform & reliability

---

### D1 — GitLab documentation parity

**The problem**

The tool supports GitHub and GitLab but every example in the docs
and walkthrough uses GitHub. A GitLab user has to guess whether
the auth flow, token format, namespace concept, invite behaviour,
and error messages are the same. They are not — GitLab has key
differences.

**Key GitLab differences to document:**

```
GitHub                          GitLab
──────────────────────────────────────────────────────────
Token format: ghp_xxx...        Token: glpat-xxxx... (PAT)
                                or CI/CD job token
Collaborator invite: email,     Access is immediate —
must be accepted before clone   no invite step at all
works (~minutes delay)

Namespace for orgs: org-name    Namespace: group/subgroup path
(flat)                          (can be nested: acme/backend)

API host default: github.com    Specify: --host gitlab.com
                                or --host gitlab.yourdomain.com

Rate limit: 5000/hour           GitLab: varies by plan
(authenticated)                 (usually 2000/min)
```

**What to build — docs/index.html additions only**

Follow DOCS_STYLE_GUIDE.md for all HTML changes.

Add a "GitLab" subsection to the walkthrough section, after the
existing GitHub walkthrough. Use the same step structure but with
GitLab-specific details:

```
Step 1 — Authenticate with GitLab
  gitcollect auth --host gitlab.com
  (uses glpat-xxxx... token format, not ghp_xxx...)

Step 2 — Create a collection
  gitcollect init my-project --host gitlab.com
  For GitLab groups: gitcollect init my-project --namespace my-group

Step 3 — No invite step needed
  GitLab access is immediate — teammates can clone as soon as you
  run gitcollect member add. No email invite to accept.

Step 4 — Self-hosted GitLab
  gitcollect auth --host gitlab.company.com
  gitcollect init my-project --host gitlab.company.com
```

Also add a "Platform differences" note box in the Philosophy section:

```html
<p class="note">
  <strong>GitLab users:</strong> access grants take effect immediately —
  there is no collaborator invite to accept. Use
  <code>--host gitlab.com</code> or your self-hosted domain on all
  commands. GitLab personal access tokens use the format
  <code>glpat-xxxx...</code>.
</p>
```

No Go code changes needed — the implementation already handles GitLab.
This is documentation only.

---

### D2 — Rate limit handling with retry backoff

**The problem**

`ErrRateLimit` is defined but when it fires, the user sees a raw
error and the command fails. There is no retry, no backoff, and no
indication of when the limit resets or how many requests remain.
For large team operations (SyncCollaborators on 150 members × 10 repos
= 1500 API calls), hitting the rate limit is a real operational risk.

**What to build**

Add retry-with-backoff to the HTTP client in `internal/api/github.go`
and `internal/api/gitlab.go`.

Add a `retryDo` wrapper:

```go
// retryDo wraps the HTTP call with automatic retry on 429 (rate limit).
// On a 429 response, reads the Retry-After header (seconds) and waits
// before retrying. Retries up to maxRetries times total.
// Prints a warning to stderr before each wait so the user knows what
// is happening.
func (c *githubClient) retryDo(req *http.Request) (*http.Response, error) {
    const maxRetries = 3
    for attempt := 0; attempt <= maxRetries; attempt++ {
        resp, err := c.httpClient.Do(req)
        if err != nil {
            return nil, err
        }
        if resp.StatusCode != http.StatusTooManyRequests {
            return resp, nil
        }
        resp.Body.Close()
        if attempt == maxRetries {
            return nil, ErrRateLimit
        }
        // Read Retry-After header — GitHub sets this on 429
        wait := 60 * time.Second  // default if header absent
        if ra := resp.Header.Get("Retry-After"); ra != "" {
            if secs, err := strconv.Atoi(ra); err == nil {
                wait = time.Duration(secs) * time.Second
            }
        }
        // Remaining rate limit context
        remaining := resp.Header.Get("X-RateLimit-Remaining")
        reset := resp.Header.Get("X-RateLimit-Reset")
        fmt.Fprintf(os.Stderr,
            "⚠ Rate limit hit (remaining: %s, resets: %s). "+
            "Waiting %s before retry %d/%d...\n",
            remaining, formatResetTime(reset), wait, attempt+1, maxRetries)
        time.Sleep(wait)
        // Re-create the request body for POST requests if needed
    }
    return nil, ErrRateLimit
}
```

Replace all `c.httpClient.Do(req)` calls in `github.go` and
`gitlab.go` with `c.retryDo(req)`.

Add `formatResetTime(unix string) string` helper that converts the
`X-RateLimit-Reset` Unix timestamp to a human-readable local time:
"resets at 14:32" rather than a raw Unix number.

**Tests:**

```
TestRetryDo_FirstAttemptSucceeds     — no retry on 200
TestRetryDo_RetriesOn429             — retries up to maxRetries
TestRetryDo_ReadsRetryAfterHeader    — wait time from header
TestRetryDo_ExceedsMaxRetries        — returns ErrRateLimit
TestRetryDo_PrintsWarningToStderr    — user sees what is happening
```

Note: In tests, mock `time.Sleep` to avoid real waits. Use a
`sleepFunc` variable injected at test time (same pattern as how
git tests mock exec).

---

## Implementation order — strict

Work through this exact sequence. Do not start a new item until the
previous one compiles and all tests pass.

```
A1 — requiresAuth helper + all call sites + tests
A2 — printRemovalImpact + printDeletionImpact + printVisibilityImpact + tests
A3 — --dry-run on delete and member remove + tests
B1 — Enrich Long fields on all commands + helpConceptsCmd + root Long
B2 — cmd/doctor.go + cmd/doctor_test.go
B3 — sync suggestion in clone (one line + three tests)
B4 — cmd/verify.go + cmd/verify_test.go
C1 — cmd/export.go + cmd/export_test.go
C2 — Documentation only: audit Long field + doctor output + docs/index.html
D1 — Documentation only: docs/index.html GitLab section + note box
D2 — retryDo in github.go + gitlab.go + tests
```

After ALL items: run go test ./... -cover and report per-package
coverage. cmd package should be above 35% by this point.

---

## New files summary

```
cmd/help_concepts.go        gitcollect concepts command
cmd/help_concepts_test.go
cmd/doctor.go               gitcollect doctor command
cmd/doctor_test.go
cmd/verify.go               gitcollect verify command
cmd/verify_test.go
cmd/export.go               gitcollect export command
cmd/export_test.go
```

## Modified files summary

```
cmd/root.go                 requiresAuth helper + root Long field
cmd/root_test.go            3 new tests for requiresAuth
cmd/clone.go                sync suggestion after partial failure
cmd/clone_test.go           3 new tests for sync suggestion
cmd/delete.go               printDeletionImpact + --dry-run flag
cmd/member.go               printRemovalImpact + --dry-run flag
cmd/visibility.go           printVisibilityImpact
cmd/audit.go                Long field update (C2)
cmd/add.go                  Long field update (B1)
cmd/show.go                 Long field update (B1)
cmd/group.go                Long field update (B1)
cmd/inspect.go              Long field update (B1)
cmd/init.go                 Long field update (B1)
cmd/sync.go                 Long field update (B1)
cmd/pull.go                 Long field update (B1)
cmd/remove.go               Long field update (B1)
cmd/repo.go                 Long field update (B1)
cmd/activity.go             Long field update (B1)
cmd/list.go                 Long field update (B1)
cmd/whoami.go               Long field update (B1)
cmd/version.go              Long field update (B1)
internal/api/github.go      retryDo wrapper (D2)
internal/api/gitlab.go      retryDo wrapper (D2)
internal/api/api_test.go    retryDo tests (D2)
docs/index.html             C2 audit note + D1 GitLab section
```

---

## Accuracy rules for docs/index.html changes

Follow DOCS_STYLE_GUIDE.md for all HTML.
GitLab token format is `glpat-xxxx...` — verify this is correct
before writing it in any user-facing output or docs.
--groups is comma-separated. --pick is space-separated (quoted).
These must never be shown incorrectly in any Long description,
example, or HTML.

---

## Commit message when complete

```
feat: gap fixes — auth checks, impact previews, help system,
      doctor, verify, export, rate limit retry, GitLab docs

- requiresAuth pre-flight check on all authenticated commands
- member remove and delete show full impact before confirmation
- --dry-run on delete and member remove
- gitcollect concepts — mental model reference command
- enriched Long descriptions with examples on all commands
- gitcollect doctor — token validity, collection health check
- gitcollect sync suggested automatically after partial clone failure
- gitcollect verify — check all repos still accessible on platform
- gitcollect export — backup and share collections as YAML or JSON
- audit log portability notes (docs + Long field)
- GitLab walkthrough and platform differences in docs
- Rate limit 429 retry with backoff in GitHub and GitLab clients
```

---

## Delivery checklist

```
GROUP A
  [ ] A1: requiresAuth in root.go + all call sites + 3 tests
  [ ] A2: impact helpers in delete.go, member.go, visibility.go + tests
  [ ] A3: --dry-run on delete + member remove + tests

GROUP B
  [ ] B1: Long fields enriched on all 20 commands
  [ ] B1: helpConceptsCmd created and registered
  [ ] B1: root Long field with QUICK START section
  [ ] B2: cmd/doctor.go + cmd/doctor_test.go (6 tests)
  [ ] B3: sync suggestion in clone.go + 3 tests
  [ ] B4: cmd/verify.go + cmd/verify_test.go (6 tests)

GROUP C
  [ ] C1: cmd/export.go + cmd/export_test.go (6 tests)
  [ ] C2: audit Long field updated
  [ ] C2: doctor output includes audit warning
  [ ] C2: docs/index.html audit note added

GROUP D
  [ ] D1: docs/index.html GitLab walkthrough subsection
  [ ] D1: docs/index.html GitLab platform differences note box
  [ ] D2: retryDo in github.go + gitlab.go
  [ ] D2: api_test.go retryDo tests (5 tests)

FINAL
  [ ] go build ./... clean
  [ ] go test ./... all green
  [ ] go vet ./... clean
  [ ] go test -cover ./cmd/... — above 35%
  [ ] docs/index.html accuracy pass (no placeholders, no wrong separators)
  [ ] PROMPT.md progress tracker updated
```
