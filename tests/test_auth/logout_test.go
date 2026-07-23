package test_auth

import (
	"net/http"
	"testing"
)

func TestLogout_RevokesSessionAndAccessToken(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, _ := env.CompleteLogin(t, "jdoe", "Sup3rSecret!Pass", secret)

	meBefore := env.JSON(t, http.MethodGet, "/auth/me", accessToken, nil)
	if meBefore.Code != http.StatusOK {
		t.Fatalf("expected /auth/me to work before logout, got %d: %s", meBefore.Code, meBefore.Body.String())
	}

	logoutRec := env.JSON(t, http.MethodPost, "/auth/logout", accessToken, nil)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", logoutRec.Code, logoutRec.Body.String())
	}

	meAfter := env.JSON(t, http.MethodGet, "/auth/me", accessToken, nil)
	if meAfter.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d: %s", meAfter.Code, meAfter.Body.String())
	}
}

func TestLogout_WithoutToken_Returns401(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/logout", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMe_ReturnsRolesAndPermissions(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t, "patients:read", "patients:write")

	rec := env.JSON(t, http.MethodGet, "/auth/me", user.AccessToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var profile struct {
		ID          string   `json:"id"`
		Permissions []string `json:"permissions"`
		Roles       []string `json:"roles"`
		MFAEnrolled bool     `json:"mfa_enrolled"`
	}
	Decode(t, rec, &profile)

	if profile.ID != user.UserID {
		t.Fatalf("expected id %s, got %s", user.UserID, profile.ID)
	}
	if len(profile.Roles) != 1 {
		t.Fatalf("expected one role, got %v", profile.Roles)
	}
	if len(profile.Permissions) != 2 {
		t.Fatalf("expected two permissions, got %v", profile.Permissions)
	}
}
