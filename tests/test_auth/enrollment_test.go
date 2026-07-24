package test_auth

import (
	"net/http"
	"testing"
	"time"

	"nodus-health/pkg/security"
)

func TestEnrollmentToken_AllowsInitialTOTPSetupAndIsConsumed(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "founder@example.com", "founder@example.com", "Sup3rSecret!Pass")
	rawToken := "one-time-enrollment-token"
	env.Repo.enrollments["enrollment-id"] = enrollmentToken{
		id: "enrollment-id", userID: userID, hash: security.HashToken(rawToken),
		expiresAt: time.Now().Add(time.Hour),
	}

	setup := env.JSON(t, http.MethodPost, "/auth/mfa/totp/setup", rawToken, nil)
	if setup.Code != http.StatusOK {
		t.Fatalf("setup: expected 200, got %d: %s", setup.Code, setup.Body.String())
	}
	var payload struct {
		Secret string `json:"secret"`
	}
	Decode(t, setup, &payload)

	confirm := env.JSON(t, http.MethodPost, "/auth/mfa/totp/confirm", rawToken, map[string]string{
		"code": CurrentTOTPCode(t, payload.Secret),
	})
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d: %s", confirm.Code, confirm.Body.String())
	}
	if !env.Repo.enrollments["enrollment-id"].consumed {
		t.Fatal("expected enrollment token to be consumed after confirmation")
	}

	reuse := env.JSON(t, http.MethodPost, "/auth/mfa/totp/setup", rawToken, nil)
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("reuse: expected 401, got %d: %s", reuse.Code, reuse.Body.String())
	}
}
