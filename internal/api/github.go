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

// CreateRepo creates a new repository under owner. If owner matches the
// authenticated user's login, the personal repo endpoint is used
// (POST /user/repos); otherwise the org endpoint is used
// (POST /orgs/{owner}/repos). The "owner == me" check calls
// GetAuthenticatedUser, whose result is memoized at the cmd layer so this
// is at most one additional network call per invocation, and only when
// the repo does not already exist.
func (c *githubClient) CreateRepo(owner, name string, private bool, description string) (RepoInfo, error) {
	me, err := c.GetAuthenticatedUser()
	if err != nil {
		return RepoInfo{}, fmt.Errorf("create repo: resolve authenticated user: %w", err)
	}

	endpoint := "/user/repos"
	if owner != me.Login {
		endpoint = fmt.Sprintf("/orgs/%s/repos", owner)
	}

	body := map[string]any{
		"name":        name,
		"private":     private,
		"description": description,
		"auto_init":   false,
	}

	resp, err := c.do(http.MethodPost, endpoint, body)
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

// retryDo is a thin wrapper around c.httpClient.Do. The context timeout is
// already on the request; no retry logic exists in this codebase.
func (c *githubClient) retryDo(r *http.Request) (*http.Response, error) {
	return c.httpClient.Do(r)
}

// paginate calls startURL repeatedly, following Link: rel="next" headers,
// until all pages are fetched. Calls fn with each page's raw response body.
func (c *githubClient) paginate(startURL string, fn func([]byte) error) error {
	pageURL := startURL
	for pageURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := c.retryDo(req)
		if err != nil {
			cancel()
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return classifyStatus(resp.StatusCode)
		}
		if err := fn(body); err != nil {
			return err
		}
		pageURL = nextPageURL(resp.Header.Get("Link"))
	}
	return nil
}

// nextPageURL parses GitHub's Link header and returns the "next" URL.
// Returns "" when there is no next page.
// Link header format:
//
//	<https://api.github.com/...?page=2>; rel="next", <...>; rel="last"
func nextPageURL(link string) string {
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			parts := strings.SplitN(part, ";", 2)
			if len(parts) > 0 {
				return strings.Trim(strings.TrimSpace(parts[0]), "<>")
			}
		}
	}
	return ""
}

func (c *githubClient) ListOrgTeams(org string) ([]TeamInfo, error) {
	startURL := fmt.Sprintf("%s/orgs/%s/teams?per_page=100", githubBaseURL, url.PathEscape(org))
	var teams []TeamInfo
	err := c.paginate(startURL, func(body []byte) error {
		var page []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
			Privacy     string `json:"privacy"`
			Parent      *struct {
				Slug string `json:"slug"`
			} `json:"parent"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, t := range page {
			ti := TeamInfo{
				ID:          t.ID,
				Name:        t.Name,
				Slug:        t.Slug,
				Description: t.Description,
				Privacy:     t.Privacy,
			}
			if t.Parent != nil {
				ti.ParentSlug = t.Parent.Slug
			}
			teams = append(teams, ti)
		}
		return nil
	})
	return teams, err
}

func (c *githubClient) ListTeamMembers(org, teamSlug, role string) ([]UserInfo, error) {
	startURL := fmt.Sprintf("%s/orgs/%s/teams/%s/members?per_page=100",
		githubBaseURL, url.PathEscape(org), url.PathEscape(teamSlug))
	if role != "" {
		startURL += "&role=" + url.QueryEscape(role)
	}
	var members []UserInfo
	err := c.paginate(startURL, func(body []byte) error {
		var page []struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, m := range page {
			members = append(members, UserInfo{
				ID:    strconv.FormatInt(m.ID, 10),
				Login: m.Login,
			})
		}
		return nil
	})
	return members, err
}

func (c *githubClient) ListTeamRepos(org, teamSlug string) ([]RepoInfo, error) {
	startURL := fmt.Sprintf("%s/orgs/%s/teams/%s/repos?per_page=100",
		githubBaseURL, url.PathEscape(org), url.PathEscape(teamSlug))
	var repos []RepoInfo
	err := c.paginate(startURL, func(body []byte) error {
		var page []struct {
			Name     string `json:"name"`
			CloneURL string `json:"clone_url"`
			Private  bool   `json:"private"`
			Archived bool   `json:"archived"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, r := range page {
			repos = append(repos, RepoInfo{
				Name:     r.Name,
				CloneURL: r.CloneURL,
				Private:  r.Private,
				Archived: r.Archived,
			})
		}
		return nil
	})
	return repos, err
}

func (c *githubClient) GetTokenScopes() ([]string, error) {
	resp, err := c.do(http.MethodGet, "/user", nil)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	scopeHeader := resp.Header.Get("X-OAuth-Scopes")
	if scopeHeader == "" {
		return []string{}, nil
	}
	var scopes []string
	for _, s := range strings.Split(scopeHeader, ",") {
		scopes = append(scopes, strings.TrimSpace(s))
	}
	return scopes, nil
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
