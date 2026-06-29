package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func (c *githubClient) GetAuthenticatedUser() (string, error) {
	resp, err := c.do(http.MethodGet, "/user", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", classifyStatus(resp.StatusCode)
	}

	var out struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("could not parse response: %w", err)
	}
	return out.Login, nil
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
		Name     string `json:"name"`
		CloneURL string `json:"clone_url"`
		Private  bool   `json:"private"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return RepoInfo{
		Name:     out.Name,
		CloneURL: out.CloneURL,
		Private:  out.Private,
		Archived: out.Archived,
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
