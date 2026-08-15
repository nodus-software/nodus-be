package test_auth

import (
	"net/http"
	"testing"
	"time"
)

func TestLogin_GoldenPath_IssuesChallenge(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)

	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "jdoe@example.com", "password": "Sup3rSecret!Pass",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ChallengeToken string   `json:"challenge_token"`
		MFAMethods     []string `json:"mfa_methods"`
	}
	Decode(t, rec, &resp)

	if resp.ChallengeToken == "" {
		t.Fatal("expected a non-empty challenge token")
	}
	if len(resp.MFAMethods) != 1 || resp.MFAMethods[0] != "totp" {
		t.Fatalf("expected mfa_methods=[totp], got %v", resp.MFAMethods)
	}
}

func TestLogin_ExpiredLockStartsFreshAttemptWindow(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)
	user := env.Repo.users[userID]
	user.FailedLoginAttempts = env.Cfg.LockoutMaxAttempts
	expired := time.Now().Add(-time.Minute)
	user.LockedUntil = &expired
	env.Repo.users[userID] = user

	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "jdoe@example.com", "password": "wrong-password",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected a fresh 401 attempt after lock expiry, got %d: %s", rec.Code, rec.Body.String())
	}
	updated := env.Repo.users[userID]
	if updated.FailedLoginAttempts != 1 || updated.LockedUntil != nil {
		t.Fatalf("attempts=%d locked_until=%v", updated.FailedLoginAttempts, updated.LockedUntil)
	}
}

func TestLogin_WrongPassword_Returns401(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)

	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "jdoe@example.com", "password": "wrong-password",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_UnknownUsername_Returns401(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "nobody@example.com", "password": "whatever123",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_NoMFAEnrolled_Returns401(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")

	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "jdoe@example.com", "password": "Sup3rSecret!Pass",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_MissingFields_Returns422(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_AccountLocksAfterMaxFailedAttempts(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)

	maxAttempts := env.Cfg.LockoutMaxAttempts
	for i := 0; i < maxAttempts; i++ {
		env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
			"email": "jdoe@example.com", "password": "wrong-password",
		})
	}

	// One more attempt (even with the CORRECT password) must now be locked.
	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "jdoe@example.com", "password": "Sup3rSecret!Pass",
	})
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423 after %d failed attempts, got %d: %s", maxAttempts, rec.Code, rec.Body.String())
	}
}
