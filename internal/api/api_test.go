package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withGitHubServer points githubBaseURL at server for the duration of the
// test and restores it afterward.
func withGitHubServer(t *testing.T, handler http.HandlerFunc) *githubClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	original := githubBaseURL
	githubBaseURL = server.URL
	t.Cleanup(func() { githubBaseURL = original })

	return newGitHubClient("github.com", "test-token")
}

func newTestGitLabClient(t *testing.T, handler http.HandlerFunc) *gitlabClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &gitlabClient{host: "gitlab.com", token: "test-token", baseURL: server.URL, httpClient: newHTTPClient()}
}

func TestNewClientDispatch(t *testing.T) {
	if _, ok := NewClient("github.com", "t").(*githubClient); !ok {
		t.Error("expected NewClient(\"github.com\", ...) to return a *githubClient")
	}
	if _, ok := NewClient("gitlab.com", "t").(*gitlabClient); !ok {
		t.Error("expected NewClient(\"gitlab.com\", ...) to return a *gitlabClient")
	}
	if _, ok := NewClient("git.internal.example.com", "t").(*gitlabClient); !ok {
		t.Error("expected a self-hosted host to dispatch to *gitlabClient")
	}
}

func TestGitHubGetAuthenticatedUser(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]string{"login": "alice"})
	})

	user, err := client.GetAuthenticatedUser()
	if err != nil {
		t.Fatalf("GetAuthenticatedUser: %v", err)
	}
	if user != "alice" {
		t.Errorf("got %q, want %q", user, "alice")
	}
}

func TestGitHubGetAuthenticatedUser_Unauthorized(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := client.GetAuthenticatedUser(); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestGitHubGetRepo(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"name":      "widgets",
			"clone_url": "https://github.com/acme/widgets.git",
			"private":   true,
			"archived":  false,
		})
	})

	info, err := client.GetRepo("acme", "widgets")
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if info.Name != "widgets" || info.CloneURL != "https://github.com/acme/widgets.git" || !info.Private || info.Archived {
		t.Errorf("unexpected RepoInfo: %+v", info)
	}
}

func TestGitHubGetRepo_NotFound(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := client.GetRepo("acme", "ghost"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGitHubAddCollaborator(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/repos/acme/widgets/collaborators/bob" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["permission"] != "pull" {
			t.Errorf("expected permission=pull in body, got %v", body)
		}
		w.WriteHeader(http.StatusCreated)
	})

	if err := client.AddCollaborator("acme", "widgets", "bob", "pull"); err != nil {
		t.Fatalf("AddCollaborator: %v", err)
	}
}

func TestGitHubRemoveCollaborator(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/repos/acme/widgets/collaborators/bob" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.RemoveCollaborator("acme", "widgets", "bob"); err != nil {
		t.Fatalf("RemoveCollaborator: %v", err)
	}
}

func TestGitHubCheckCollaborator(t *testing.T) {
	cases := map[int]bool{http.StatusNoContent: true, http.StatusNotFound: false}
	for status, want := range cases {
		client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
		got, err := client.CheckCollaborator("acme", "widgets", "bob")
		if err != nil {
			t.Fatalf("CheckCollaborator (status=%d): %v", status, err)
		}
		if got != want {
			t.Errorf("CheckCollaborator (status=%d) = %v, want %v", status, got, want)
		}
	}
}

func TestGitLabGetAuthenticatedUser(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "test-token" {
			t.Errorf("PRIVATE-TOKEN header = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})

	user, err := client.GetAuthenticatedUser()
	if err != nil {
		t.Fatalf("GetAuthenticatedUser: %v", err)
	}
	if user != "alice" {
		t.Errorf("got %q, want %q", user, "alice")
	}
}

func TestGitLabGetRepo(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name":             "widgets",
			"http_url_to_repo": "https://gitlab.com/acme/widgets.git",
			"visibility":       "private",
			"archived":         false,
		})
	})

	info, err := client.GetRepo("acme", "widgets")
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if info.Name != "widgets" || !info.Private {
		t.Errorf("unexpected RepoInfo: %+v", info)
	}
}

func TestGitLabAddCollaborator_FallsBackToUpdateOnConflict(t *testing.T) {
	var lookupCalls, addCalls, updateCalls int
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users":
			lookupCalls++
			json.NewEncoder(w).Encode([]map[string]int{{"id": 42}})
		case r.Method == http.MethodPost:
			addCalls++
			w.WriteHeader(http.StatusConflict) // already a member
		case r.Method == http.MethodPut:
			updateCalls++
			var body map[string]int
			json.NewDecoder(r.Body).Decode(&body)
			if body["access_level"] != 20 {
				t.Errorf("expected access_level 20 (pull→Reporter), got %v", body)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := client.AddCollaborator("acme", "widgets", "bob", "pull"); err != nil {
		t.Fatalf("AddCollaborator: %v", err)
	}
	if lookupCalls != 1 || addCalls != 1 || updateCalls != 1 {
		t.Errorf("expected exactly one lookup, one failed add, one update; got %d/%d/%d", lookupCalls, addCalls, updateCalls)
	}
}

func TestGitLabCheckCollaborator(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode([]map[string]int{{"id": 42}})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	has, err := client.CheckCollaborator("acme", "widgets", "bob")
	if err != nil {
		t.Fatalf("CheckCollaborator: %v", err)
	}
	if !has {
		t.Error("expected CheckCollaborator to report true")
	}
}

func TestGitLabCheckCollaborator_UnknownUser(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]int{}) // lookupUserID finds nobody
	})

	has, err := client.CheckCollaborator("acme", "widgets", "ghost")
	if err != nil {
		t.Fatalf("CheckCollaborator: %v", err)
	}
	if has {
		t.Error("expected CheckCollaborator to report false for an unknown user")
	}
}

func TestHost(t *testing.T) {
	gh := newGitHubClient("github.com", "t")
	if gh.Host() != "github.com" {
		t.Errorf("githubClient.Host() = %q", gh.Host())
	}
	gl := &gitlabClient{host: "gitlab.example.com"}
	if gl.Host() != "gitlab.example.com" {
		t.Errorf("gitlabClient.Host() = %q", gl.Host())
	}
}

func TestAccessLevel(t *testing.T) {
	cases := map[string]int{"pull": 20, "push": 30, "admin": 40, "unknown": 20}
	for permission, want := range cases {
		if got := accessLevel(permission); got != want {
			t.Errorf("accessLevel(%q) = %d, want %d", permission, got, want)
		}
	}
}

func TestGitLabRemoveCollaborator(t *testing.T) {
	var sawDelete bool
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode([]map[string]int{{"id": 42}})
		case r.Method == http.MethodDelete:
			sawDelete = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := client.RemoveCollaborator("acme", "widgets", "bob"); err != nil {
		t.Fatalf("RemoveCollaborator: %v", err)
	}
	if !sawDelete {
		t.Error("expected a DELETE request to the project member endpoint")
	}
}

func TestGitLabRemoveCollaborator_UnknownUserIsNoop(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]int{}) // lookupUserID finds nobody
	})

	if err := client.RemoveCollaborator("acme", "widgets", "ghost"); err != nil {
		t.Fatalf("expected RemoveCollaborator to be a no-op for an unknown user: %v", err)
	}
}

func TestGitLabGetAuthenticatedUser_Error(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := client.GetAuthenticatedUser(); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGitLabGetRepo_Error(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := client.GetRepo("acme", "ghost"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGitLabAddCollaborator_LookupFails(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if err := client.AddCollaborator("acme", "widgets", "bob", "pull"); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestGitLabAddCollaborator_UpdateFails(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode([]map[string]int{{"id": 42}})
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusConflict)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	if err := client.AddCollaborator("acme", "widgets", "bob", "pull"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden from the fallback update, got %v", err)
	}
}

func TestGitLabCheckCollaborator_Error(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode([]map[string]int{{"id": 42}})
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	if _, err := client.CheckCollaborator("acme", "widgets", "bob"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGitLabRemoveCollaborator_Error(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode([]map[string]int{{"id": 42}})
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	if err := client.RemoveCollaborator("acme", "widgets", "bob"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGitHubAddCollaborator_Error(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if err := client.AddCollaborator("acme", "widgets", "bob", "pull"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGitHubRemoveCollaborator_Error(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if err := client.RemoveCollaborator("acme", "widgets", "bob"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGitHubCheckCollaborator_Error(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := client.CheckCollaborator("acme", "widgets", "bob"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGitHubGetRepo_DefaultBranch(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name":           "widgets",
			"clone_url":      "https://github.com/acme/widgets.git",
			"default_branch": "main",
		})
	})

	info, err := client.GetRepo("acme", "widgets")
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if info.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want %q", info.DefaultBranch, "main")
	}
}

func TestGitHubListCommits(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/commits" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("sha"); got != "main" {
			t.Errorf("expected sha=main, got %q", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "5" {
			t.Errorf("expected per_page=5, got %q", got)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"sha":    "abc123",
				"author": map[string]string{"login": "alice"},
				"commit": map[string]any{
					"message": "Fix bug\n\nLonger description here.",
					"author":  map[string]any{"name": "Alice Raw", "date": "2026-01-15T10:00:00Z"},
				},
			},
			{
				"sha":    "def456",
				"author": nil,
				"commit": map[string]any{
					"message": "Unlinked commit",
					"author":  map[string]any{"name": "Bob Raw", "date": "2026-01-14T09:00:00Z"},
				},
			},
		})
	})

	commits, err := client.ListCommits("acme", "widgets", "main", 5)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].Author != "alice" {
		t.Errorf("expected first commit's author to resolve to the linked login %q, got %q", "alice", commits[0].Author)
	}
	if commits[0].Message != "Fix bug" {
		t.Errorf("expected message to be truncated to its first line, got %q", commits[0].Message)
	}
	if commits[1].Author != "Bob Raw" {
		t.Errorf("expected unlinked commit to fall back to the raw author name, got %q", commits[1].Author)
	}
	wantDate := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	if !commits[0].CommittedAt.Equal(wantDate) {
		t.Errorf("CommittedAt = %v, want %v", commits[0].CommittedAt, wantDate)
	}
}

func TestGitHubListCommits_Error(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := client.ListCommits("acme", "ghost", "main", 5); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGitHubGetPendingInvite(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/invitations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"invitee": map[string]string{"login": "bob"}},
		})
	})

	has, err := client.GetPendingInvite("acme", "widgets", "bob")
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if !has {
		t.Error("expected bob to have a pending invite")
	}

	has, err = client.GetPendingInvite("acme", "widgets", "carol")
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if has {
		t.Error("expected carol (not in the invitations list) to have no pending invite")
	}
}

func TestGitHubGetPendingInvite_Error(t *testing.T) {
	client := withGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := client.GetPendingInvite("acme", "ghost", "bob"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGitLabGetPendingInvite_AlwaysFalse(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("GetPendingInvite should never make a request on GitLab")
	})
	has, err := client.GetPendingInvite("acme", "widgets", "bob")
	if err != nil || has {
		t.Fatalf("expected (false, nil), got (%v, %v)", has, err)
	}
}

func TestGitLabGetRepo_DefaultBranch(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name":           "widgets",
			"default_branch": "master",
		})
	})

	info, err := client.GetRepo("acme", "widgets")
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if info.DefaultBranch != "master" {
		t.Errorf("DefaultBranch = %q, want %q", info.DefaultBranch, "master")
	}
}

func TestGitLabListCommits(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ref_name"); got != "master" {
			t.Errorf("expected ref_name=master, got %q", got)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":             "abc123",
				"title":          "Fix bug",
				"author_name":    "Alice Raw",
				"committed_date": "2026-01-15T10:00:00Z",
			},
		})
	})

	commits, err := client.ListCommits("acme", "widgets", "master", 5)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].Author != "Alice Raw" {
		t.Errorf("expected GitLab's Author to always be the raw author_name, got %q", commits[0].Author)
	}
	if commits[0].SHA != "abc123" || commits[0].Message != "Fix bug" {
		t.Errorf("unexpected commit: %+v", commits[0])
	}
}

func TestGitLabListCommits_Error(t *testing.T) {
	client := newTestGitLabClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := client.ListCommits("acme", "widgets", "master", 5); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestClassifyStatus(t *testing.T) {
	cases := map[int]error{
		http.StatusUnauthorized:    ErrUnauthorized,
		http.StatusForbidden:       ErrForbidden,
		http.StatusNotFound:        ErrNotFound,
		http.StatusTooManyRequests: ErrRateLimit,
	}
	for status, want := range cases {
		if got := classifyStatus(status); got != want {
			t.Errorf("classifyStatus(%d) = %v, want %v", status, got, want)
		}
	}
	if err := classifyStatus(http.StatusTeapot); err == nil {
		t.Error("expected an error for an unrecognised status code")
	}
}
