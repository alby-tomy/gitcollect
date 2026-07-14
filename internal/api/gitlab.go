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

type gitlabClient struct {
	host       string
	token      string
	baseURL    string
	httpClient *http.Client
}

func newGitLabClient(host, token string) *gitlabClient {
	return &gitlabClient{
		host:       host,
		token:      token,
		baseURL:    "https://" + host + "/api/v4",
		httpClient: newHTTPClient(),
	}
}

func (c *gitlabClient) Host() string { return c.host }

// accessLevel maps gitcollect's generic permission names onto GitLab's
// numeric project access levels. "pull" maps to Reporter (20) rather than
// Guest (10) because Guest cannot read repository code on most GitLab
// versions, and gitcollect's "pull" always means read access to code.
func accessLevel(permission string) int {
	switch permission {
	case "admin":
		return 40 // Maintainer
	case "push":
		return 30 // Developer
	default:
		return 20 // Reporter ("pull")
	}
}

func (c *gitlabClient) do(method, path string, body any) (*http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("could not build request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.retryDo(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", c.host, err)
	}
	return resp, nil
}

func (c *gitlabClient) GetAuthenticatedUser() (UserInfo, error) {
	resp, err := c.do(http.MethodGet, "/user", nil)
	if err != nil {
		return UserInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, classifyStatus(resp.StatusCode)
	}

	var out struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return UserInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return UserInfo{ID: strconv.FormatInt(out.ID, 10), Login: out.Username}, nil
}

// GetUser resolves username to its GitLab account ID. Built on top of
// lookupUserID below — a different, pre-existing concern (GitLab's
// project-member endpoints require a numeric ID in the request, not a
// username, so AddCollaborator/RemoveCollaborator/CheckCollaborator have
// always needed to resolve one internally) that happens to call the same
// GET /users?username= endpoint GetUser needs. GitLab's exact-match query
// means the input username is already confirmed correct on success, so
// there's no need to re-decode it from the response.
func (c *gitlabClient) GetUser(username string) (UserInfo, error) {
	id, err := c.lookupUserID(username)
	if err != nil {
		if err == ErrNotFound {
			return UserInfo{}, fmt.Errorf("%w: %s", ErrUserNotFound, username)
		}
		return UserInfo{}, err
	}
	return UserInfo{ID: strconv.Itoa(id), Login: username}, nil
}

func (c *gitlabClient) GetRepo(owner, repo string) (RepoInfo, error) {
	id := url.QueryEscape(owner + "/" + repo)
	resp, err := c.do(http.MethodGet, "/projects/"+id, nil)
	if err != nil {
		return RepoInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RepoInfo{}, classifyStatus(resp.StatusCode)
	}

	var out struct {
		Name          string `json:"name"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		DefaultBranch string `json:"default_branch"`
		Visibility    string `json:"visibility"`
		Archived      bool   `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return RepoInfo{
		Name:          out.Name,
		CloneURL:      out.HTTPURLToRepo,
		DefaultBranch: out.DefaultBranch,
		Private:       out.Visibility != "public",
		Archived:      out.Archived,
	}, nil
}

// lookupUserID resolves a username to GitLab's internal numeric user ID,
// required by the project-members endpoints.
func (c *gitlabClient) lookupUserID(username string) (int, error) {
	resp, err := c.do(http.MethodGet, "/users?username="+url.QueryEscape(username), nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, classifyStatus(resp.StatusCode)
	}

	var out []struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("could not parse response: %w", err)
	}
	if len(out) == 0 {
		return 0, ErrNotFound
	}
	return out[0].ID, nil
}

func (c *gitlabClient) AddCollaborator(owner, repo, username, permission string) error {
	userID, err := c.lookupUserID(username)
	if err != nil {
		return err
	}

	id := url.QueryEscape(owner + "/" + repo)
	body := map[string]int{"user_id": userID, "access_level": accessLevel(permission)}

	resp, err := c.do(http.MethodPost, "/projects/"+id+"/members", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusConflict:
		// Already a member: update their access level instead.
		return c.updateCollaborator(owner, repo, userID, permission)
	default:
		return classifyStatus(resp.StatusCode)
	}
}

func (c *gitlabClient) updateCollaborator(owner, repo string, userID int, permission string) error {
	id := url.QueryEscape(owner + "/" + repo)
	body := map[string]int{"access_level": accessLevel(permission)}

	resp, err := c.do(http.MethodPut, fmt.Sprintf("/projects/%s/members/%d", id, userID), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return classifyStatus(resp.StatusCode)
	}
	return nil
}

func (c *gitlabClient) RemoveCollaborator(owner, repo, username string) error {
	userID, err := c.lookupUserID(username)
	if err != nil {
		if err == ErrNotFound {
			return nil // no such user, nothing to remove
		}
		return err
	}

	id := url.QueryEscape(owner + "/" + repo)
	resp, err := c.do(http.MethodDelete, fmt.Sprintf("/projects/%s/members/%d", id, userID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	default:
		return classifyStatus(resp.StatusCode)
	}
}

// ListCommits returns the most recent commits on branch, newest first.
// Unlike GitHub, GitLab's commits endpoint does not expose the pushing
// user's platform username — only the raw git author_name/author_email
// recorded in the commit itself — so Author is always the commit author
// name here, never a resolved GitLab account.
func (c *gitlabClient) ListCommits(owner, repo, branch string, limit int) ([]CommitInfo, error) {
	id := url.QueryEscape(owner + "/" + repo)
	path := fmt.Sprintf("/projects/%s/repository/commits?ref_name=%s&per_page=%s",
		id, url.QueryEscape(branch), strconv.Itoa(limit))
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyStatus(resp.StatusCode)
	}

	var out []struct {
		ID            string    `json:"id"`
		Title         string    `json:"title"`
		AuthorName    string    `json:"author_name"`
		CommittedDate time.Time `json:"committed_date"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}

	commits := make([]CommitInfo, 0, len(out))
	for _, c := range out {
		commits = append(commits, CommitInfo{
			SHA:         c.ID,
			Author:      c.AuthorName,
			Message:     c.Title,
			CommittedAt: c.CommittedDate,
		})
	}
	return commits, nil
}

// CreateRepo creates a new GitLab project under namespace_path (owner).
// GitLab uses a single POST /projects endpoint for both personal and group
// namespaces — namespace_path distinguishes them.
func (c *gitlabClient) CreateRepo(owner, name string, private bool, description string) (RepoInfo, error) {
	visibility := "private"
	if !private {
		visibility = "public"
	}

	body := map[string]any{
		"name":                   name,
		"path":                   name,
		"namespace_path":         owner,
		"visibility":             visibility,
		"description":            description,
		"initialize_with_readme": false,
	}

	resp, err := c.do(http.MethodPost, "/projects", body)
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
		Name          string `json:"name"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		Visibility    string `json:"visibility"`
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

func (c *gitlabClient) CheckCollaborator(owner, repo, username string) (bool, error) {
	userID, err := c.lookupUserID(username)
	if err != nil {
		if err == ErrNotFound {
			return false, nil
		}
		return false, err
	}

	id := url.QueryEscape(owner + "/" + repo)
	resp, err := c.do(http.MethodGet, fmt.Sprintf("/projects/%s/members/%d", id, userID), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, classifyStatus(resp.StatusCode)
	}
}

// GetPendingInvite always returns false: GitLab project membership added
// via the API takes effect immediately, with no GitHub-style "pending,
// must be accepted" state to detect.
func (c *gitlabClient) GetPendingInvite(owner, repo, username string) (bool, error) {
	return false, nil
}

// retryDo executes r through the package-level doWithRetry helper
// (defined in github.go — same package).
func (c *gitlabClient) retryDo(r *http.Request) (*http.Response, error) {
	return doWithRetry(r, c.httpClient, c.host)
}

// paginateGitLab calls startURL repeatedly following Link: rel="next" headers
// (GitLab REST API uses the same Link header format as GitHub). Falls back to
// the X-Next-Page header when the Link header is absent.
func (c *gitlabClient) paginateGitLab(startURL string, fn func([]byte) error) error {
	pageURL := startURL
	for pageURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("PRIVATE-TOKEN", c.token)

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
		// nextPageURL is defined in github.go (same package) and parses the
		// Link header format that GitLab also produces.
		if next := nextPageURL(resp.Header.Get("Link")); next != "" {
			pageURL = next
		} else if pg := resp.Header.Get("X-Next-Page"); pg != "" {
			u, parseErr := url.Parse(startURL)
			if parseErr != nil {
				break
			}
			q := u.Query()
			q.Set("page", pg)
			u.RawQuery = q.Encode()
			pageURL = u.String()
		} else {
			pageURL = ""
		}
	}
	return nil
}

// ListOpenPRs returns open merge requests for a GitLab project (owner/repo).
func (c *gitlabClient) ListOpenPRs(owner, repo string) ([]PRInfo, error) {
	id := url.PathEscape(owner + "/" + repo)
	startURL := fmt.Sprintf("%s/projects/%s/merge_requests?state=opened&per_page=100", c.baseURL, id)
	var prs []PRInfo
	err := c.paginateGitLab(startURL, func(body []byte) error {
		var page []struct {
			IID       int       `json:"iid"`
			Title     string    `json:"title"`
			State     string    `json:"state"`
			WebURL    string    `json:"web_url"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Author    struct {
				Username string `json:"username"`
			} `json:"author"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, p := range page {
			prs = append(prs, PRInfo{
				Number:    p.IID,
				Title:     p.Title,
				Author:    p.Author.Username,
				State:     p.State,
				URL:       p.WebURL,
				Repo:      repo,
				CreatedAt: p.CreatedAt,
				UpdatedAt: p.UpdatedAt,
			})
		}
		return nil
	})
	return prs, err
}

// ListOrgRepos returns all projects in a GitLab group, handling pagination.
func (c *gitlabClient) ListOrgRepos(org string) ([]RepoInfo, error) {
	startURL := fmt.Sprintf("%s/groups/%s/projects?per_page=100&include_subgroups=false",
		c.baseURL, url.PathEscape(org))
	var repos []RepoInfo
	err := c.paginateGitLab(startURL, func(body []byte) error {
		var page []struct {
			Name          string `json:"name"`
			HTTPURLToRepo string `json:"http_url_to_repo"`
			DefaultBranch string `json:"default_branch"`
			Visibility    string `json:"visibility"`
			Archived      bool   `json:"archived"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, r := range page {
			repos = append(repos, RepoInfo{
				Name:          r.Name,
				CloneURL:      r.HTTPURLToRepo,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Visibility != "public",
				Archived:      r.Archived,
			})
		}
		return nil
	})
	return repos, err
}

// ListOrgTeams returns the direct subgroups of the given GitLab group,
// mapped onto TeamInfo. The group slug is used as the Slug field.
func (c *gitlabClient) ListOrgTeams(org string) ([]TeamInfo, error) {
	startURL := fmt.Sprintf("%s/groups/%s/subgroups?per_page=100",
		c.baseURL, url.PathEscape(org))
	var teams []TeamInfo
	err := c.paginateGitLab(startURL, func(body []byte) error {
		var page []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Path        string `json:"path"`
			Description string `json:"description"`
			Visibility  string `json:"visibility"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, g := range page {
			privacy := "private"
			if g.Visibility == "public" {
				privacy = "public"
			}
			teams = append(teams, TeamInfo{
				ID:          g.ID,
				Name:        g.Name,
				Slug:        g.Path,
				Description: g.Description,
				Privacy:     privacy,
			})
		}
		return nil
	})
	return teams, err
}

// ListTeamMembers returns members of a GitLab subgroup (org/teamSlug).
// The role parameter is accepted but not currently filtered server-side;
// GitLab's members endpoint returns all access levels and the caller
// can filter on the returned data if needed.
func (c *gitlabClient) ListTeamMembers(org, teamSlug, role string) ([]UserInfo, error) {
	groupPath := url.QueryEscape(org + "/" + teamSlug)
	startURL := fmt.Sprintf("%s/groups/%s/members?per_page=100", c.baseURL, groupPath)
	var members []UserInfo
	err := c.paginateGitLab(startURL, func(body []byte) error {
		var page []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, m := range page {
			members = append(members, UserInfo{
				ID:    strconv.FormatInt(m.ID, 10),
				Login: m.Username,
			})
		}
		return nil
	})
	return members, err
}

// ListTeamRepos returns the projects accessible to a GitLab subgroup (org/teamSlug).
func (c *gitlabClient) ListTeamRepos(org, teamSlug string) ([]RepoInfo, error) {
	groupPath := url.QueryEscape(org + "/" + teamSlug)
	startURL := fmt.Sprintf("%s/groups/%s/projects?per_page=100", c.baseURL, groupPath)
	var repos []RepoInfo
	err := c.paginateGitLab(startURL, func(body []byte) error {
		var page []struct {
			Name          string `json:"name"`
			HTTPURLToRepo string `json:"http_url_to_repo"`
			Visibility    string `json:"visibility"`
			Archived      bool   `json:"archived"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return err
		}
		for _, r := range page {
			repos = append(repos, RepoInfo{
				Name:     r.Name,
				CloneURL: r.HTTPURLToRepo,
				Private:  r.Visibility != "public",
				Archived: r.Archived,
			})
		}
		return nil
	})
	return repos, err
}

// GetTokenScopes returns the OAuth scopes for the current GitLab token.
// Tries the personal access token info endpoint first (GitLab 15.0+),
// then falls back to the OAuth token info endpoint.
func (c *gitlabClient) GetTokenScopes() ([]string, error) {
	resp, err := c.do(http.MethodGet, "/personal_access_tokens/self", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var out struct {
			Scopes []string `json:"scopes"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return []string{}, nil
		}
		return out.Scopes, nil
	}

	// Fall back to OAuth token info (space-separated scope string).
	resp2, err := c.do(http.MethodGet, "/oauth/token/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return []string{}, nil
	}
	var out2 struct {
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&out2); err != nil {
		return []string{}, nil
	}
	var scopes []string
	for _, s := range strings.Split(out2.Scope, " ") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes, nil
}

func (c *gitlabClient) SearchRepos(org, pattern, topic string, limit int) ([]RepoInfo, error) {
	return nil, fmt.Errorf("search is not supported for GitLab")
}
