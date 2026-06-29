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

// cachedClient and cachedUser memoize the authenticated client and caller
// identity for the lifetime of one command invocation, satisfying the
// "GetAuthenticatedUser called once per invocation" efficiency rule. A
// process only ever runs one command, so these never need to be cleared.
var (
	cachedClient api.Client
	cachedUser   string
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

// currentUser returns the authenticated username for client, calling
// GetAuthenticatedUser at most once per command invocation. The result is
// also cached to config so other commands (e.g. list) can resolve "who am
// I" from disk without a network call.
func currentUser(client api.Client) (string, error) {
	if cachedUser != "" {
		return cachedUser, nil
	}
	user, err := client.GetAuthenticatedUser()
	if err != nil {
		return "", fmt.Errorf("could not verify identity: %w", err)
	}
	cachedUser = user
	if err := config.SaveUser(client.Host(), user); err != nil {
		output.Warn("could not cache username for %s: %v", client.Host(), err)
	}
	return user, nil
}

// loadCollection loads name for an owner-perspective command (init, delete,
// visibility, member/group management, repo access) and maps a missing
// manifest to gitcollect's standard, friendly not-found message. These
// commands inherently require the caller to already know whether their own
// collection exists, so there is nothing to disclose by being specific.
func loadCollection(name string) (*collection.Collection, error) {
	col, err := collection.Load(name)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			return nil, fmt.Errorf("collection %q not found. Run: gitcollect list", name)
		}
		return nil, err
	}
	return col, nil
}

// loadForRead loads name for a read/discovery command (show, inspect,
// clone, pull, status) and enforces collection-level access, resolving the
// caller's identity only if the collection turns out to be private (public
// collections need no authentication at all). A missing manifest and an
// access-denied result produce the exact same access.ErrForbidden so a
// private collection's existence is never disclosed to a non-member.
// caller is "" for public collections, since no identity was needed.
func loadForRead(name string) (col *collection.Collection, caller string, err error) {
	col, err = collection.Load(name)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			return nil, "", access.ErrForbidden
		}
		return nil, "", err
	}

	if col.Visibility == collection.VisibilityPublic {
		return col, "", nil
	}

	client, err := currentClient(col.Host)
	if err != nil {
		return nil, "", err
	}
	caller, err = currentUser(client)
	if err != nil {
		return nil, "", err
	}
	if err := access.CheckCollectionAccess(col, caller); err != nil {
		return nil, "", err
	}
	return col, caller, nil
}

// loadForGit loads name for clone/pull/status: unlike loadForRead, it always
// resolves an authenticated client and caller, even for public collections,
// because these commands need a real api.Client to fetch clone URLs and
// verify platform collaborator status — there is no client-free path for
// them the way there is for purely-local commands like show or inspect.
func loadForGit(name string) (col *collection.Collection, caller string, client api.Client, err error) {
	col, err = collection.Load(name)
	if err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			return nil, "", nil, access.ErrForbidden
		}
		return nil, "", nil, err
	}

	client, err = currentClient(col.Host)
	if err != nil {
		return nil, "", nil, err
	}
	caller, err = currentUser(client)
	if err != nil {
		return nil, "", nil, err
	}
	if err := access.CheckCollectionAccess(col, caller); err != nil {
		return nil, "", nil, err
	}
	return col, caller, client, nil
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
