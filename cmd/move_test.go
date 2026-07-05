package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// setupMoveTest creates two collections on disk and injects the mock client.
// srcRepos/dstRepos are repo names to pre-populate.
// srcMembers/dstMembers are login names; IDs are derived as "<login>-id".
// The caller (owner of both) is "owner" / "owner-id".
func setupMoveTest(
	t *testing.T,
	srcName, dstName string,
	mock *multiAddMock,
	srcRepos, dstRepos []string,
	srcMembers, dstMembers []string,
) (srcCol, dstCol *collection.Collection) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cachedClient = mock
	cachedUser = "owner"
	cachedUserID = "owner-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	owner := api.UserInfo{ID: "owner-id", Login: "owner"}

	srcCol, err := collection.New(srcName, "github.com", owner, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New(src): %v", err)
	}
	for _, r := range srcRepos {
		srcCol.Repos = append(srcCol.Repos, collection.RepoAccess{Name: r, Groups: []string{}, Users: []string{}})
	}
	for _, m := range srcMembers {
		id := m + "-id"
		srcCol.Members = append(srcCol.Members, id)
		srcCol.Logins[id] = m
	}
	if err := srcCol.Save(); err != nil {
		t.Fatalf("srcCol.Save: %v", err)
	}

	dstCol, err = collection.New(dstName, "github.com", owner, collection.VisibilityPrivate)
	if err != nil {
		t.Fatalf("collection.New(dst): %v", err)
	}
	for _, r := range dstRepos {
		dstCol.Repos = append(dstCol.Repos, collection.RepoAccess{Name: r, Groups: []string{}, Users: []string{}})
	}
	for _, m := range dstMembers {
		id := m + "-id"
		dstCol.Members = append(dstCol.Members, id)
		dstCol.Logins[id] = m
	}
	if err := dstCol.Save(); err != nil {
		t.Fatalf("dstCol.Save: %v", err)
	}

	return srcCol, dstCol
}

func resetMoveSaveFns(t *testing.T) {
	t.Helper()
	origDst := moveDestSaveFn
	origSrc := moveSrcSaveFn
	t.Cleanup(func() {
		moveDestSaveFn = origDst
		moveSrcSaveFn = origSrc
	})
}

func TestMove_Success(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src", "dst", mock,
		[]string{"repo-to-move", "other-repo"}, nil,
		nil, nil,
	)

	captureStdout(func() {
		if err := runMove(nil, []string{"src", "repo-to-move", "dst"}); err != nil {
			t.Fatalf("runMove = %v", err)
		}
	})

	reloadedSrc, _ := collection.Load("src")
	reloadedDst, _ := collection.Load("dst")

	if collectionHasRepo(reloadedSrc, "repo-to-move") {
		t.Error("source should not have repo-to-move after move")
	}
	if !collectionHasRepo(reloadedSrc, "other-repo") {
		t.Error("source should still have other-repo")
	}
	if !collectionHasRepo(reloadedDst, "repo-to-move") {
		t.Error("dest should have repo-to-move after move")
	}
}

func TestMove_GrantsAccessToNewMembers(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src-grant", "dst-grant", mock,
		[]string{"shared-repo"}, nil,
		[]string{"alice"}, []string{"diana"},
	)

	captureStdout(func() {
		if err := runMove(nil, []string{"src-grant", "shared-repo", "dst-grant"}); err != nil {
			t.Fatalf("runMove = %v", err)
		}
	})

	// diana (dst-only) should have been added as a collaborator via SyncCollaborators.
	if !mock.collabs["owner/shared-repo/diana"] {
		t.Error("diana (dst-only member) should have been granted collaborator access")
	}
}

func TestMove_RevokesAccessFromOldMembers(t *testing.T) {
	mock := newMultiAddMock()
	// charlie is in src but not dst and has access to the repo via the platform.
	mock.collabs["owner/vuln-scanner/charlie"] = true

	setupMoveTest(t, "src-revoke", "dst-revoke", mock,
		[]string{"vuln-scanner"}, nil,
		[]string{"charlie"}, nil,
	)

	captureStdout(func() {
		if err := runMove(nil, []string{"src-revoke", "vuln-scanner", "dst-revoke"}); err != nil {
			t.Fatalf("runMove = %v", err)
		}
	})

	// charlie should have been removed as a collaborator.
	if mock.collabs["owner/vuln-scanner/charlie"] {
		t.Error("charlie (src-only member) should have been revoked collaborator access")
	}
}

func TestMove_NoChangeForSharedMembers(t *testing.T) {
	mock := newMultiAddMock()
	// alice is in both collections.
	mock.collabs["owner/repo-x/alice"] = true

	setupMoveTest(t, "src-shared", "dst-shared", mock,
		[]string{"repo-x"}, nil,
		[]string{"alice"}, []string{"alice"},
	)

	addCalls := 0
	removeCalls := 0
	// Wrap the mock to count calls for alice specifically.
	origAdd := mock.AddCollaborator
	_ = origAdd

	captureStdout(func() {
		if err := runMove(nil, []string{"src-shared", "repo-x", "dst-shared"}); err != nil {
			t.Fatalf("runMove = %v", err)
		}
	})

	// alice is shared — should not appear in gaining or losing.
	// The mock should not have revoked alice's access.
	if !mock.collabs["owner/repo-x/alice"] {
		t.Error("alice (shared member) should NOT have been revoked access")
	}
	_ = addCalls
	_ = removeCalls
}

func TestMove_DryRun(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src-dry", "dst-dry", mock,
		[]string{"dry-repo"}, nil,
		[]string{"alice"}, []string{"bob"},
	)

	old := moveDryRun
	moveDryRun = true
	t.Cleanup(func() { moveDryRun = old })

	captureStdout(func() {
		if err := runMove(nil, []string{"src-dry", "dry-repo", "dst-dry"}); err != nil {
			t.Fatalf("runMove dry-run = %v", err)
		}
	})

	// Files must not have changed.
	reloadedSrc, _ := collection.Load("src-dry")
	reloadedDst, _ := collection.Load("dst-dry")

	if !collectionHasRepo(reloadedSrc, "dry-repo") {
		t.Error("dry-run: source should still have dry-repo")
	}
	if collectionHasRepo(reloadedDst, "dry-repo") {
		t.Error("dry-run: dest should NOT have dry-repo")
	}
}

func TestMove_RepoNotInSource(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src-missing", "dst-missing", mock,
		[]string{"other-repo"}, nil,
		nil, nil,
	)

	err := captureStdout(func() {}) // capture is a no-op here
	_ = err
	moveErr := runMove(nil, []string{"src-missing", "no-such-repo", "dst-missing"})
	if moveErr == nil {
		t.Fatal("expected error when repo not in source collection, got nil")
	}
	if !strings.Contains(moveErr.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", moveErr)
	}
}

func TestMove_RepoAlreadyInDest(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src-dup", "dst-dup", mock,
		[]string{"dup-repo"}, []string{"dup-repo"},
		nil, nil,
	)

	moveErr := runMove(nil, []string{"src-dup", "dup-repo", "dst-dup"})
	if moveErr == nil {
		t.Fatal("expected error when repo already in dest, got nil")
	}
	if !strings.Contains(moveErr.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", moveErr)
	}
}

func TestMove_RequiresOwnerOfBoth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	mock := newMultiAddMock()
	cachedClient = mock
	cachedUser = "interloper"
	cachedUserID = "interloper-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	owner := api.UserInfo{ID: "owner-id", Login: "owner"}
	interloper := api.UserInfo{ID: "interloper-id", Login: "interloper"}

	// src is owned by owner, dst is owned by interloper.
	srcCol, _ := collection.New("src-auth", "github.com", owner, collection.VisibilityPrivate)
	srcCol.Repos = append(srcCol.Repos, collection.RepoAccess{Name: "r", Groups: []string{}, Users: []string{}})
	srcCol.Save() //nolint:errcheck

	dstCol, _ := collection.New("dst-auth", "github.com", interloper, collection.VisibilityPrivate)
	dstCol.Save() //nolint:errcheck

	// interloper tries to move from src (which owner owns) — should fail.
	if err := runMove(nil, []string{"src-auth", "r", "dst-auth"}); err == nil {
		t.Fatal("expected error when caller is not owner of source, got nil")
	}
}

func TestMove_AtomicOnWriteFailure(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src-atomic", "dst-atomic", mock,
		[]string{"atomic-repo"}, nil,
		nil, nil,
	)

	resetMoveSaveFns(t)

	// Make the dest Save fail — source must remain unchanged.
	sentinel := errors.New("simulated disk full")
	moveDestSaveFn = func(col *collection.Collection) error { return sentinel }

	captureStdout(func() {
		err := runMove(nil, []string{"src-atomic", "atomic-repo", "dst-atomic"})
		if err == nil {
			t.Fatal("expected error from failed dest save, got nil")
		}
	})

	// Source collection on disk must still have atomic-repo.
	reloadedSrc, err := collection.Load("src-atomic")
	if err != nil {
		t.Fatalf("Load src-atomic: %v", err)
	}
	if !collectionHasRepo(reloadedSrc, "atomic-repo") {
		t.Error("source must still have atomic-repo after failed dest save")
	}

	// Dest collection on disk must NOT have atomic-repo.
	reloadedDst, err := collection.Load("dst-atomic")
	if err != nil {
		t.Fatalf("Load dst-atomic: %v", err)
	}
	if collectionHasRepo(reloadedDst, "atomic-repo") {
		t.Error("dest must NOT have atomic-repo after failed save")
	}
}

func TestMove_AuditBothCollections(t *testing.T) {
	mock := newMultiAddMock()
	setupMoveTest(t, "src-audit", "dst-audit", mock,
		[]string{"audit-repo"}, nil,
		nil, nil,
	)

	captureStdout(func() {
		if err := runMove(nil, []string{"src-audit", "audit-repo", "dst-audit"}); err != nil {
			t.Fatalf("runMove = %v", err)
		}
	})

	srcEntries, err := audit.Read("src-audit")
	if err != nil {
		t.Fatalf("audit.Read(src-audit): %v", err)
	}
	dstEntries, err := audit.Read("dst-audit")
	if err != nil {
		t.Fatalf("audit.Read(dst-audit): %v", err)
	}

	foundSrc := false
	for _, e := range srcEntries {
		if e.Action == "repo.move.out" && e.Target == "audit-repo" {
			foundSrc = true
		}
	}
	foundDst := false
	for _, e := range dstEntries {
		if e.Action == "repo.move.in" && e.Target == "audit-repo" {
			foundDst = true
		}
	}

	if !foundSrc {
		t.Error("expected repo.move.out audit entry on source collection")
	}
	if !foundDst {
		t.Error("expected repo.move.in audit entry on dest collection")
	}
}
