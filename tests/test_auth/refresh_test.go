package test_auth

import (
	"net/http"
	"testing"
)

func TestRefresh_GoldenPath_RotatesToken(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, refreshToken := env.CompleteLogin(t, "jdoe", "Sup3rSecret!Pass", secret)

	refreshRec := env.JSON(t, http.MethodPost, "/auth/refresh", "", map[string]string{
		"refresh_token": refreshToken,
	})
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", refreshRec.Code, refreshRec.Body.String())
	}

	var newPair struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	Decode(t, refreshRec, &newPair)
	if newPair.RefreshToken == refreshToken {
		t.Fatal("expected refresh token to rotate, got the same value back")
	}
	if newPair.AccessToken == accessToken {
		t.Fatal("expected a new access token")
	}

	// The old refresh token must now be revoked (single use).
	replayRec := env.JSON(t, http.MethodPost, "/auth/refresh", "", map[string]string{
		"refresh_token": refreshToken,
	})
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay of old refresh token to be rejected with 401, got %d", replayRec.Code)
	}
}

func TestRefresh_UnknownToken_Returns401(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/refresh", "", map[string]string{
		"refresh_token": "not-a-real-token",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}
