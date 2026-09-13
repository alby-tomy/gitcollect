package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
)

// prMock serves a canned PR list per repo and can fail for named repos, so
// tests can cover the mixed success/failure case fetchOpenPRs has to
// survive.
type prMock struct {
	multiAddMock
	prs     map[string][]api.PRInfo
	failFor map[string]error
}

func (m *prMock) ListOpenPRs(_, repo string) ([]api.PRInfo, error) {
	if err, ok := m.failFor[repo]; ok {
		return nil, err
	}
	return m.prs[repo], nil
}

func newPRMock(prs map[string][]api.PRInfo) *prMock {
	return &prMock{multiAddMock: *newMultiAddMock(), prs: prs, failFor: map[string]error{}}
}

func day(n int) time.Time {
	return time.Date(2026, 3, n, 12, 0, 0, 0, time.UTC)
}

func repoList(names ...string) []collection.RepoAccess {
	repos := make([]collection.RepoAccess, 0, len(names))
	for _, n := range names {
		repos = append(repos, collection.RepoAccess{Name: n})
	}
	return repos
}

func TestFetchOpenPRs_AggregatesAcrossRepos(t *testing.T) {
	mock := newPRMock(map[string][]api.PRInfo{
		"api": {{Number: 1, Title: "fix auth", Author: "alice", UpdatedAt: day(3)}},
		"web": {
			{Number: 7, Title: "bump deps", Author: "bob", UpdatedAt: day(5)},
			{Number: 8, Title: "typo", Author: "alice", UpdatedAt: day(1)},
		},
	})

	got, failed := fetchOpenPRs(mock, "acme", repoList("api", "web"))
	if len(failed) != 0 {
		t.Errorf("expected no failures, got %v", failed)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 PRs across both repos, got %d", len(got))
	}
}

// Each PR must carry the repo it came from — the table's REPO column and
// the JSON output both read it, and it is set by the fetcher, not the API.
func TestFetchOpenPRs_TagsEachPRWithItsRepo(t *testing.T) {
	mock := newPRMock(map[string][]api.PRInfo{
		"api": {{Number: 1, Title: "a"}},
		"web": {{Number: 2, Title: "b"}},
	})

	byNumber := map[int]string{}
	all, _ := fetchOpenPRs(mock, "acme", repoList("api", "web"))
	for _, p := range all {
		byNumber[p.Number] = p.Repo
	}
	if byNumber[1] != "api" || byNumber[2] != "web" {
		t.Errorf("PRs not tagged with their repo: %v", byNumber)
	}
}

func TestFetchOpenPRs_EmptyWhenNoRepos(t *testing.T) {
	mock := newPRMock(nil)
	if got, _ := fetchOpenPRs(mock, "acme", nil); len(got) != 0 {
		t.Errorf("expected no PRs for an empty repo list, got %d", len(got))
	}
}

// A repo whose API call fails must not take the whole listing down: the
// PRs that were fetched successfully still come back.
func TestFetchOpenPRs_SurvivesPerRepoFailure(t *testing.T) {
	mock := newPRMock(map[string][]api.PRInfo{
		"api": {{Number: 1, Title: "fix auth", Author: "alice"}},
	})
	mock.failFor["web"] = errors.New("403 forbidden")

	got, failed := fetchOpenPRs(mock, "acme", repoList("api", "web"))
	if len(got) != 1 {
		t.Fatalf("expected the one reachable repo's PRs, got %d", len(got))
	}
	if got[0].Repo != "api" {
		t.Errorf("expected the surviving PR to be from api, got %q", got[0].Repo)
	}
	// The failure must be reported, not silently swallowed: an unreachable
	// repo and a repo with no PRs are different answers.
	if len(failed) != 1 || failed[0] != "web" {
		t.Errorf("expected web reported as unreadable, got %v", failed)
	}
}

func TestTruncatePRTitle(t *testing.T) {
	short := "a short title"
	if got := truncatePRTitle(short); got != short {
		t.Errorf("short titles must pass through unchanged, got %q", got)
	}

	long := strings.Repeat("x", 80)
	got := truncatePRTitle(long)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected an ellipsis on a long title, got %q", got)
	}
	if n := len([]rune(got)); n != 53 { // 50 runes + "..."
		t.Errorf("expected 53 runes, got %d", n)
	}
}

// Titles are counted in runes, not bytes, so a multi-byte title is not cut
// mid-character.
func TestTruncatePRTitle_CountsRunesNotBytes(t *testing.T) {
	title := strings.Repeat("é", 40) // 40 runes, 80 bytes
	if got := truncatePRTitle(title); got != title {
		t.Errorf("a 40-rune title must not be truncated, got %q", got)
	}
}
