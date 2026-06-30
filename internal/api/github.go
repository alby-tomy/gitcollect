package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// githubBaseURL is a var, not a const, so api_test.go can point it at an
// httptest.Server. gitlabClient already has an equivalent per-instance
// baseURL field for the same reason; githubClient never needed one until
// tests required it, since production code only ever talks to the real API.
var githubBaseURL = "https://api.github.com"

type githubClient struct {
	host       string
	token      string
	httpClient *http.Client
}

func newGitHubClient(host, token string) *githubClient {
	return &githubClient{host: host, token: token, httpClient: newHTTPClient()}
}

func (c *githubClient) Host() string { return c.host }

func (c *githubClient) do(method, path string, body any) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("could not encode request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, githubBaseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("could not build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", c.host, err)
	}
	return resp, nil
}

func (c *githubClient) GetAuthenticatedUser() (UserInfo, error) {
	resp, err := c.do(http.MethodGet, "/user", nil)
	if err != nil {
		return UserInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, classifyStatus(resp.StatusCode)
	}

	var out struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return UserInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return UserInfo{ID: strconv.FormatInt(out.ID, 10), Login: out.Login}, nil
}

// GetUser resolves username to its GitHub account ID via GET /users/{username}
// — distinct from GetAuthenticatedUser, which resolves whoever the current
// token belongs to via GET /user.
func (c *githubClient) GetUser(username string) (UserInfo, error) {
	path := fmt.Sprintf("/users/%s", url.PathEscape(username))
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return UserInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return UserInfo{}, fmt.Errorf("%w: %s", ErrUserNotFound, username)
	}
	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, classifyStatus(resp.StatusCode)
	}

	var out struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return UserInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return UserInfo{ID: strconv.FormatInt(out.ID, 10), Login: out.Login}, nil
}

func (c *githubClient) GetRepo(owner, repo string) (RepoInfo, error) {
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return RepoInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RepoInfo{}, classifyStatus(resp.StatusCode)
	}

	var out struct {
		Name          string `json:"name"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		Archived      bool   `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return RepoInfo{
		Name:          out.Name,
		CloneURL:      out.CloneURL,
		DefaultBranch: out.DefaultBranch,
		Private:       out.Private,
		Archived:      out.Archived,
	}, nil
}

func (c *githubClient) AddCollaborator(owner, repo, username, permission string) error {
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(username))
	resp, err := c.do(http.MethodPut, path, map[string]string{"permission": permission})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusNoContent, http.StatusOK:
		return nil
	default:
		return classifyStatus(resp.StatusCode)
	}
}

func (c *githubClient) RemoveCollaborator(owner, repo, username string) error {
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(username))
	resp, err := c.do(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	default:
		return classifyStatus(resp.StatusCode)
	}
}

// ListCommits returns the most recent commits on branch, newest first.
// GitHub links a commit to an account when the commit's email matches a
// verified GitHub user; Author falls back to the raw commit author name
// when GitHub couldn't make that link (author is null in the response).
func (c *githubClient) ListCommits(owner, repo, branch string, limit int) ([]CommitInfo, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(branch), limit)
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyStatus(resp.StatusCode)
	}

	var out []struct {
		SHA    string `json:"sha"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	commits := make([]CommitInfo, 0, len(out))
	for _, c := range out {
		author := c.Commit.Author.Name
		if c.Author != nil && c.Author.Login != "" {
			author = c.Author.Login
		}
		commits = append(commits, CommitInfo{
			SHA:         c.SHA,
			Author:      author,
			Message:     firstLine(c.Commit.Message),
			CommittedAt: c.Commit.Author.Date,
		})
	}
	return commits, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i != -1 {
		return s[:i]
	}
	return s
}

func (c *githubClient) CheckCollaborator(owner, repo, username string) (bool, error) {
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(username))
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, classifyStatus(resp.StatusCode)
	}
}

// GetPendingInvite checks GitHub's list of not-yet-accepted repository
// invitations for owner/repo and reports whether username is among them.
func (c *githubClient) GetPendingInvite(owner, repo, username string) (bool, error) {
	path := fmt.Sprintf("/repos/%s/%s/invitations", url.PathEscape(owner), url.PathEscape(repo))
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, classifyStatus(resp.StatusCode)
	}

	var out []struct {
		Invitee struct {
			Login string `json:"login"`
		} `json:"invitee"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, fmt.Errorf("could not parse response: %w", err)
	}

	for _, inv := range out {
		if inv.Invitee.Login == username {
			return true, nil
		}
	}
	return false, nil
}
