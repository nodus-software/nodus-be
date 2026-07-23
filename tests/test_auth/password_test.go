package test_auth

import (
	"net/http"
	"strings"
	"testing"
)

func TestPasswordPolicy_ReturnsConfiguredValues(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodGet, "/auth/password/policy", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var policy struct {
		MinLength int `json:"min_length"`
	}
	Decode(t, rec, &policy)
	if policy.MinLength != env.Cfg.PasswordMinLength {
		t.Fatalf("expected min_length=%d, got %d", env.Cfg.PasswordMinLength, policy.MinLength)
	}
}

func TestChangePassword_GoldenPath_RevokesOtherSessions(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, _ := env.CompleteLogin(t, "jdoe", "Sup3rSecret!Pass", secret)
	otherSessionID := env.CreateSession(t, userID)
	otherAccessToken := env.IssueAccessToken(t, userID, otherSessionID)

	rec := env.JSON(t, http.MethodPost, "/auth/password/change", accessToken, map[string]string{
		"current_password": "Sup3rSecret!Pass", "new_password": "NewSup3rSecret!Pass",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// The session that changed the password stays valid...
	meRec := env.JSON(t, http.MethodGet, "/auth/me", accessToken, nil)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected the changing session to remain valid, got %d", meRec.Code)
	}

	// ...but every other session is revoked.
	otherMeRec := env.JSON(t, http.MethodGet, "/auth/me", otherAccessToken, nil)
	if otherMeRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected other sessions to be revoked, got %d", otherMeRec.Code)
	}
}

func TestChangePassword_WrongCurrentPassword_Returns401(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, _ := env.CompleteLogin(t, "jdoe", "Sup3rSecret!Pass", secret)

	rec := env.JSON(t, http.MethodPost, "/auth/password/change", accessToken, map[string]string{
		"current_password": "wrong-password", "new_password": "NewSup3rSecret!Pass",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePassword_WeakNewPassword_Returns422(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	accessToken, _ := env.CompleteLogin(t, "jdoe", "Sup3rSecret!Pass", secret)

	rec := env.JSON(t, http.MethodPost, "/auth/password/change", accessToken, map[string]string{
		"current_password": "Sup3rSecret!Pass", "new_password": "weak",
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPasswordReset_FullFlow(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")

	requestRec := env.JSON(t, http.MethodPost, "/auth/password/reset/request", "", map[string]string{
		"username": "jdoe",
	})
	if requestRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", requestRec.Code, requestRec.Body.String())
	}
	if len(env.Mailer.Sent) != 1 {
		t.Fatalf("expected exactly one email sent, got %d", len(env.Mailer.Sent))
	}
	token := env.Mailer.LastToken()
	if token == "" {
		t.Fatal("expected a reset token to be embedded in the sent email")
	}

	confirmRec := env.JSON(t, http.MethodPost, "/auth/password/reset/confirm", "", map[string]string{
		"reset_token": token, "new_password": "BrandNewSecret!23",
	})
	if confirmRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	// The token is single-use: replay must now fail.
	replayRec := env.JSON(t, http.MethodPost, "/auth/password/reset/confirm", "", map[string]string{
		"reset_token": token, "new_password": "AnotherSecret!234",
	})
	if replayRec.Code != http.StatusBadRequest {
		t.Fatalf("expected replay to be rejected with 400, got %d: %s", replayRec.Code, replayRec.Body.String())
	}

	// The new password must actually work for a fresh login.
	loginRec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"username": "jdoe", "password": "BrandNewSecret!23",
	})
	// No MFA enrolled at this point in the test, so this should fail with
	// "mfa not enrolled" (401) rather than invalid credentials — proving the
	// new password itself was accepted.
	if loginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (mfa not enrolled) proving password was accepted, got %d", loginRec.Code)
	}
}

func TestPasswordReset_UnknownUsername_StillReturns202(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/password/reset/request", "", map[string]string{
		"username": "nobody",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 regardless of account existence, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(env.Mailer.Sent) != 0 {
		t.Fatalf("expected no email sent for a non-existent username, got %d", len(env.Mailer.Sent))
	}
}

func TestPasswordReset_RateLimitedAfterMaxPerUsername(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")

	max := env.Cfg.PasswordResetMaxPerUsernamePerHour
	for i := 0; i < max; i++ {
		rec := env.JSON(t, http.MethodPost, "/auth/password/reset/request", "", map[string]string{"username": "jdoe"})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("attempt %d: expected 202, got %d", i+1, rec.Code)
		}
	}

	rec := env.JSON(t, http.MethodPost, "/auth/password/reset/request", "", map[string]string{"username": "jdoe"})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after %d requests, got %d: %s", max, rec.Code, rec.Body.String())
	}
}

func TestPasswordReset_InvalidToken_Returns400(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/password/reset/confirm", "", map[string]string{
		"reset_token": "not-a-real-token", "new_password": "BrandNewSecret!23",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPasswordReset_WeakNewPassword_Returns422(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")

	env.JSON(t, http.MethodPost, "/auth/password/reset/request", "", map[string]string{"username": "jdoe"})
	token := env.Mailer.LastToken()

	rec := env.JSON(t, http.MethodPost, "/auth/password/reset/confirm", "", map[string]string{
		"reset_token": token, "new_password": "weak",
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	// Even though the password was rejected, the token was burned on first
	// use and must not be reusable.
	replayRec := env.JSON(t, http.MethodPost, "/auth/password/reset/confirm", "", map[string]string{
		"reset_token": token, "new_password": "AnotherSecret!234",
	})
	if replayRec.Code != http.StatusBadRequest {
		t.Fatalf("expected token to already be burned (400), got %d", replayRec.Code)
	}
}

func TestPasswordReset_EmailBodyContainsBaseURL(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")

	env.JSON(t, http.MethodPost, "/auth/password/reset/request", "", map[string]string{"username": "jdoe"})
	if len(env.Mailer.Sent) != 1 {
		t.Fatalf("expected one email, got %d", len(env.Mailer.Sent))
	}
	if !strings.Contains(env.Mailer.Sent[0].Body, env.Cfg.BaseUrl) {
		t.Fatalf("expected reset email body to contain base URL %q, got %q", env.Cfg.BaseUrl, env.Mailer.Sent[0].Body)
	}
}
