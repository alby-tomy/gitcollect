// Package api abstracts the GitHub and GitLab collaborator APIs behind a
// single Client interface. gitcollect never maintains a shadow permission
// system: every access mutation drives one of these calls to completion.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// requestTimeout bounds every HTTP request made to a platform API.
const requestTimeout = 15 * time.Second

// Client is gitcollect's abstraction over the GitHub and GitLab collaborator
// APIs.
type Client interface {
	GetRepo(owner, repo string) (RepoInfo, error)
	GetAuthenticatedUser() (UserInfo, error)
	// GetUser resolves username (as typed on the command line — by "member
	// add", "group add", "repo grant", "repo access --users", or during
	// old-format-collection migration) to that account's platform identity.
	// Returns ErrUserNotFound if no account with that username exists.
	GetUser(username string) (UserInfo, error)
	AddCollaborator(owner, repo, username, permission string) error
	RemoveCollaborator(owner, repo, username string) error
	CheckCollaborator(owner, repo, username string) (bool, error)
	// GetPendingInvite returns true if username has been granted access to
	// owner/repo but hasn't accepted it yet. GitHub creates a pending
	// "repository invitation" whenever AddCollaborator grants someone who
	// isn't already org-level entitled — they show up as NOT a confirmed
	// collaborator (CheckCollaborator false) until they accept it, which
	// otherwise looks identical to never having been granted at all.
	// GitLab has no equivalent state — project membership added via its
	// API takes effect immediately — so gitlabClient always returns false.
	GetPendingInvite(owner, repo, username string) (bool, error)
	// ListCommits returns the most recent commits on branch, newest first,
	// capped at limit. Used by "gitcollect activity" to report code changes
	// — distinct from the collaborator methods above, which gitcollect's
	// access-control mutations drive.
	ListCommits(owner, repo, branch string, limit int) ([]CommitInfo, error)
	// CreateRepo creates a new repository under the given owner/org.
	// name must be a valid repo name (validated before calling).
	// private controls visibility. description may be empty.
	// Returns the created repo's info (including its clone URL) on success.
	// Returns ErrNameConflict if the repo already exists (race condition guard).
	// Returns ErrForbidden if the authenticated user cannot create repos
	// under the given owner (e.g. not a member of the org).
	CreateRepo(owner, name string, private bool, description string) (RepoInfo, error)
	// ListOrgTeams returns all teams in the org, handling pagination.
	// Requires read:org scope on GitHub.
	// On GitLab: returns subgroups of the group.
	ListOrgTeams(org string) ([]TeamInfo, error)
	// ListTeamMembers returns members of a team.
	// role: "" = all, "maintainer" = maintainers only, "member" = non-maintainers.
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
	// SearchRepos finds repos in org matching a name pattern or topic.
	// pattern is a glob-style string (e.g. "payments-*"); empty = skip.
	// topic is a GitHub topic name (e.g. "payments"); empty = skip.
	// Returns up to limit repos (max 100). GitHub only; GitLab returns an error.
	SearchRepos(org, pattern, topic string, limit int) ([]RepoInfo, error)
	// ListOrgRepos returns all repositories in an org/group, handling pagination.
	// GitHub: GET /orgs/{org}/repos?type=all&per_page=100
	// GitLab: GET /groups/{group}/projects?per_page=100
	// Archived repos are included; callers may filter them out.
	ListOrgRepos(org string) ([]RepoInfo, error)
	// ListOpenPRs returns open pull requests (GitHub) or open merge requests
	// (GitLab) for a single repo, handling pagination.
	ListOpenPRs(owner, repo string) ([]PRInfo, error)
	Host() string
}

// GitHubNotificationsURL is where a user accepts a pending GitHub
// collaborator invitation. There's no API endpoint to accept one
// programmatically — it's always a manual, web-based step.
const GitHubNotificationsURL = "https://github.com/notifications"

// PermissionPull is the only permission gitcollect ever grants. A
// collection says who may *reach* a repo, never what they may do once
// there — write access stays whatever the platform already granted
// through org or team membership, which gitcollect does not manage.
//
// Every AddCollaborator call must pass this rather than a literal. The
// literal is how the two call sites drifted apart: SyncCollaborators
// granted "pull" while diff --fix granted "push", quietly upgrading every
// member it repaired from read to write across every repo they could
// reach. One constant means a future call site cannot make that mistake
// without writing the wrong thing on purpose.
const PermissionPull = "pull"

// UserInfo identifies a platform account. ID is the platform's own
// immutable numeric identifier, stable across username/login renames —
// gitcollect stores this, never the login, anywhere it needs to decide
// "is this the same person" (Collection.Owner, Members, group membership,
// per-repo individual grants). Login is the current, human-readable
// username: required for API path-building (GitHub and GitLab both
// address repos and collaborators by login in the URL, never by ID) and
// anywhere gitcollect displays or types out a command involving this
// person. Both are carried as strings — ID is numeric on both platforms,
// but kept as a string here so gitcollect's storage/comparison code never
// has to special-case GitHub's int64 range vs GitLab's int.
type UserInfo struct {
	ID    string
	Login string
}

// RepoInfo is the subset of platform repository metadata gitcollect needs.
type RepoInfo struct {
	Name          string
	CloneURL      string // always HTTPS
	DefaultBranch string
	Private       bool
	Archived      bool
}

// CommitInfo is the subset of platform commit metadata gitcollect needs to
// report code activity. Author is the platform username when the platform
// can resolve one (GitHub links a commit to an account); it falls back to
// the raw commit author name otherwise (notably always on GitLab, which
// does not expose the pushing user's username on this endpoint).
type CommitInfo struct {
	SHA         string
	Author      string
	Message     string // first line only
	CommittedAt time.Time
}

// PRInfo is the subset of pull-request (GitHub) or merge-request (GitLab)
// metadata gitcollect needs to list open work across a collection.
type PRInfo struct {
	Number    int
	Title     string
	Author    string
	State     string // "open" (GitHub) | "opened" (GitLab)
	URL       string
	Repo      string // repo name within the collection namespace
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TeamInfo describes a single team (GitHub) or subgroup (GitLab) returned
// by ListOrgTeams.
type TeamInfo struct {
	ID          int64
	Name        string // display name e.g. "Payments Team"
	Slug        string // URL-safe e.g. "payments-team"
	Description string
	Privacy     string // "closed" | "secret" (GitHub) | "private" (GitLab)
	ParentSlug  string // non-empty for nested teams
}

var (
	ErrNotFound     = errors.New("repository not found")
	ErrUnauthorized = errors.New("invalid or missing token")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrRateLimit    = errors.New("API rate limit exceeded")
	// ErrUserNotFound is returned by GetUser when no account with the
	// given username exists on the platform — distinct from ErrNotFound,
	// which means "repository not found", so callers can tell the two
	// apart in error messages.
	ErrUserNotFound = errors.New("platform user not found")
	// ErrNameConflict is returned by CreateRepo when the repository already
	// exists — either because of a race condition between the existence
	// check and the create call, or because the repo was created on the
	// platform by another means between the two. Callers treat this as
	// success: the repo exists, which is what we wanted.
	ErrNameConflict = errors.New("repository already exists")
)

// NewClient returns the Client implementation for host: GitHub for
// "github.com", GitLab (cloud or self-hosted) for everything else.
func NewClient(host, token string) Client {
	if host == "github.com" {
		return newGitHubClient(host, token)
	}
	return newGitLabClient(host, token)
}

func newHTTPClient() *http.Client {
	return &http.Client{}
}

// classifyStatus maps a non-2xx HTTP status code to one of api's sentinel
// errors, or a generic error for anything unrecognised. Callers must handle
// any status codes that carry their own meaning (e.g. GitHub's 204/404
// pair for collaborator checks) before falling back to this.
func classifyStatus(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimit
	default:
		return fmt.Errorf("unexpected status %d", statusCode)
	}
}
