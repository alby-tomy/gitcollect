# gitcollect — Feature: Dynamic Scalability & Group Admins

> Read this file completely before touching any code.
> Verify the clean baseline first. Work through the delivery
> checklist in exact order. Flag any deviation before acting.
> go build ./... and go test ./... after every file changed.

---

## Context — why this feature exists

The current gitcollect model has one role: owner. The owner created
the collection and is the only person who can mutate it. Everyone
else is a member. This works perfectly for small teams (2-10 people,
one person clearly in charge).

It breaks at scale. An ecommerce platform with 10 teams and 150+
people means the collection owner becomes a human bottleneck:

  "Hey CTO, can you add john@ to the payments-team group?"
  [CTO is in a meeting]
  "Also sarah@ is leaving, can you remove her?"
  [2 hours later]

At that scale, the tool gets abandoned — not because the core idea
is wrong, but because the operational model does not scale.

This feature adds a progressive tier system. Collections stay simple
by default. Large teams opt into group admin delegation when they
actually need it. The complexity is invisible until it is useful.

---

## The three tiers

```
PERSONAL (default)
  Owner only. No members, no groups, no admin overhead.
  Perfect for: one developer, private tool collection.
  group_admins_enabled: false (implicit)

TEAM (default when members exist)
  Owner + members + groups. Owner manages everything.
  Perfect for: 2-30 people, one clear lead.
  group_admins_enabled: false

ORGANISATION (opt-in)
  Owner + group admins + members + groups.
  Each group has an admin who manages that group independently.
  Owner handles structure only: new groups, repo access, visibility.
  Perfect for: 30+ people, multiple team leads.
  group_admins_enabled: true
```

Switching between tiers is one command. No migration, no data loss,
no starting over. A team that grows from TEAM to ORGANISATION just
runs one command and the feature unlocks.

---

## Feature 1 — Ownership transfer

### What it does

Transfers collection ownership from the current owner to another
member. The previous owner becomes a regular member and retains
access. The new owner gets full admin control.

### Why it is needed before group admins

Without transfer, a collection is permanently tied to its creator.
If the creator leaves a company or project, nobody can take over.
Transfer is small in scope and is the logical prerequisite to any
multi-admin model.

### Command

```bash
gitcollect transfer <collection> <new-owner-username>
```

### Behaviour

1. Verify caller is the current owner
2. Verify new-owner-username is already a collection member
   - If not: error with exact command to add them first
3. Call client.GetUser(new-owner-username) to resolve their platform ID
4. Print a summary of what will change and require typed confirmation:

```
$ gitcollect transfer cybersecurity alice

⚠ This will transfer ownership of cybersecurity to alice.
  You (bob) will become a regular member.
  This action cannot be undone by you — only alice can transfer it back.

Type "alice" to confirm: alice

✓ Transferred cybersecurity to alice
  You have been added as a member with full access
  Run: gitcollect show cybersecurity  to verify the new state
```

5. Update col.OwnerID and col.Owner (cached login) to the new owner
6. If the previous owner is NOT already in Members, add them
7. Append audit entry: action "collection.transfer", target = new owner login
8. Save atomically

### Validation rules

- Cannot transfer to yourself (no-op, return clear error)
- Cannot transfer to a non-member (must join collection first)
- Cannot transfer if group_admins_enabled and the new owner is a
  group admin — they must be removed as group admin first to avoid
  role ambiguity (a group admin who is also the owner is confusing)
- Requires typing the new owner's exact username to confirm

### New command file

```
cmd/transfer.go
```

### Tests

```
TestTransfer_RequiresOwner          — non-owner caller returns ErrNotOwner
TestTransfer_RequiresMember         — non-member target returns guided error
TestTransfer_SelfTransfer           — same user returns ErrSelfTransfer
TestTransfer_UpdatesOwnerID         — col.OwnerID updated to new owner's ID
TestTransfer_PreviousOwnerAddedAsMember — previous owner lands in Members
TestTransfer_AuditEntry             — collection.transfer action logged
TestTransfer_RequiresTypedConfirm   — wrong confirmation word aborts
```

---

## Feature 2 — Dynamic scalability (group admins)

### The YAML change

Add two new fields to the Collection struct:

```go
type Collection struct {
    // ... existing fields ...

    // GroupAdminsEnabled controls whether the organisation tier is
    // active. When false (default), only the owner can mutate groups.
    // When true, group admins listed in GroupAdmins can manage their
    // own group's membership independently.
    GroupAdminsEnabled bool `yaml:"group_admins_enabled,omitempty"`

    // GroupAdmins maps group name → list of member platform IDs who
    // are admins of that group. Only populated when
    // GroupAdminsEnabled is true.
    GroupAdmins map[string][]string `yaml:"group_admins,omitempty"`
}
```

Updated YAML example:

```yaml
version: "2"
name: ecom
host: github.com
owner: "583231"
visibility: private
group_admins_enabled: true

groups:
  payments-team:
    - "923841"
    - "112938"
  mobile-team:
    - "884729"
    - "441892"

group_admins:
  payments-team:
    - "923841"    # payments-lead — can manage payments-team members only
  mobile-team:
    - "884729"    # mobile-lead — can manage mobile-team members only

logins:
  "583231": cto
  "923841": payments-lead
  "112938": alice
  "884729": mobile-lead
  "441892": bob
```

### The opt-in prompt during init

When `gitcollect init` is run interactively (stdout is a TTY), after
creating the collection, show a single optional prompt:

```
$ gitcollect init ecom

✓ Created collection: ecom (private)

Will this collection be managed by multiple team leads? [y/N]: y
✓ Group admin support enabled
  Use: gitcollect group admin add ecom <group> <username>
  to assign a team lead who can manage their group independently.
```

If the user answers N (or presses Enter), group_admins_enabled stays
false and the prompt is never shown again for this collection.

If stdout is NOT a TTY (non-interactive context), skip the prompt
entirely. group_admins_enabled defaults to false silently.

The prompt must use output.Confirm() so it respects the existing
non-interactive detection pattern already used by other commands.

### The scale switch command

Users can switch tiers at any time, in either direction:

```bash
# Enable group admin delegation (TEAM → ORGANISATION)
gitcollect scale <collection> organisation

# Disable group admin delegation (ORGANISATION → TEAM)
gitcollect scale <collection> team
```

Switching to `organisation` with no group admins assigned yet:
```
$ gitcollect scale ecom organisation

✓ Group admin support enabled for ecom
  No group admins are assigned yet.
  Assign team leads with: gitcollect group admin add ecom <group> <username>
```

Switching back to `team`:
```
$ gitcollect scale ecom team

⚠ This will remove group admin privileges from:
  payments-team: payments-lead
  mobile-team:   mobile-lead
  They will become regular members. Only you (cto) can manage groups.

Continue? [y/N]: y
✓ Group admin support disabled for ecom
```

If switching to team while group admins are assigned, always
show the full list of who loses admin rights and require y/N
confirmation before proceeding.

New command file: `cmd/scale.go`

### New group admin management commands

```bash
# Assign a group admin (owner-only)
gitcollect group admin add <collection> <group> <username>

# Remove a group admin (owner-only, or self-removal)
gitcollect group admin remove <collection> <group> <username>

# List group admins for all groups (owner sees all, members see their own)
gitcollect group admin list <collection>
```

These are subcommands of `group admin` — a new sub-level under the
existing `gitcollect group` command. Cobra supports this natively.

Add to `cmd/group.go` as nested subcommands under a new `adminCmd`.

### Authorization model — who can do what

This is the core of the feature. Every mutation command must check
this table before acting. Read carefully — this is the exact logic
to implement, not a summary:

```
COMMAND                          OWNER    GROUP ADMIN        MEMBER
─────────────────────────────────────────────────────────────────────
collection init/delete/transfer  ✓        ✗                  ✗
visibility change                ✓        ✗                  ✗
scale organisation/team          ✓        ✗                  ✗
member add/remove                ✓        ✗                  ✗
group create/delete              ✓        ✗                  ✗
group admin add/remove           ✓        ✗ (cannot self-assign) ✗
group add <group> <user>         ✓        ✓ (own group only) ✗
group remove <group> <user>      ✓        ✓ (own group only) ✗
group list/show                  ✓        ✓ (all groups)     ✓
repo access/open                 ✓        ✗                  ✗
repo add/remove                  ✓        ✗                  ✗
clone/pull/sync/status           ✓        ✓                  ✓
inspect/show/audit               ✓        ✓                  ✓
```

"own group only" means: a group admin of `payments-team` can add
and remove members of `payments-team` only. Attempting to modify
`mobile-team` returns:

```
✗ group add: you are a group admin of payments-team, not mobile-team
  Only the collection owner (cto) can manage mobile-team membership.
```

### New helper — isGroupAdmin

Add to internal/collection/access.go:

```go
// IsGroupAdmin returns true if callerID is a group admin of the
// named group AND group_admins_enabled is true on the collection.
// Returns false in all other cases, including when the collection
// is in TEAM mode (group_admins_enabled: false).
func (c *Collection) IsGroupAdmin(callerID, group string) bool {
    if !c.GroupAdminsEnabled {
        return false
    }
    admins, ok := c.GroupAdmins[group]
    if !ok {
        return false
    }
    for _, id := range admins {
        if id == callerID {
            return true
        }
    }
    return false
}

// CanManageGroup returns true if callerID can add/remove members
// of the named group. True for: collection owner, group admin of
// that specific group (when GroupAdminsEnabled is true).
func (c *Collection) CanManageGroup(callerID, group string) bool {
    return c.IsOwner(callerID) || c.IsGroupAdmin(callerID, group)
}
```

### Changes to existing commands

#### cmd/group.go — group add and group remove

The current authorization check is:
```go
if caller != col.Owner { return ErrNotOwner }
```

Replace with:
```go
if !col.CanManageGroup(callerID, groupName) {
    if col.GroupAdminsEnabled && col.IsGroupAdmin(callerID, otherGroup) {
        // They are a group admin, just not of this group
        return fmt.Errorf(
            "group add: you are a group admin of %s, not %s\n"+
            "  Only the collection owner (%s) can manage %s membership.",
            theirGroup, groupName, col.Logins[col.Owner], groupName,
        )
    }
    return fmt.Errorf(
        "group add: only %s (the owner) can manage groups in %q",
        col.Logins[col.Owner], name,
    )
}
```

This gives a specific, helpful error to group admins who try to
manage the wrong group, versus a generic error to plain members.

#### cmd/group.go — group create and group delete

These remain owner-only. No change to their authorization check.

#### cmd/show.go — show group admins when enabled

When group_admins_enabled is true, the show output must include
a GROUPS table with an ADMIN column:

```
GROUPS
NAME             MEMBERS   ADMIN
payments-team    12        payments-lead
mobile-team       8        mobile-lead
devops-team       5        devops-lead
frontend-team     6        (none assigned)
```

When group_admins_enabled is false, the GROUPS table stays as it
is today (no ADMIN column).

### New helper function — findCallerGroups

Used to generate the specific error message when a group admin
tries to manage the wrong group:

```go
// GroupAdminOf returns the list of group names callerID is an admin of.
// Returns empty slice if group_admins_enabled is false or caller is
// not an admin of any group.
func (c *Collection) GroupAdminOf(callerID string) []string
```

### Mutation safety rules

- Group admin assignment is always owner-only — a group admin cannot
  appoint another group admin (prevents privilege escalation)
- Group admin removal is owner-only OR self-removal (a group admin
  can step down voluntarily)
- GroupAdmins entries are cleaned up automatically when a member is
  removed from the collection (RemoveMember must also remove them
  from GroupAdmins if present)
- GroupAdmins entries are cleaned up when a group is deleted
  (DeleteGroup must clear the GroupAdmins[group] entry)
- If GroupAdminsEnabled is false, GroupAdmins is ignored in all
  access checks even if it has data — the flag is the gate, not
  the presence of data

---

## Project structure changes

```
New files:
  cmd/transfer.go              — gitcollect transfer command
  cmd/transfer_test.go
  cmd/scale.go                 — gitcollect scale organisation|team
  cmd/scale_test.go

Modified files:
  internal/collection/collection.go   — GroupAdminsEnabled, GroupAdmins fields
  internal/collection/access.go       — IsGroupAdmin, CanManageGroup, GroupAdminOf
  internal/collection/mutation.go     — RemoveMember cleans GroupAdmins;
                                         DeleteGroup cleans GroupAdmins
  internal/collection/collection_test.go — new struct tests
  cmd/group.go                        — group add/remove auth updated;
                                         group admin add/remove/list added
  cmd/group_test.go                   — new auth path tests
  cmd/init.go                         — optional scale prompt on init
  cmd/init_test.go                    — prompt shown/hidden correctly
  cmd/show.go                         — ADMIN column when enabled
  cmd/show_test.go                    — owner view with admin column
```

---

## Implementation order

Work strictly in this order. Each step must compile and pass
go test ./... before the next step starts.

```
Step 1 — internal/collection/collection.go
  Add GroupAdminsEnabled bool and GroupAdmins map[string][]string fields.
  Update Validate() to check that every ID in GroupAdmins is also in
  Members (orphaned group admin IDs are a validation error).

Step 2 — internal/collection/access.go
  Add IsGroupAdmin, CanManageGroup, GroupAdminOf.
  Add tests: IsGroupAdmin when disabled, when enabled but wrong group,
  when enabled and correct group. CanManageGroup for owner/admin/member.

Step 3 — internal/collection/mutation.go
  Update RemoveMember: after removing from Members and all Groups, also
  remove from GroupAdmins across all groups. Log each group admin removal
  as a separate audit entry.
  Update DeleteGroup: clear GroupAdmins[group] if present.

Step 4 — cmd/transfer.go
  Full implementation with typed confirmation, audit log, previous owner
  added to Members, all validation rules. Tests listed above.

Step 5 — cmd/scale.go
  gitcollect scale <collection> organisation|team.
  Confirmation prompt when disabling (lists who loses admin rights).
  Audit entry for both directions.

Step 6 — cmd/group.go
  Add group admin add/remove/list as nested subcommands.
  Update group add/remove authorization to use CanManageGroup.
  Update show output to include ADMIN column when enabled.

Step 7 — cmd/init.go
  Add opt-in prompt at end of runInit (TTY only, skipped non-interactively).

Step 8 — All test files
  Tests listed per-file above, plus the full authorization matrix below.
```

---

## Test coverage requirements

### Authorization matrix — every row must be covered

```
Scenario                                             Expected
──────────────────────────────────────────────────   ─────────────────────────────
owner runs group admin add                         → allowed
owner runs group add (any group)                   → allowed
group admin runs group add (own group)             → allowed
group admin runs group add (different group)       → ErrWrongGroup with specific msg
group admin runs group admin add (privilege esc.)  → ErrNotOwner
group admin runs member add                        → ErrNotOwner
group admin runs group create                      → ErrNotOwner
group admin runs repo access                       → ErrNotOwner
group admin runs scale                             → ErrNotOwner
member runs group add                              → ErrNotOwner
member runs group admin add                        → ErrNotOwner
IsGroupAdmin when GroupAdminsEnabled=false         → false always
IsGroupAdmin when GroupAdminsEnabled=true, correct → true
IsGroupAdmin when GroupAdminsEnabled=true, wrong   → false
CanManageGroup owner                               → true always
CanManageGroup group admin own group               → true
CanManageGroup group admin other group             → false
CanManageGroup member                              → false
RemoveMember removes from GroupAdmins              → GroupAdmins entry cleaned
DeleteGroup removes from GroupAdmins               → GroupAdmins[group] cleared
Transfer to non-member                             → guided error with fix command
Transfer updates OwnerID to new owner's ID         → col.OwnerID = new ID
Transfer previous owner added to Members           → previous owner in Members
Scale to team shows who loses admin rights         → confirmation lists all admins
Scale to team with wrong confirm word              → aborted, no change
```

### New sentinel errors

Define in internal/collection/access.go or mutation.go:

```go
var (
    ErrGroupAdminsDisabled = errors.New(
        "group admin support is not enabled — run: gitcollect scale <collection> organisation")
    ErrWrongGroup = errors.New(
        "you are a group admin but not of this group")
    ErrSelfTransfer = errors.New(
        "cannot transfer ownership to yourself")
    ErrAdminPrivilegeEscalation = errors.New(
        "group admins cannot assign other group admins")
)
```

---

## What this feature explicitly does NOT add

These are out of scope. Do not implement them even if they seem
like natural extensions:

- Repo-level admins (only group admins, not per-repo admins)
- Group admins managing repo access rules (owner-only)
- Group admins viewing the full audit log (owner-only)
- Group admins adding new members to the collection (owner-only)
- Group admins creating or deleting groups (owner-only)
- Multiple owners or co-owners (one owner, always)
- Role hierarchy beyond owner/group-admin/member
- Permission inheritance between groups
- Group admin notifications when their group changes

If any of these are requested during implementation, flag it and
wait for explicit approval before building it.

---

## Audit log entries

Every new mutation must write an audit entry:

```
collection.transfer    → target = new owner login
scale.organisation     → detail = "Group admin support enabled"
scale.team             → detail = "Group admin support disabled; N admins revoked"
group.admin.add        → target = username, detail = "Added as admin of <group>"
group.admin.remove     → target = username, detail = "Removed as admin of <group>"
```

---

## Commit message when complete

```
feat: ownership transfer + dynamic group admin scalability

- gitcollect transfer: owner can hand off a collection to a member
- gitcollect scale organisation|team: opt-in group admin delegation
- gitcollect group admin add/remove/list: assign team leads per group
- Group admins can manage their own group's membership independently
- Authorization matrix: group admins cannot manage other groups,
  cannot assign other admins, cannot touch repo access or members
- opt-in prompt on init for teams that need delegation from day one
- GroupAdmins cleaned up automatically on RemoveMember and DeleteGroup
- Full audit trail for all new mutations
- Authorization matrix test coverage for all role/command combinations
```

---

## Delivery checklist

```
STEP 0
  [ ] go build ./... clean
  [ ] go test ./... all green

IMPLEMENTATION
  [ ] Step 1: collection.go — new fields + Validate update
  [ ] Step 2: access.go — IsGroupAdmin, CanManageGroup, GroupAdminOf + tests
  [ ] Step 3: mutation.go — RemoveMember + DeleteGroup cleanup + tests
  [ ] Step 4: cmd/transfer.go + tests (all 7 test cases)
  [ ] Step 5: cmd/scale.go + tests
  [ ] Step 6: cmd/group.go — admin subcommands + auth update + show ADMIN column
  [ ] Step 7: cmd/init.go — opt-in prompt
  [ ] Step 8: all test files — auth matrix coverage

SENTINEL ERRORS
  [ ] ErrGroupAdminsDisabled
  [ ] ErrWrongGroup
  [ ] ErrSelfTransfer
  [ ] ErrAdminPrivilegeEscalation

AUDIT TRAIL
  [ ] collection.transfer logged
  [ ] scale.organisation logged
  [ ] scale.team logged
  [ ] group.admin.add logged
  [ ] group.admin.remove logged

AUTHORIZATION MATRIX
  [ ] All rows in test matrix covered
  [ ] group admin cannot escalate privileges
  [ ] group admin cannot manage other groups
  [ ] GroupAdminsEnabled=false blocks all group admin paths

FINAL
  [ ] go build ./... clean
  [ ] go test ./... all green
  [ ] go vet ./... clean
  [ ] PROMPT.md progress tracker updated
```
