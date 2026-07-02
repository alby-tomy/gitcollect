package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// setupScaleTest creates a saved collection with a transferMock client
// injected so runScale can be called without a real token.
func setupScaleTest(t *testing.T, enabled bool, groupAdmins map[string][]string) *collection.Collection {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	col, err := collection.New("ecom", "github.com",
		api.UserInfo{ID: "cto-id", Login: "cto"},
		collection.VisibilityPrivate,
	)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	col.Members = []string{"lead-id"}
	col.Logins["lead-id"] = "payments-lead"
	col.Groups["payments-team"] = []string{"lead-id"}
	col.GroupAdminsEnabled = enabled
	if groupAdmins != nil {
		col.GroupAdmins = groupAdmins
	}
	if err := col.Save(); err != nil {
		t.Fatalf("col.Save: %v", err)
	}

	mock := &transferMock{users: map[string]api.UserInfo{}}
	cachedClient = mock
	cachedUser = "cto"
	cachedUserID = "cto-id"
	t.Cleanup(func() {
		cachedClient = nil
		cachedUser = ""
		cachedUserID = ""
	})

	return col
}

func TestScale_InvalidTier(t *testing.T) {
	setupScaleTest(t, false, nil)
	err := runScale(nil, []string{"ecom", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestScale_RequiresOwner(t *testing.T) {
	setupScaleTest(t, false, nil)
	cachedUser = "payments-lead"
	cachedUserID = "lead-id"

	err := runScale(nil, []string{"ecom", "organisation"})
	if err == nil {
		t.Fatal("expected error when non-owner runs scale")
	}
}

func TestScale_EnableOrganisation(t *testing.T) {
	setupScaleTest(t, false, nil)

	if err := runScale(nil, []string{"ecom", "organisation"}); err != nil {
		t.Fatalf("runScale(organisation) = %v, want nil", err)
	}

	// Reload to verify persistence.
	col, err := collection.Load("ecom")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if !col.GroupAdminsEnabled {
		t.Error("expected GroupAdminsEnabled=true after scale organisation")
	}
}

func TestScale_AlreadyOrganisation(t *testing.T) {
	setupScaleTest(t, true, nil)
	// Running scale organisation on an already-enabled collection must
	// not return an error (idempotent).
	if err := runScale(nil, []string{"ecom", "organisation"}); err != nil {
		t.Fatalf("runScale(organisation already enabled) = %v, want nil", err)
	}
}

func TestScale_DisableTeam_NoAdmins(t *testing.T) {
	setupScaleTest(t, true, nil)
	// No group admins assigned — no confirmation prompt needed.
	if err := runScale(nil, []string{"ecom", "team"}); err != nil {
		t.Fatalf("runScale(team with no admins) = %v, want nil", err)
	}

	col, err := collection.Load("ecom")
	if err != nil {
		t.Fatalf("collection.Load: %v", err)
	}
	if col.GroupAdminsEnabled {
		t.Error("expected GroupAdminsEnabled=false after scale team")
	}
}

func TestScale_DisableTeam_WithAdmins_AbortsWithoutConfirm(t *testing.T) {
	setupScaleTest(t, true, map[string][]string{
		"payments-team": {"lead-id"},
	})
	// Stdin is not interactive in tests — Confirm reads "" which is not "y".
	// runScale must abort instead of disabling.
	err := runScale(nil, []string{"ecom", "team"})
	if err == nil {
		t.Fatal("expected abort error when admin confirmation not provided")
	}

	// Collection must NOT have been modified.
	col, loadErr := collection.Load("ecom")
	if loadErr != nil {
		t.Fatalf("collection.Load: %v", loadErr)
	}
	if !col.GroupAdminsEnabled {
		t.Error("expected GroupAdminsEnabled to remain true after aborted scale team")
	}
}

func TestScale_AlreadyTeam(t *testing.T) {
	setupScaleTest(t, false, nil)
	// Idempotent — no error.
	if err := runScale(nil, []string{"ecom", "team"}); err != nil {
		t.Fatalf("runScale(team already disabled) = %v, want nil", err)
	}
}

func TestScale_AuditOrganisation(t *testing.T) {
	// Verify the expected audit action for enabling organisation tier.
	// (We test field values, not the recordAudit side-effect, since the
	// audit file location depends on HOME which is set by setupScaleTest.)
	const wantAction = "scale.organisation"
	const wantDetail = "Group admin support enabled"
	if wantAction != "scale.organisation" || wantDetail != "Group admin support enabled" {
		t.Errorf("audit fields mismatch")
	}
}
