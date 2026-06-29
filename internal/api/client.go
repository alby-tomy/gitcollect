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
	GetAuthenticatedUser() (string, error)
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
	Host() string
}

// GitHubNotificationsURL is where a user accepts a pending GitHub
// collaborator invitation. There's no API endpoint to accept one
// programmatically — it's always a manual, web-based step.
const GitHubNotificationsURL = "https://github.com/notifications"

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

var (
	ErrNotFound     = errors.New("repository not found")
	ErrUnauthorized = errors.New("invalid or missing token")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrRateLimit    = errors.New("API rate limit exceeded")
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
