package access

import (
	"errors"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// mockClient is an in-memory api.Client for exercising enforce.go without
// any network access.
type mockClient struct {
	collaborators map[string]bool
	failCheck     bool
}

func newMockClient() *mockClient { return &mockClient{collaborators: map[string]bool{}} }

func key(owner, repo, username string) string { return owner + "/" + repo + "/" + username }

func (m *mockClient) GetRepo(owner, repo string) (api.RepoInfo, error) {
	return api.RepoInfo{Name: repo, CloneURL: "https://example.com/" + owner + "/" + repo + ".git"}, nil
}
func (m *mockClient) GetAuthenticatedUser() (string, error) { return "owner", nil }
func (m *mockClient) AddCollaborator(owner, repo, username, permission string) error {
	m.collaborators[key(owner, repo, username)] = true
	return nil
}
func (m *mockClient) RemoveCollaborator(owner, repo, username string) error {
	delete(m.collaborators, key(owner, repo, username))
	return nil
}
func (m *mockClient) CheckCollaborator(owner, repo, username string) (bool, error) {
	if m.failCheck {
		return false, errors.New("mock check failure")
	}
	return m.collaborators[key(owner, repo, username)], nil
}
func (m *mockClient) ListCommits(owner, repo, branch string, limit int) ([]api.CommitInfo, error) {
	return nil, nil
}
func (m *mockClient) GetPendingInvite(owner, repo, username string) (bool, error) {
	return false, nil
}
func (m *mockClient) Host() string { return "github.com" }

func newCol(t *testing.T, visibility collection.Visibility) *collection.Collection {
	t.Helper()
	col, err := collection.New("acme", "github.com", "owner", visibility)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	return col
}

// TestAccessDecisionMatrix covers every row of PROMPT.md's access control
// test matrix for CanAccessRepo (and the local half of CheckRepoAccess).
func TestAccessDecisionMatrix(t *testing.T) {
	cases := []struct {
		name       string
		visibility collection.Visibility
		owner      string
		members    []string
		groups     map[string][]string
		repo       collection.RepoAccess
		caller     string
		wantAccess bool
	}{
		{
			name:       "public collection + any caller",
			visibility: collection.VisibilityPublic,
			owner:      "owner",
			repo:       collection.RepoAccess{Name: "r"},
			caller:     "stranger",
			wantAccess: true,
		},
		{
			name:       "private + member + repo open",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice"},
			repo:       collection.RepoAccess{Name: "r"},
			caller:     "alice",
			wantAccess: true,
		},
		{
			name:       "private + member + repo groups=[G] + member in G",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice"},
			groups:     map[string][]string{"G": {"alice"}},
			repo:       collection.RepoAccess{Name: "r", Groups: []string{"G"}},
			caller:     "alice",
			wantAccess: true,
		},
		{
			name:       "private + member + repo users=[U] + caller == U",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice"},
			repo:       collection.RepoAccess{Name: "r", Users: []string{"alice"}},
			caller:     "alice",
			wantAccess: true,
		},
		{
			name:       "union: groups=[G] + member in G AND users=[U]",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice", "diana"},
			groups:     map[string][]string{"G": {"alice"}},
			repo:       collection.RepoAccess{Name: "r", Groups: []string{"G"}, Users: []string{"diana"}},
			caller:     "alice",
			wantAccess: true,
		},
		{
			name:       "union: groups=[G] users=[U] + caller not in G but caller == U",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice", "diana"},
			groups:     map[string][]string{"G": {"alice"}},
			repo:       collection.RepoAccess{Name: "r", Groups: []string{"G"}, Users: []string{"diana"}},
			caller:     "diana",
			wantAccess: true,
		},
		{
			name:       "groups=[G] + member NOT in G + NOT in users",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice", "charlie"},
			groups:     map[string][]string{"G": {"alice"}},
			repo:       collection.RepoAccess{Name: "r", Groups: []string{"G"}},
			caller:     "charlie",
			wantAccess: false,
		},
		{
			name:       "private + non-member",
			visibility: collection.VisibilityPrivate,
			owner:      "owner",
			members:    []string{"alice"},
			repo:       collection.RepoAccess{Name: "r"},
			caller:     "stranger",
			wantAccess: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := newCol(t, tc.visibility)
			col.Members = tc.members
			if tc.groups != nil {
				col.Groups = tc.groups
			}
			col.Repos = []collection.RepoAccess{tc.repo}

			if got := col.CanAccessRepo(tc.caller, tc.repo.Name); got != tc.wantAccess {
				t.Errorf("CanAccessRepo(%q, %q) = %v, want %v", tc.caller, tc.repo.Name, got, tc.wantAccess)
			}
		})
	}
}

func TestCheckCollectionAccess(t *testing.T) {
	col := newCol(t, collection.VisibilityPrivate)
	col.Members = []string{"alice"}

	if err := CheckCollectionAccess(col, "owner"); err != nil {
		t.Errorf("expected owner to always pass: %v", err)
	}
	if err := CheckCollectionAccess(col, "alice"); err != nil {
		t.Errorf("expected member to pass: %v", err)
	}
	if err := CheckCollectionAccess(col, "stranger"); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-member, got %v", err)
	}

	pub := newCol(t, collection.VisibilityPublic)
	if err := CheckCollectionAccess(pub, "anyone"); err != nil {
		t.Errorf("expected public collection to pass for anyone: %v", err)
	}
}

func TestCheckRepoAccess(t *testing.T) {
	col := newCol(t, collection.VisibilityPrivate)
	col.Members = []string{"alice", "bob"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []collection.RepoAccess{{Name: "r", Groups: []string{"red-team"}}}

	client := newMockClient()

	// alice: passes local rule but platform hasn't synced yet.
	if err := CheckRepoAccess(col, "r", "alice", client); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("expected ErrNoAccess before platform sync, got %v", err)
	}

	client.collaborators[key("owner", "r", "alice")] = true
	if err := CheckRepoAccess(col, "r", "alice", client); err != nil {
		t.Fatalf("expected access once platform is synced: %v", err)
	}

	// bob: not in red-team, fails the local rule regardless of platform state.
	if err := CheckRepoAccess(col, "r", "bob", client); !errors.Is(err, ErrGroupDenied) {
		t.Fatalf("expected ErrGroupDenied for bob, got %v", err)
	}

	// stranger: not even a member.
	if err := CheckRepoAccess(col, "r", "stranger", client); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-member, got %v", err)
	}
}

func TestFilterAccessible(t *testing.T) {
	col := newCol(t, collection.VisibilityPrivate)
	col.Members = []string{"alice", "bob"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []collection.RepoAccess{
		{Name: "open", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	client := newMockClient()
	client.collaborators[key("owner", "open", "alice")] = true
	client.collaborators[key("owner", "restricted", "alice")] = true

	got, err := FilterAccessible(col, "alice", client)
	if err != nil {
		t.Fatalf("FilterAccessible: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected alice to access both synced repos, got %v", got)
	}

	// bob is a member but only locally entitled to "open", and the platform
	// hasn't granted it yet, so he should see nothing.
	got, err = FilterAccessible(col, "bob", client)
	if err != nil {
		t.Fatalf("FilterAccessible: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected bob to access nothing, got %v", got)
	}

	if _, err := FilterAccessible(col, "stranger", client); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-member, got %v", err)
	}
}

func TestUserAccessMapAndRepoAccessMapAndFullMatrix(t *testing.T) {
	col := newCol(t, collection.VisibilityPrivate)
	col.Members = []string{"alice", "bob"}
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []collection.RepoAccess{
		{Name: "open", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	userMap := UserAccessMap(col, "bob")
	if len(userMap) != 2 {
		t.Fatalf("expected 2 repo entries, got %d", len(userMap))
	}
	for _, d := range userMap {
		if d.RepoName == "open" && !d.CanAccess {
			t.Error("expected bob to access the open repo")
		}
		if d.RepoName == "restricted" && d.CanAccess {
			t.Error("expected bob to be denied the restricted repo")
		}
	}

	repoMap := RepoAccessMap(col, "restricted")
	if len(repoMap) != 2 {
		t.Fatalf("expected 2 member entries, got %d", len(repoMap))
	}

	matrix := FullMatrix(col)
	if len(matrix.Members) != 2 || len(matrix.Repos) != 2 {
		t.Fatalf("unexpected matrix shape: %+v", matrix)
	}
	aliceIdx, restrictedIdx := -1, -1
	for i, m := range matrix.Members {
		if m == "alice" {
			aliceIdx = i
		}
	}
	for j, r := range matrix.Repos {
		if r == "restricted" {
			restrictedIdx = j
		}
	}
	if !matrix.Grid[aliceIdx][restrictedIdx] {
		t.Error("expected alice (red-team) to access the restricted repo in the matrix")
	}
}

// TestUserAccessMap_OwnerBypass is a regression test: col.CanAccessRepo's
// owner bypass (now baked directly into the function — see access.go) must
// make UserAccessMap report CanAccess=true for the owner on every repo
// even when the owner isn't separately listed as a member, paired with a
// reason that actually agrees with that true (not the old contradictory
// false/"owner" pairing this was originally written to catch).
func TestUserAccessMap_OwnerBypass(t *testing.T) {
	col := newCol(t, collection.VisibilityPrivate)
	// Deliberately do NOT add "owner" to col.Members.
	col.Groups = map[string][]string{"red-team": {"alice"}}
	col.Repos = []collection.RepoAccess{
		{Name: "open", Groups: []string{}, Users: []string{}},
		{Name: "restricted", Groups: []string{"red-team"}},
	}

	details := UserAccessMap(col, "owner")
	if len(details) != 2 {
		t.Fatalf("expected 2 repo entries, got %d", len(details))
	}
	for _, d := range details {
		if !d.CanAccess {
			t.Errorf("expected owner to access %q even though not a listed member, got CanAccess=false", d.RepoName)
		}
		if d.Reason != "owner — full access" {
			t.Errorf("expected owner's reason to be %q, got %q", "owner — full access", d.Reason)
		}
	}

	memberDetails := RepoAccessMap(col, "restricted")
	for _, d := range memberDetails {
		if d.Username == "owner" {
			t.Fatal("owner is not in col.Members, so RepoAccessMap should not produce an entry for them")
		}
	}
}
