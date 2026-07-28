package test_auth

import (
	"net/http"
	"testing"
	"time"

	"nodus-health/internal/auth"
)

func TestRefresh_GoldenPath_RotatesToken(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, refreshToken := env.CompleteLogin(t, "jdoe@example.com", "Sup3rSecret!Pass", secret)
	refreshCookie := &http.Cookie{Name: "nodus_refresh", Value: refreshToken, Path: "/auth"}

	refreshRec := env.JSONWithCookie(t, http.MethodPost, "/auth/refresh", "", nil, refreshCookie)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", refreshRec.Code, refreshRec.Body.String())
	}

	var newPair struct {
		AccessToken string `json:"access_token"`
	}
	Decode(t, refreshRec, &newPair)
	rotatedCookies := refreshRec.Result().Cookies()
	if len(rotatedCookies) != 1 || rotatedCookies[0].Value == "" || rotatedCookies[0].Value == refreshToken {
		t.Fatalf("expected refresh cookie to rotate, got %#v", rotatedCookies)
	}
	if newPair.AccessToken == accessToken {
		t.Fatal("expected a new access token")
	}

	// The old refresh token must now be revoked (single use).
	replayRec := env.JSONWithCookie(t, http.MethodPost, "/auth/refresh", "", nil, refreshCookie)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay of old refresh token to be rejected with 401, got %d", replayRec.Code)
	}
	if cookies := replayRec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("revoked-token replay must not clear a cookie concurrently rotated by another tab, got %#v", cookies)
	}
}

func TestRefresh_UnknownToken_Returns401(t *testing.T) {
	env := Setup(t)

	rec := env.JSONWithCookie(t, http.MethodPost, "/auth/refresh", "", nil, &http.Cookie{Name: "nodus_refresh", Value: "not-a-real-token"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRememberMe_ControlsCookieAndServerLifetime(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)

	sessionChallenge := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")
	sessionRec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]any{
		"challenge_token": sessionChallenge, "method": "totp", "code": CurrentTOTPCode(t, secret), "remember_me": false,
	})
	sessionCookie := sessionRec.Result().Cookies()[0]
	if sessionCookie.MaxAge != 0 || !sessionCookie.Expires.IsZero() {
		t.Fatalf("expected browser-session cookie, got %#v", sessionCookie)
	}
	var sessionToken auth.RefreshToken
	for _, token := range env.Repo.refresh {
		sessionToken = token
	}
	if remaining := time.Until(sessionToken.ExpiresAt); remaining < 119*time.Minute || remaining > 121*time.Minute {
		t.Fatalf("expected non-remembered token lifetime near 2h, got %s", remaining)
	}

	rememberedChallenge := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")
	rememberedRec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]any{
		"challenge_token": rememberedChallenge, "method": "totp", "code": CurrentTOTPCode(t, secret), "remember_me": true,
	})
	rememberedCookie := rememberedRec.Result().Cookies()[0]
	if rememberedCookie.MaxAge <= 0 || rememberedCookie.Expires.IsZero() {
		t.Fatalf("expected persistent cookie, got %#v", rememberedCookie)
	}
	var remembered auth.RefreshToken
	for _, token := range env.Repo.refresh {
		if token.ExpiresAt.After(remembered.ExpiresAt) {
			remembered = token
		}
	}
	if remaining := time.Until(remembered.ExpiresAt); remaining < 23*time.Hour || remaining > 25*time.Hour {
		t.Fatalf("expected remembered token lifetime near 24h, got %s", remaining)
	}

	refreshRec := env.JSONWithCookie(t, http.MethodPost, "/auth/refresh", "", nil, rememberedCookie)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected remembered refresh to succeed, got %d: %s", refreshRec.Code, refreshRec.Body.String())
	}
	rotated := refreshRec.Result().Cookies()[0]
	if rotated.MaxAge <= 0 || rotated.Expires.IsZero() {
		t.Fatalf("expected rotation to preserve persistent cookie, got %#v", rotated)
	}
}

func TestRefresh_WithoutCookie_Returns401AndClearsCookie(t *testing.T) {
	env := Setup(t)
	rec := env.JSON(t, http.MethodPost, "/auth/refresh", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("expected expired refresh cookie, got %#v", cookies)
	}
}
