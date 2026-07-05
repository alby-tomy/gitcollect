package cmd

import (
	"testing"

	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/config"
)

func setupWhoamiTest(t *testing.T, getAuthErr error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	if err := config.SaveToken("github.com", "fake-token"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	origFactory := whoamiNewClient
	whoamiNewClient = func(host, token string) api.Client {
		return &rootMock{
			userInfo:   api.UserInfo{ID: "user-id", Login: "testuser"},
			getAuthErr: getAuthErr,
		}
	}
	t.Cleanup(func() { whoamiNewClient = origFactory })
}

func TestWhoami_Check_ValidToken(t *testing.T) {
	setupWhoamiTest(t, nil) // no error from GetAuthenticatedUser

	old := whoamiCheck
	whoamiCheck = true
	t.Cleanup(func() { whoamiCheck = old })

	if err := runWhoami(nil, nil); err != nil {
		t.Errorf("runWhoami --check with valid token = %v, want nil", err)
	}
}

func TestWhoami_Check_RejectedToken(t *testing.T) {
	setupWhoamiTest(t, api.ErrUnauthorized) // simulate rejected token

	old := whoamiCheck
	whoamiCheck = true
	t.Cleanup(func() { whoamiCheck = old })

	err := runWhoami(nil, nil)
	if err == nil {
		t.Fatal("runWhoami --check with rejected token = nil, want error")
	}
}

func TestWhoami_NoCheck_RejectedToken_NoError(t *testing.T) {
	setupWhoamiTest(t, api.ErrUnauthorized) // simulate rejected token

	old := whoamiCheck
	whoamiCheck = false
	t.Cleanup(func() { whoamiCheck = old })

	if err := runWhoami(nil, nil); err != nil {
		t.Errorf("runWhoami (no --check) with rejected token = %v, want nil", err)
	}
}
