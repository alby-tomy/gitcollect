# gitcollect — Documentation Audit & Install Section

> Read this file completely before touching any code or HTML.
> Every change must be verified against the actual codebase before
> writing it into the docs — accuracy over speed, always.
> Run go build ./... and go test ./... once at the start to confirm
> clean baseline. After all edits: open docs/index.html in a browser
> locally and visually verify every new section renders correctly.

---

## Step 0 — Baseline verification before touching anything

Run these and report the output before any edits:

```bash
go build ./...
go test ./...
```

Then verify these specific things in the actual codebase:

```bash
# 1. Confirm --namespace flag exists in init command
grep -n "namespace" cmd/init.go

# 2. Confirm RepoNamespace() method exists
grep -n "RepoNamespace" internal/collection/collection.go

# 3. Confirm repo grant/revoke are gone
grep -rn "runRepoGrant\|runRepoRevoke\|repo grant\|repo revoke" --include="*.go" .

# 4. Confirm activity is marked experimental in cmd
grep -n "EXPERIMENTAL\|experimental" cmd/activity.go

# 5. Confirm --groups and --users are comma-separated (not space)
grep -n "groups\|users" cmd/repo.go | grep "Flag\|flag"

# 6. Check what the binary is actually named in .goreleaser.yaml
grep -n "binary\|project_name" .goreleaser.yaml

# 7. Check the module path
head -3 go.mod

# 8. Check what go version is required
grep "^go " go.mod
```

Report every finding before writing a single character of HTML.
Do not assume — verify against the actual files.

---

## Part 1 — Audit and fix existing content

Fix these confirmed gaps in docs/index.html in order:

### Fix 1 — Add --namespace flag to init command table

The `gitcollect init` command table currently shows three flags:
`--host`, `--description`, `--public`. The `--namespace` flag added
in Priority 3 of the pre-ship session is missing. Verify it exists
in cmd/init.go first (Step 0 above), then add it to the table:

```
--namespace   GitHub/GitLab username or org whose repos this
              collection contains. Defaults to the authenticated
              user's login. Set this when the repos live under an
              org (e.g. --namespace acme-corp) rather than your
              personal account.
```

Also add a one-line note below the init description:

```
When repos live under an organisation rather than your personal
account, pass --namespace <org-name> so gitcollect builds API paths
correctly. The collection owner (who administers it) is always the
authenticated user regardless of namespace.
```

### Fix 2 — Add Namespace row to the Philosophy concepts table

The Philosophy table currently has: Collection, Member, Group,
Repo access, Audit log. Add a Namespace row after Collection:

```
Namespace    The GitHub/GitLab username or org under which the
             collection's repos actually live — used for all API
             path-building (GET /repos/{namespace}/{repo}).
             Defaults to the collection owner's login. Set
             --namespace acme-corp on init for org repos.
             Distinct from the collection owner, who is always
             the authenticated user.
```

### Fix 3 — Add identity model paragraph to Philosophy section

After the existing core principle paragraph ("gitcollect's local YAML
manifest is a declaration of intent..."), add a second paragraph:

```
Identity is based on immutable platform user IDs, not usernames.
Renaming a GitHub or GitLab account never breaks collection
ownership or membership — gitcollect resolves and caches the
platform's permanent numeric ID alongside the login string, and all
access decisions compare by ID. The cached login is used only for
display, API path-building, and audit log entries — never for
security decisions. Collections automatically update a stale cached
login the next time a write command is run.
```

### Fix 4 — Add [experimental] label to activity command

Verify cmd/activity.go has `[EXPERIMENTAL]` in its Long description
(Step 0 confirms this). Then in docs/index.html, add a visible
experimental badge to the `gitcollect activity` command heading and
add this note directly below it:

```
[experimental] — output format and flag names may change in a future
release. Functionality is stable but the interface is not yet frozen.
```

Style the badge to match how the page currently renders — use the
same visual treatment as any other status indicators on the page,
or a simple italic note if the page has no existing badge style.

### Fix 5 — Add namespace to walkthrough step 1

The walkthrough step 1 comment says:
"the owner is always whoever ran gitcollect auth — there's no --owner flag"

Update this comment to mention namespace:

```
# the owner is always whoever ran "gitcollect auth"
# for repos under an org, pass --namespace to set the repo namespace:
#   gitcollect init cybersecurity --namespace acme-corp
gitcollect init cybersecurity
```

### Fix 6 — Add collection sharing "coming soon" note in walkthrough step 3

After the two Options (A and B) in step 3, add:

```
A gitcollect fetch command for sharing collections directly by URL
is planned for a future release — this will eliminate the manual
file-copy step.
```

### Fix 7 — Add Windows path note

In the Philosophy table, Collection row, add a note:

```
On Windows, ~/.gitcollect/ maps to %USERPROFILE%\.gitcollect\
```

Or add it as a small note below the Philosophy table — wherever it
fits cleanly in the existing HTML structure.

### Fix 8 — Fix footer links

The footer links point to:
- https://alby-tomy.github.io/README.md
- https://alby-tomy.github.io/PROMPT.md

These almost certainly 404. Verify by checking whether README.md and
PROMPT.md are in the docs/ folder or served from the repo root in
GitHub Pages config. Fix them to the correct paths — likely:
- https://github.com/alby-tomy/gitcollect/blob/main/README.md
- https://github.com/alby-tomy/gitcollect/blob/main/PROMPT.md

Or simply link to the GitHub repo pages directly if PROMPT.md is not
intended to be public-facing.

---

## Part 2 — New section: Installation

This is the most important addition. Add a complete "Installation"
section between the current "Install & first run" section and the
"Walkthrough" section. Replace the thin existing install content with
a full per-OS breakdown.

The section title should be "Installation" and must have a nav anchor
`id="install"` so the existing nav link still works.

### What to verify before writing this section

Read these files first:
- `.goreleaser.yaml` — get the exact binary name, goos/goarch targets
- `go.mod` — get the exact module path and minimum Go version
- `cmd/version.go` — get the exact output of `gitcollect version`
- `cmd/completion.go` — confirm supported shells and exact subcommand

Use real values from these files everywhere. Never invent a path,
module name, or command that isn't verified.

---

### Section structure

Write the section in this exact order:

#### Method 1 — go install (recommended for Go developers)

```
Requires Go [actual version from go.mod]+

go install [actual module path from go.mod]@latest
```

Followed by a note:
```
This compiles gitcollect on your machine and places the binary in
~/go/bin/ (Linux/Mac) or %USERPROFILE%\go\bin\ (Windows). If that
directory is already on your PATH — which it is by default after a
standard Go installation — gitcollect is immediately available.

To upgrade to a newer version, run the same command again with @latest.
```

#### Method 2 — Pre-built binary (no Go required)

Intro sentence:
```
Download a pre-built binary from the GitHub Releases page:
https://github.com/alby-tomy/gitcollect/releases
```

Then three subsections — Linux, macOS, Windows — each as a distinct
block with a heading. Read .goreleaser.yaml to get the exact archive
names and targets. Use the real binary name from the config.

**Linux:**

```
# Intel/AMD (amd64)
curl -L https://github.com/alby-tomy/gitcollect/releases/latest/download/[real filename from goreleaser].tar.gz | tar xz
sudo mv [binary name] /usr/local/bin/

# ARM64 (e.g. Raspberry Pi, AWS Graviton)
curl -L https://github.com/alby-tomy/gitcollect/releases/latest/download/[real filename].tar.gz | tar xz
sudo mv [binary name] /usr/local/bin/

# Verify
gitcollect version
```

**macOS:**

```
# Apple Silicon (M1/M2/M3 — arm64)
curl -L https://github.com/alby-tomy/gitcollect/releases/latest/download/[real filename].tar.gz | tar xz
sudo mv [binary name] /usr/local/bin/

# Intel Mac (amd64)
curl -L https://github.com/alby-tomy/gitcollect/releases/latest/download/[real filename].tar.gz | tar xz
sudo mv [binary name] /usr/local/bin/

# Verify
gitcollect version
```

Include a note after macOS:
```
If macOS blocks the binary with "cannot be opened because the developer
cannot be verified", run:
  xattr -d com.apple.quarantine /usr/local/bin/[binary name]
```

**Windows:**

Windows gets a more detailed treatment because PATH setup is not
automatic for most users:

```
1. Download [binary name]_windows_amd64.zip from:
   https://github.com/alby-tomy/gitcollect/releases/latest

2. Extract the zip — you get [binary name].exe

3. Move [binary name].exe to a permanent location, e.g.:
   C:\Users\YourName\bin\[binary name].exe

4. Add that folder to your PATH:
   a. Open Start → search "Edit the system environment variables"
   b. Click "Environment Variables..."
   c. Under "User variables", select "Path" → Edit → New
   d. Add: C:\Users\YourName\bin
   e. Click OK on all dialogs

5. Open a new terminal (the old one won't see the updated PATH) and run:
   [binary name] version
```

Add a note:
```
Note: Windows support is provided as a best-effort build. If you
encounter any Windows-specific issues, please open an issue:
https://github.com/alby-tomy/gitcollect/issues
```

#### Method 3 — Build from source

```
# Requires Go [actual version]+ and git
git clone https://github.com/alby-tomy/gitcollect.git
cd gitcollect
go build -ldflags="-s -w -X main.version=dev" -o bin/[binary name] .
# Move to somewhere on your PATH:
sudo mv bin/[binary name] /usr/local/bin/   # Linux/Mac
# or add bin/ to your PATH on Windows
```

#### Method 4 — Homebrew (coming soon)

```
brew install alby-tomy/tap/[binary name]
```

Mark clearly as not yet available:
```
Note: The Homebrew tap is not yet published. This will be available
in a future release. Use one of the methods above in the meantime.
```

#### Verify the installation

Regardless of method:
```
[binary name] version
# Expected output (actual format from cmd/version.go):
[real output from cmd/version.go — read the file to get this]
```

#### Shell completion (subsection within install)

Read cmd/completion.go to get the exact subcommand and supported shells,
then write one block per shell:

**Bash (Linux/Mac):**
```
[binary name] completion bash >> ~/.bashrc
source ~/.bashrc
```

**Zsh (Mac default shell):**
```
[binary name] completion zsh >> ~/.zshrc
source ~/.zshrc
```

Or for Oh My Zsh:
```
[binary name] completion zsh > ~/.oh-my-zsh/completions/_[binary name]
```

**Fish:**
```
[binary name] completion fish > ~/.config/fish/completions/[binary name].fish
```

**PowerShell (Windows):**
```
[binary name] completion powershell >> $PROFILE
. $PROFILE
```

---

## Part 3 — Final accuracy pass

After all edits are made, do this before finishing:

```bash
# 1. Search for any remaining grant/revoke references in the HTML
grep -n "repo grant\|repo revoke\|runRepoGrant\|runRepoRevoke" docs/index.html

# 2. Search for placeholder text that wasn't replaced
grep -n "\[real\|\[actual\|\[binary name\|\[exact\|TODO\|FIXME" docs/index.html

# 3. Confirm --namespace appears in the init section
grep -n "namespace" docs/index.html

# 4. Confirm identity model paragraph exists
grep -n "immutable\|platform user ID\|numeric ID" docs/index.html

# 5. Confirm experimental label on activity
grep -n "experimental\|EXPERIMENTAL" docs/index.html

# 6. Confirm footer links are updated
grep -n "alby-tomy.github.io/README\|alby-tomy.github.io/PROMPT" docs/index.html
# The above should return nothing if links are fixed
```

Report the output of every check. If any check fails, fix it before
finishing.

---

## Delivery checklist

Mark each done only after the accuracy pass confirms it:

```
FIXES TO EXISTING CONTENT
  [ ] --namespace flag in init table — verified in cmd/init.go first
  [ ] Namespace row in Philosophy table
  [ ] Identity model paragraph in Philosophy
  [ ] [experimental] on activity — verified in cmd/activity.go first
  [ ] Namespace mention in walkthrough step 1
  [ ] "fetch coming soon" note in walkthrough step 3
  [ ] Windows path note (~/.gitcollect → %USERPROFILE%)
  [ ] Footer links fixed and verified

NEW INSTALL SECTION
  [ ] go install (with real module path from go.mod)
  [ ] Linux binary — both amd64 and arm64 (real filenames from goreleaser)
  [ ] macOS binary — both arm64 and amd64 + quarantine note
  [ ] Windows binary — full step-by-step PATH instructions
  [ ] Build from source
  [ ] Homebrew marked as coming soon
  [ ] Verify install block (real version output from cmd/version.go)
  [ ] Shell completion — bash, zsh, fish, powershell (real subcommand)

ACCURACY PASS
  [ ] No grant/revoke in HTML
  [ ] No placeholder text remaining
  [ ] namespace appears in init section
  [ ] Identity model paragraph confirmed present
  [ ] experimental label confirmed present
  [ ] Footer links confirmed fixed

FINAL CHECK
  [ ] go build ./... — clean (HTML edits cannot break Go)
  [ ] docs/index.html opens in browser without broken layout
  [ ] All nav links (#install, #philosophy, etc.) still anchor correctly
  [ ] Update PROMPT.md progress tracker with session log entry
```
