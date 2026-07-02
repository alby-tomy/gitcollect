# gitcollect — Feature Addition: Auto-create repo on add

> Append this to PRE_SHIP_IMPROVEMENTS.md or paste it as a standalone
> session after the pre-ship improvements are complete. All existing
> session discipline rules apply: read completely before touching code,
> go build + go test after every file, flag deviations before acting.

---

## Feature: guided repo creation when adding a non-existent repo

### What this feature does

Currently, when a user runs `gitcollect add cybersecurity new-tool` and
`new-tool` does not exist on GitHub/GitLab, the command fails with a 404
error and the user must go create the repo manually on the platform, then
come back and run `add` again. This breaks the workflow at exactly the
moment the user is most actively building their collection.

The new behaviour: when `add` detects that a repo does not exist on the
platform, it pauses and asks the owner whether to create it. If yes, it
creates the repo, then continues the normal `add` flow (adds it to the
collection, syncs collaborator access). If no, it skips that repo and
continues with any others in the batch.

### Why this belongs in gitcollect

gitcollect manages the relationship between repos and collections. Creating
a repo is a natural extension of "I want this repo to be part of my
collection" — the tool already knows the namespace, the platform, and the
authenticated user's credentials. Forcing the user to leave the terminal,
open a browser, create the repo, come back, and re-run the command is
unnecessary friction that this feature eliminates cleanly.

### Design decisions — understand these before implementing

**Decision 1: confirmation happens at `add`, not at `clone` or `sync`**

`add` is the deliberate collection-building command. It is the right place
to ask "this doesn't exist yet, should I create it?" `clone` and `sync`
are operational commands that execute against whatever the collection already
contains — they must never silently create repos. A user running
`gitcollect clone cybersecurity` must be able to trust that no side effects
happen beyond cloning.

**Decision 2: skip confirmation in non-interactive contexts**

If stdout is not a TTY (output is being piped or redirected), skip the
confirmation prompt entirely and treat the missing repo as a hard error,
same as today. Non-interactive contexts (CI/CD scripts, automated pipelines)
must not block waiting for input. Use `output.IsTerminal()` or
`term.IsTerminal(int(os.Stdout.Fd()))` to detect this.

**Decision 3: creation uses the collection's namespace**

The repo is created under `col.RepoNamespace()` — the same namespace
helper added in the pre-ship improvements Priority 3. If the collection
has `namespace: acme-corp`, the new repo is created under `acme-corp`.
If no namespace, it is created under the authenticated user's login.
This is consistent with how every other API call in the codebase works.

**Decision 4: visibility defaults to private, overridable per invocation**

New repos are created as private by default. The user can override this
with a new flag `--new-repo-visibility public|private` on the `add` command.
This flag only has effect when a repo does not exist and creation is
confirmed — it is silently ignored for repos that already exist.

**Decision 5: batch behaviour**

When `add` is called with multiple repo names (batch mode), each missing
repo is confirmed individually before any creation happens. The confirmation
prompt shows the repo name and namespace clearly. If the user declines
creation for one repo in a batch, that repo is skipped; the others continue
normally. Batch failures are collected and reported together at the end,
consistent with the existing batch pattern.

**Decision 6: creation is audited**

Every repo creation must be logged as an audit entry with action
`"repo.create"` before the normal `"repo.add"` entry that already follows.
This gives a clear record of what was created through gitcollect versus
what already existed.

---

## API client changes — internal/api/client.go

Add `CreateRepo` to the `Client` interface:

```go
type Client interface {
    // ... existing methods ...

    // CreateRepo creates a new repository under the given owner/org.
    // name must be a valid repo name (validated before calling).
    // private controls visibility. description may be empty.
    // Returns the created repo's info (including its clone URL) on success.
    // Returns ErrNameConflict if the repo already exists (race condition guard).
    // Returns ErrForbidden if the authenticated user cannot create repos
    // under the given owner (e.g. not a member of the org).
    CreateRepo(owner, name string, private bool, description string) (RepoInfo, error)
}

// Add to sentinel errors
var (
    // existing errors...
    ErrNameConflict = errors.New("repository already exists")
    ErrForbidden    = errors.New("insufficient permissions") // may already exist
)
```

### GitHub implementation — internal/api/github.go

```go
func (c *githubClient) CreateRepo(owner, name string, private bool, description string) (RepoInfo, error) {
    // Determine endpoint: user repo vs org repo
    // If owner == authenticated user's login → POST /user/repos
    // If owner != authenticated user's login → POST /orgs/{owner}/repos
    // Use c.GetAuthenticatedUser() (already cached) to compare
    endpoint := "/user/repos"
    if owner != c.cachedLogin {
        endpoint = fmt.Sprintf("/orgs/%s/repos", owner)
    }

    body := map[string]any{
        "name":        name,
        "private":     private,
        "description": description,
        "auto_init":   false,  // never auto-init — user controls their repo
    }

    resp, err := c.doJSON(http.MethodPost, endpoint, body)
    if err != nil {
        return RepoInfo{}, err
    }
    defer resp.Body.Close()

    switch resp.StatusCode {
    case http.StatusCreated:
        // success
    case http.StatusUnprocessableEntity:
        return RepoInfo{}, ErrNameConflict
    case http.StatusForbidden, http.StatusNotFound:
        return RepoInfo{}, ErrForbidden
    default:
        return RepoInfo{}, classifyStatus(resp.StatusCode)
    }

    var out struct {
        Name     string `json:"name"`
        CloneURL string `json:"clone_url"`
        Private  bool   `json:"private"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return RepoInfo{}, fmt.Errorf("parse create repo response: %w", err)
    }
    return RepoInfo{
        Name:     out.Name,
        CloneURL: out.CloneURL,
        Private:  out.Private,
    }, nil
}
```

### GitLab implementation — internal/api/gitlab.go

```go
func (c *gitlabClient) CreateRepo(owner, name string, private bool, description string) (RepoInfo, error) {
    // GitLab uses POST /projects
    // namespace_path determines personal vs group
    visibility := "private"
    if !private {
        visibility = "public"
    }

    body := map[string]any{
        "name":           name,
        "path":           name,
        "namespace_path": owner,
        "visibility":     visibility,
        "description":    description,
        "initialize_with_readme": false,
    }

    resp, err := c.doJSON(http.MethodPost, "/projects", body)
    if err != nil {
        return RepoInfo{}, err
    }
    defer resp.Body.Close()

    switch resp.StatusCode {
    case http.StatusCreated:
        // success
    case http.StatusConflict:
        return RepoInfo{}, ErrNameConflict
    case http.StatusForbidden, http.StatusNotFound:
        return RepoInfo{}, ErrForbidden
    default:
        return RepoInfo{}, classifyStatus(resp.StatusCode)
    }

    var out struct {
        Name              string `json:"name"`
        HTTPURLToRepo     string `json:"http_url_to_repo"`
        Visibility        string `json:"visibility"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return RepoInfo{}, fmt.Errorf("parse create repo response: %w", err)
    }
    return RepoInfo{
        Name:     out.Name,
        CloneURL: out.HTTPURLToRepo,
        Private:  out.Visibility == "private",
    }, nil
}
```

---

## Collection mutation — internal/collection/mutation.go

No changes needed to the existing `AddMember` or `SyncCollaborators` flow.
Repo creation happens at the cmd layer before the existing `AddRepo` mutation
is called. The mutation layer receives a repo that already exists on the
platform — its job remains unchanged.

---

## Command changes — cmd/add.go

### New flag

```go
var newRepoVisibility string

// In the command's init or RunE setup:
addCmd.Flags().StringVar(
    &newRepoVisibility,
    "new-repo-visibility",
    "private",
    `visibility for auto-created repos: "public" or "private" (default "private")`,
)
```

Validate the flag value early in `RunE`:

```go
if newRepoVisibility != "public" && newRepoVisibility != "private" {
    return fmt.Errorf(
        "add: invalid --new-repo-visibility %q: must be \"public\" or \"private\"",
        newRepoVisibility,
    )
}
```

### New helper: ensureRepoExists

Extract a helper that the main `add` loop calls per repo before attempting
to add it to the collection. This keeps the creation logic isolated and
testable:

```go
// ensureRepoExists checks whether repoName exists on the platform under
// col.RepoNamespace(). If it does, returns nil immediately.
// If it does not exist and the context is interactive, asks the owner
// whether to create it. If confirmed, creates it and logs the action.
// If declined, returns ErrSkipped so the caller can skip this repo.
// If not interactive (not a TTY), returns ErrNotFound immediately.
func ensureRepoExists(
    col *collection.Collection,
    repoName string,
    client api.Client,
    caller api.UserInfo,
    private bool,
) error {
    namespace := col.RepoNamespace()

    _, err := client.GetRepo(namespace, repoName)
    if err == nil {
        return nil // repo exists, nothing to do
    }
    if !errors.Is(err, api.ErrNotFound) {
        return fmt.Errorf("add: checking repo %q: %w", repoName, err)
    }

    // Repo does not exist.
    // If not interactive, treat as hard error.
    if !term.IsTerminal(int(os.Stdout.Fd())) {
        return fmt.Errorf(
            "add: repo %q not found under %s (running non-interactively — create it manually first)",
            repoName, namespace,
        )
    }

    // Interactive: ask the owner.
    output.Warn("repo %q does not exist under %s", repoName, namespace)
    confirmed := output.Confirm(fmt.Sprintf(
        "Create %s/%s as a %s repository?",
        namespace, repoName, private_word(private),
    ))
    if !confirmed {
        return ErrSkipped
    }

    // Create it.
    output.Info("Creating %s/%s...", namespace, repoName)
    _, err = client.CreateRepo(namespace, repoName, private, "")
    if errors.Is(err, api.ErrNameConflict) {
        // Race condition: someone created it between our check and create.
        // Treat as success — the repo now exists.
        output.Info("repo %q was just created by someone else — continuing", repoName)
        return nil
    }
    if err != nil {
        return fmt.Errorf("add: create repo %q: %w", repoName, err)
    }

    // Audit the creation.
    _ = audit.Append(audit.AuditEntry{
        Timestamp:  time.Now().UTC(),
        Collection: col.Name,
        Actor:      caller.Username,
        Action:     "repo.create",
        Target:     repoName,
        Detail:     fmt.Sprintf("Created %s/%s (%s)", namespace, repoName, private_word(private)),
        Result:     "ok",
    })

    output.Success("Created %s/%s", namespace, repoName)
    return nil
}

// private_word returns "private" or "public" — used in display strings only.
func private_word(private bool) string {
    if private {
        return "private"
    }
    return "public"
}
```

Add `ErrSkipped` as a package-level sentinel in `cmd/add.go`:

```go
var ErrSkipped = errors.New("skipped by user")
```

### Updated add loop

In the main `RunE` function where repos are processed one by one (or in
batch), call `ensureRepoExists` before the existing collection mutation:

```go
for _, repoName := range repoNames {
    // Ensure the repo exists on the platform before adding to collection.
    if err := ensureRepoExists(col, repoName, client, caller, newRepoVisibility == "private"); err != nil {
        if errors.Is(err, ErrSkipped) {
            results.skipped = append(results.skipped, repoName)
            continue
        }
        results.failed[repoName] = err
        continue
    }

    // Existing add logic continues unchanged from here.
    if err := col.AddRepo(repoName, client, caller.ID); err != nil {
        results.failed[repoName] = err
        continue
    }
    results.added = append(results.added, repoName)
}
```

### Updated output after a batch

When printing results, distinguish between repos that were added to existing
repos, repos that were created-then-added, and repos that were skipped by
the user:

```
$ gitcollect add cybersecurity pen-test-tools new-scanner not-creating-this

✓ Added pen-test-tools (already existed)
⚠ new-scanner not found under alby-tomy
  Create alby-tomy/new-scanner as a private repository? [y/N]: y
  ✓ Created alby-tomy/new-scanner
  ✓ Added new-scanner
  Skipped not-creating-this (declined creation)

2 repos added · 1 skipped
```

---

## Tests — internal/api/api_test.go

Add these tests for `CreateRepo` on both GitHub and GitLab clients:

```
TestGitHubCreateRepo_PersonalAccount      — POST /user/repos, 201 response
TestGitHubCreateRepo_OrgAccount           — POST /orgs/{org}/repos, 201 response
TestGitHubCreateRepo_AlreadyExists        — 422 response → ErrNameConflict
TestGitHubCreateRepo_NoPermission         — 403 response → ErrForbidden
TestGitLabCreateRepo_Success              — POST /projects, 201 response
TestGitLabCreateRepo_AlreadyExists        — 409 response → ErrNameConflict
```

## Tests — cmd/add_test.go

Add these tests for the new flow:

```
TestEnsureRepoExists_ExistingRepo         — GetRepo returns success, no prompt
TestEnsureRepoExists_MissingNonInteractive — not a TTY, returns ErrNotFound
TestEnsureRepoExists_MissingDeclined      — interactive, user says N, ErrSkipped
TestEnsureRepoExists_MissingConfirmed     — interactive, user says Y, CreateRepo called
TestEnsureRepoExists_RaceCondition        — CreateRepo returns ErrNameConflict, treated as success
TestAdd_BatchWithMixedExistence           — some repos exist, one missing+confirmed, one declined
TestAdd_NewRepoVisibilityFlag_Invalid     — flag validation rejects unknown values
TestAdd_NewRepoVisibilityDefault          — --new-repo-visibility defaults to "private"
TestAdd_NonInteractive_MissingRepo        — piped context, missing repo is hard error
TestAdd_AuditEntry_OnCreate              — creation generates repo.create audit entry
```

---

## mock client updates

Add `CreateRepo` to the existing `mockClient` in test files:

```go
type mockClient struct {
    // existing fields...
    createRepoFunc func(owner, name string, private bool, description string) (api.RepoInfo, error)
}

func (m *mockClient) CreateRepo(owner, name string, private bool, description string) (api.RepoInfo, error) {
    if m.createRepoFunc != nil {
        return m.createRepoFunc(owner, name, private, description)
    }
    // Default: simulate successful creation
    return api.RepoInfo{
        Name:     name,
        CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, name),
        Private:  private,
    }, nil
}
```

---

## Scope boundary — what this feature does NOT do

These are explicitly out of scope. Do not implement them:

- **Repo initialisation options** — no `--init-readme`, no `--gitignore-template`,
  no `--license`. Create a bare empty repo only. GitHub's UI handles setup.
- **Repo settings after creation** — no branch protection, no topics, no
  description prompt. Keep creation to the minimum.
- **Creation during `clone` or `sync`** — these commands never create repos.
  They operate on existing collection state only.
- **Deleting repos** — gitcollect manages collections, not repo lifecycle
  beyond creation. No `gitcollect remove` triggering repo deletion.
- **Forking** — if the user wants to fork an existing repo into a collection,
  that is a separate workflow not addressed here.

---

## Delivery checklist

Mark each done only when `go test ./...` passes with it included:

```
API layer
  [ ] internal/api/client.go      — CreateRepo in interface + ErrNameConflict sentinel
  [ ] internal/api/github.go      — CreateRepo implementation (personal + org)
  [ ] internal/api/gitlab.go      — CreateRepo implementation
  [ ] internal/api/api_test.go    — 6 new tests listed above

cmd layer
  [ ] cmd/add.go                  — ensureRepoExists helper + --new-repo-visibility flag
                                     + updated add loop + updated batch output
  [ ] cmd/add_test.go             — 10 new tests listed above
  [ ] mock client                 — CreateRepo added to mockClient in test files

Audit
  [ ] audit.go constants          — "repo.create" action used consistently
                                     (no new code needed if action is a plain string)

Documentation
  [ ] gitcollect add --help       — --new-repo-visibility flag visible in help
  [ ] README.md                   — add one paragraph to the repo management section
                                     explaining the auto-create behaviour

FINAL CHECK
  [ ] go build ./...              — clean
  [ ] go test ./...               — all packages green
  [ ] go vet ./...                — clean
  [ ] Manual test (dry run):
      gitcollect add <collection> <nonexistent-repo> --dry-run
      Should print the confirmation prompt with no side effects
  [ ] PROMPT.md tracker           — session log entry added
```
