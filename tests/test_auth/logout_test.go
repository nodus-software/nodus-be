package test_auth

import (
	"net/http"
	"testing"

	"nodus-health/internal/auth"
)

func TestLogout_RevokesSessionAndAccessToken(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, refreshToken := env.CompleteLogin(t, "jdoe@example.com", "Sup3rSecret!Pass", secret)

	meBefore := env.JSON(t, http.MethodGet, "/auth/me", accessToken, nil)
	if meBefore.Code != http.StatusOK {
		t.Fatalf("expected /auth/me to work before logout, got %d: %s", meBefore.Code, meBefore.Body.String())
	}

	logoutRec := env.JSONWithCookie(t, http.MethodPost, "/auth/logout", accessToken, nil, &http.Cookie{Name: "nodus_refresh", Value: refreshToken})
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", logoutRec.Code, logoutRec.Body.String())
	}
	if cookies := logoutRec.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("expected logout to clear refresh cookie, got %#v", cookies)
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

// A superuser role carries no explicit role_permissions rows - it's authorized via the
// "*" wildcard (see Service.Authorize) instead of a resolved permission list. /auth/me
// must report that same wildcard, or clients that gate UI on the permissions array (as
// opposed to calling the API and checking for a 403) will hide everything for the one
// account - the founding org admin - that can actually do anything.
func TestMe_SuperuserRole_ReturnsWildcardPermission(t *testing.T) {
	env := Setup(t)
	id := env.CreateUser(t, "admin", "admin@example.com", "Sup3rSecret!Pass")
	env.Repo.roles[id] = []auth.Role{{ID: "role", Name: "Administrator", IsSuperuserRole: true}}
	sid := env.CreateSession(t, id)
	accessToken := env.IssueAccessToken(t, id, sid)

	rec := env.JSON(t, http.MethodGet, "/auth/me", accessToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var profile struct {
		Permissions []string `json:"permissions"`
	}
	Decode(t, rec, &profile)

	if len(profile.Permissions) != 1 || profile.Permissions[0] != "*" {
		t.Fatalf("expected wildcard permission for superuser, got %v", profile.Permissions)
	}
}
