// Package cmd implements the gitcollect command-line interface.
package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/access"
	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/output"
)

// Exit codes, per gitcollect's CLI conventions: 0 success, 1 operational
// error, 2 usage/argument error.
const (
	ExitSuccess = 0
	ExitError   = 1
	ExitUsage   = 2
)

// appVersion holds the build version injected by main via SetVersion.
var appVersion = "dev"

// ranPersistentPreRun becomes true once cobra has resolved the command,
// parsed its flags, and validated its arguments — i.e. once it actually
// starts running command logic. Any error returned before that point
// (unknown command, unknown flag, wrong argument count) is a usage error;
// any error returned after it came from the command's own logic.
var ranPersistentPreRun bool

var rootCmd = &cobra.Command{
	Use:   "gitcollect",
	Short: "Group GitHub/GitLab repositories into access-controlled collections",
	Long: `gitcollect groups GitHub/GitLab repositories into named collections and
controls who can access them, at both the collection level (membership) and
the repo level (which groups or individuals can reach which repos).

It wraps Git and the GitHub/GitLab APIs: every access change gitcollect makes
locally is driven through to the real platform via its collaborator API
before the local YAML is ever written. The two never diverge.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ranPersistentPreRun = true
		return nil
	},
}

// UsageError marks an error as a usage/argument problem (exit code 2)
// rather than an operational failure (exit code 1). Commands wrap errors
// caused by malformed input — invalid names, conflicting flags — with
// NewUsageError so Execute reports the correct exit code even though the
// error surfaced from inside RunE rather than from cobra's own arg parsing.
type UsageError struct {
	err error
}

func (e *UsageError) Error() string { return e.err.Error() }
func (e *UsageError) Unwrap() error { return e.err }

// NewUsageError wraps err so Execute treats it as a usage error.
func NewUsageError(err error) error {
	return &UsageError{err: err}
}

// SetVersion records the build version for use by the version command.
// Must be called once, before Execute, by main.
func SetVersion(v string) {
	appVersion = v
}

// Execute runs the root command and returns the process exit code to use.
// It never calls os.Exit itself so the caller retains control of shutdown.
func Execute() int {
	err := rootCmd.Execute()
	if err == nil {
		return ExitSuccess
	}

	output.Error("%s", err.Error())

	// A stored token that the platform itself has since rejected (expired,
	// revoked, scopes changed) surfaces here as api.ErrUnauthorized from
	// deep inside whatever API call the command happened to make — there's
	// no single call site to attach this hint to, so it's centralized here
	// instead, the same way the no-token-at-all case is hinted at the point
	// currentClient first discovers ErrNotAuthenticated.
	if errors.Is(err, api.ErrUnauthorized) {
		output.Suggestion("gitcollect auth")
	}

	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return ExitUsage
	}
	if !ranPersistentPreRun {
		return ExitUsage
	}
	return ExitError
}

// cachedClient, cachedUser, and cachedUserID memoize the authenticated
// client and caller identity for the lifetime of one command invocation,
// satisfying the "GetAuthenticatedUser called once per invocation" rule.
// A process only ever runs one command, so these never need to be
// cleared. cachedUser is the caller's login (display/audit); cachedUserID
// is their platform ID (the form collection.Collection.IsOwner/IsMember/
// CanAccessRepo expect) — same login/ID split as collection.Collection's
// own Owner/Members + Logins.
var (
	cachedClient api.Client
	cachedUser   string
	cachedUserID string
)

// currentClient returns an authenticated api.Client for host, loading the
// stored token from ~/.gitcollect/config.
func currentClient(host string) (api.Client, error) {
	if cachedClient != nil && cachedClient.Host() == host {
		return cachedClient, nil
	}
	token, err := config.LoadToken(host)
	if err != nil {
		if errors.Is(err, config.ErrNotAuthenticated) {
			return nil, fmt.Errorf("not authenticated. Run: gitcollect auth")
		}
		return nil, err
	}
	cachedClient = api.NewClient(host, token)
	return cachedClient, nil
}

// currentUserInfo returns the authenticated platform identity for client
// — both login and ID — calling GetAuthenticatedUser at most once per
// command invocation. The result is also cached to config (both forms)
// so other commands (e.g. list) can resolve "who am I" from disk without
// a network call. currentUser and currentUserID below are thin accessors
// over this same cached resolution, for the common case where a caller
// only needs one half of the identity.
func currentUserInfo(client api.Client) (api.UserInfo, error) {
	if cachedUser != "" {
		return api.UserInfo{ID: cachedUserID, Login: cachedUser}, nil
	}
	user, err := client.GetAuthenticatedUser()
	if err != nil {
		return api.UserInfo{}, fmt.Errorf("could not verify identity: %w", err)
	}
	cachedUser = user.Login
	cachedUserID = user.ID
	if err := config.SaveUser(client.Host(), user.Login); err != nil {
		output.Warn("could not cache username for %s: %v", client.Host(), err)
	}
	if err := config.SaveUserID(client.Host(), user.ID); err != nil {
		output.Warn("could not cache user ID for %s: %v", client.Host(), err)
	}
	return user, nil
}

// currentUser returns the authenticated login for client — see
// currentUserInfo. Used everywhere gitcollect displays or audits the
// caller's identity.
func currentUser(client api.Client) (string, error) {
	user, err := currentUserInfo(client)
	if err != nil {
		return "", err
	}
	return user.Login, nil
}

// currentUserID returns the authenticated platform ID for client — see
// currentUserInfo. Used everywhere gitcollect compares the caller's
// identity against a collection.Collection (IsOwner, IsMember,
// CanAccessRepo) — never the login, which can change.
func currentUserID(client api.Client) (string, error) {
	user, err := currentUserInfo(client)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

// loadCollection loads name and maps a missing manifest to gitcollect's
// standard, friendly not-found message. Used internally by loadForOwner
// below — every owner-perspective command (init, delete, visibility,
// member/group management, repo access) inherently requires the caller to
// already know whether their own collection exists, so there is nothing
// to disclose by being specific.
func loadCollection(name string) (*collection.Collection, error) {
	col, err := collection.Load(name)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			msg := fmt.Sprintf("collection %q not found. Run: gitcollect list", name)
			if suggestion := suggestCollectionName(name); suggestion != "" {
				msg += fmt.Sprintf("\nDid you mean %q?", suggestion)
			}
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return col, nil
}

// loadForOwner loads name for an owner-perspective command (add, delete,
// visibility, member/group management, repo access/grant/revoke),
// resolving the authenticated client and caller identity (both login and
// platform ID) and opportunistically migrating an old-format collection —
// this is one of the three call sites in this file allowed to do that;
// see migrateIfNeeded's doc comment for why the other two (list's bulk
// scan, loadForRead's public fast-path) must not. Deliberately does NOT
// check ownership itself: callers still write their own "only %s (the
// owner) can <the specific thing>" check and message, since those vary
// too much (which name goes in the message — collection name in most
// commands, repo name in repo.go's) to generate generically. verb is used
// only to prefix the returned error, the same convention loadForRead/
// loadForGit's callers already apply manually.
func loadForOwner(verb, name string) (col *collection.Collection, caller, callerID string, client api.Client, err error) {
	col, err = loadCollection(name)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("%s: %w", verb, err)
	}

	client, err = currentClient(col.Host)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("%s: %w", verb, err)
	}
	caller, err = currentUser(client)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("%s: %w", verb, err)
	}
	callerID, err = currentUserID(client)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("%s: %w", verb, err)
	}

	if err := migrateIfNeeded(col, client); err != nil {
		return nil, "", "", nil, fmt.Errorf("%s: %w", verb, err)
	}

	return col, caller, callerID, client, nil
}

// migrateIfNeeded bumps col from the legacy username-based format
// (Version "1") to ID-based storage (collection.CurrentVersion) if
// needed, resolving every referenced username via client and saving the
// result. No-op if col is already current. Called only from loadForOwner,
// loadForGit, and loadForRead's private-collection branch — every call
// site in this file that already holds an authenticated client for
// col.Host. Deliberately NOT called from list.go's bulk scan (which must
// stay network-free across however many different hosts the caller's
// collections span) or loadForRead's public-collection fast-path (which
// must stay auth-free) — both keep working against an unmigrated file
// exactly as before. Migration is opportunistic: triggered by whichever
// owner-perspective or git command happens to touch a given collection
// next, never guaranteed on every read.
func migrateIfNeeded(col *collection.Collection, client api.Client) error {
	if col.Version == collection.CurrentVersion {
		return nil
	}
	if err := col.Migrate(client); err != nil {
		return fmt.Errorf("could not migrate %q to ID-based access (some platform usernames could not be resolved): %w", col.Name, err)
	}
	if err := col.Save(); err != nil {
		return fmt.Errorf("could not save migrated %q: %w", col.Name, err)
	}
	output.Info("migrated %q to ID-based access", col.Name)
	return nil
}

// loginsFor maps each ID in ids to its cached login via col.Logins,
// preserving order — the standard way display code turns an ID-based
// list (col.Members, a group's member list, a repo's individually-
// granted Users) into something a human can read. An ID with no cached
// login (should not normally happen on a CurrentVersion collection — see
// Collection.Validate) falls back to printing the raw ID rather than an
// empty string, so a display bug is at least visible instead of silently
// blank.
func loginsFor(col *collection.Collection, ids []string) []string {
	logins := make([]string, len(ids))
	for i, id := range ids {
		if login := col.Logins[id]; login != "" {
			logins[i] = login
		} else {
			logins[i] = id
		}
	}
	return logins
}

// suggestCollectionName returns the closest existing local collection name
// to name (Levenshtein distance <= 2), or "" if none is close enough or
// the lookup itself fails. Only ever compared against your OWN local
// ~/.gitcollect/collections/*.yaml filenames — never against anything you
// don't already have a local file for — so this can't be used to probe
// for the existence of a private collection you're not a member of.
func suggestCollectionName(name string) string {
	names, err := collection.List()
	if err != nil {
		return ""
	}

	best, bestDist := "", 3 // distance must be <= 2 to suggest anything
	for _, candidate := range names {
		if candidate == name {
			continue
		}
		if d := levenshtein(name, candidate); d < bestDist {
			best, bestDist = candidate, d
		}
	}
	return best
}

// levenshtein returns the edit distance between a and b (insertions,
// deletions, substitutions, each cost 1) via the standard O(len(a)*len(b))
// dynamic-programming table.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}

	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// loadForRead loads name for a read/discovery command (show, inspect) and
// enforces collection-level access, resolving the caller's identity only
// if the collection turns out to be private (public collections need no
// authentication at all). A missing manifest and an access-denied result
// produce the exact same access.ErrForbidden so a private collection's
// existence is never disclosed to a non-member. caller and callerID are
// both "" for public collections, since no identity was needed. This is
// one of the three call sites allowed to opportunistically migrate an
// old-format collection — but only on the private branch below, where a
// client is already being resolved anyway; the public fast-path stays
// exactly as documented, network-free and auth-free, even against an
// old-format file. See migrateIfNeeded's doc comment.
func loadForRead(name string) (col *collection.Collection, caller, callerID string, err error) {
	col, err = collection.Load(name)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			return nil, "", "", access.ErrForbidden
		}
		return nil, "", "", err
	}

	if col.Visibility == collection.VisibilityPublic {
		return col, "", "", nil
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return nil, "", "", err
	}
	caller, err = currentUser(client)
	if err != nil {
		return nil, "", "", err
	}
	callerID, err = currentUserID(client)
	if err != nil {
		return nil, "", "", err
	}
	if err := migrateIfNeeded(col, client); err != nil {
		return nil, "", "", err
	}
	if err := access.CheckCollectionAccess(col, callerID); err != nil {
		return nil, "", "", err
	}
	return col, caller, callerID, nil
}

// loadForGit loads name for clone/pull/status/sync: unlike loadForRead, it
// always resolves an authenticated client and caller, even for public
// collections, because these commands need a real api.Client to fetch
// clone URLs and verify platform collaborator status — there is no
// client-free path for them the way there is for purely-local commands
// like show or inspect. Always migrates an old-format collection if
// needed, for the same reason: a client is always available here.
func loadForGit(name string) (col *collection.Collection, caller, callerID string, client api.Client, err error) {
	col, err = collection.Load(name)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			return nil, "", "", nil, access.ErrForbidden
		}
		return nil, "", "", nil, err
	}

	client, err = currentClient(col.Host)
	if err != nil {
		return nil, "", "", nil, err
	}
	caller, err = currentUser(client)
	if err != nil {
		return nil, "", "", nil, err
	}
	callerID, err = currentUserID(client)
	if err != nil {
		return nil, "", "", nil, err
	}
	if err := migrateIfNeeded(col, client); err != nil {
		return nil, "", "", nil, err
	}
	if err := access.CheckCollectionAccess(col, callerID); err != nil {
		return nil, "", "", nil, err
	}
	return col, caller, callerID, client, nil
}

// recordAudit appends entry to the collection's audit log. It is called
// synchronously, not from a detached goroutine: main calls os.Exit
// immediately after Execute returns, and a truly fire-and-forget append
// could be killed mid-write. A local file append is cheap enough that this
// costs nothing meaningful. Any append failure is a warning, never a
// command error — auditability must not block the operation it's auditing.
func recordAudit(entry audit.AuditEntry) {
	entry.Timestamp = time.Now().UTC()
	if err := audit.Append(entry); err != nil {
		output.Warn("could not write audit log: %v", err)
	}
}
