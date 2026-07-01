package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/config"
)

// TestCurrentUserInfo_SavesBothLoginAndID is the regression guard for the
// Session 13 addition that also persists the platform user ID to config.
// If config.SaveUserID is removed or skipped, roleFor (and every command
// that relies on callerID) will silently break for Version "2" collections.
func TestCurrentUserInfo_SavesBothLoginAndID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetCallerCache(t)

	mock := &rootMock{userInfo: api.UserInfo{ID: "alice-platform-id", Login: "alice-login"}}

	if _, err := currentUserInfo(mock); err != nil {
		t.Fatalf("currentUserInfo: %v", err)
	}

	login, err := config.LoadUser("github.com")
	if err != nil {
		t.Fatalf("config.LoadUser: %v", err)
	}
	if login != "alice-login" {
		t.Errorf("config login = %q, want alice-login", login)
	}

	id, err := config.LoadUserID("github.com")
	if err != nil {
		t.Fatalf("config.LoadUserID: %v", err)
	}
	if id != "alice-platform-id" {
		t.Errorf("config userID = %q, want alice-platform-id", id)
	}
}

// TestCurrentUserInfo_LoginAndIDMatchResponse verifies that the UserInfo
// returned from currentUserInfo carries both the Login and the ID fields
// intact — a subtle regression risk if the struct assignment ever drops
// one of the two fields.
func TestCurrentUserInfo_LoginAndIDMatchResponse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetCallerCache(t)

	mock := &rootMock{userInfo: api.UserInfo{ID: "bob-id-12345", Login: "bob-login"}}

	user, err := currentUserInfo(mock)
	if err != nil {
		t.Fatalf("currentUserInfo: %v", err)
	}
	if user.Login != "bob-login" {
		t.Errorf("Login = %q, want bob-login", user.Login)
	}
	if user.ID != "bob-id-12345" {
		t.Errorf("ID = %q, want bob-id-12345", user.ID)
	}
}
