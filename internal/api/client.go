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
	Host() string
}

// RepoInfo is the subset of platform repository metadata gitcollect needs.
type RepoInfo struct {
	Name     string
	CloneURL string // always HTTPS
	Private  bool
	Archived bool
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
