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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", c.host, err)
	}
	return resp, nil
}

func (c *gitlabClient) GetAuthenticatedUser() (string, error) {
	resp, err := c.do(http.MethodGet, "/user", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", classifyStatus(resp.StatusCode)
	}

	var out struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("could not parse response: %w", err)
	}
	return out.Username, nil
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
		Name         string `json:"name"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		Visibility   string `json:"visibility"`
		Archived     bool   `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoInfo{}, fmt.Errorf("could not parse response: %w", err)
	}
	return RepoInfo{
		Name:     out.Name,
		CloneURL: out.HTTPURLToRepo,
		Private:  out.Visibility != "public",
		Archived: out.Archived,
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
