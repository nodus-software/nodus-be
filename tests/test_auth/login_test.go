package test_auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"nodus-health/internal/auth"
)

type testTurnstile struct{ available bool }

func (v testTurnstile) Verify(_ context.Context, token, _ string) (bool, error) {
	if !v.available {
		return false, errors.New("unavailable")
	}
	return token == "valid-test-token", nil
}

type failingRateLimits struct{}

func (failingRateLimits) Increment(context.Context, string, string, time.Duration) (int64, error) {
	return 0, errors.New("redis unavailable")
}
func (failingRateLimits) Count(context.Context, string, string) (int64, error) {
	return 0, errors.New("redis unavailable")
}

func TestLogin_RequiresTurnstileAfterFivePasswordFailures(t *testing.T) {
	env := setupWithSecurity(t, "enforcement", auth.SecurityControls{Turnstile: testTurnstile{available: true}})
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)
	for range 5 {
		env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "wrong-password"})
	}

	challenge := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "Sup3rSecret!Pass"})
	if challenge.Code != http.StatusTooManyRequests || !strings.Contains(challenge.Body.String(), "AUTH_CHALLENGE_REQUIRED") {
		t.Fatalf("expected Turnstile challenge, got %d: %s", challenge.Code, challenge.Body.String())
	}
	accepted := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "Sup3rSecret!Pass", "turnstile_token": "valid-test-token"})
	if accepted.Code != http.StatusOK {
		t.Fatalf("expected verified login, got %d: %s", accepted.Code, accepted.Body.String())
	}
}

func TestLoginRestrictionNotificationIsAggregatedPerWindow(t *testing.T) {
	env := setupWithSecurity(t, "enforcement", auth.SecurityControls{Turnstile: testTurnstile{available: true}})
	userID := env.CreateUser(t, "notify-lock", "notify-lock@example.com", "CorrectPassw0rd!")
	for attempt := 1; attempt <= 10; attempt++ {
		body := map[string]string{"email": "notify-lock@example.com", "password": "WrongPassw0rd!"}
		if attempt > 5 {
			body["turnstile_token"] = "valid-test-token"
		}
		rec := env.JSON(t, http.MethodPost, "/auth/login", "", body)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %s", attempt, rec.Code, rec.Body.String())
		}
		if attempt >= 6 && attempt < 10 {
			state := env.Repo.failureStates[userID+":password"]
			past := time.Now().Add(-time.Second)
			state.NextAttemptAt = &past
			env.Repo.failureStates[userID+":password"] = state
		}
	}
	if len(env.Mailer.Sent) != 1 {
		t.Fatalf("expected one aggregated lock notification, got %d", len(env.Mailer.Sent))
	}
	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "notify-lock@example.com", "password": "WrongPassw0rd!", "turnstile_token": "valid-test-token"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("restricted retry: %d %s", rec.Code, rec.Body.String())
	}
	if len(env.Mailer.Sent) != 1 {
		t.Fatalf("restricted retry flooded notifications: %d", len(env.Mailer.Sent))
	}
}

func TestLogin_TurnstileOutageFailsClosedOnlyWhenRequired(t *testing.T) {
	env := setupWithSecurity(t, "enforcement", auth.SecurityControls{Turnstile: testTurnstile{}})
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)
	for range 5 {
		env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "wrong-password"})
	}
	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "Sup3rSecret!Pass", "turnstile_token": "anything"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_RedisOutageFailsOpenToPostgresControls(t *testing.T) {
	env := setupWithSecurity(t, "enforcement", auth.SecurityControls{RateLimits: failingRateLimits{}, Turnstile: testTurnstile{available: true}})
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)
	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "Sup3rSecret!Pass"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected PostgreSQL-controlled login to continue, got %d: %s", rec.Code, rec.Body.String())
	}
}

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
	state := env.Repo.failureStates[userID+":password"]
	if updated.FailedLoginAttempts != 0 || updated.LockedUntil != nil || state.FailureCount != 1 {
		t.Fatalf("legacy_attempts=%d locked_until=%v observed_attempts=%d", updated.FailedLoginAttempts, updated.LockedUntil, state.FailureCount)
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

func TestLogin_KnownAndUnknownWrongCredentialsHaveSameResponse(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)
	known := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "wrong-password"})
	unknown := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "nobody@example.com", "password": "wrong-password"})
	if known.Code != unknown.Code || known.Body.String() != unknown.Body.String() {
		t.Fatalf("credential responses differ: known=%d %s unknown=%d %s", known.Code, known.Body.String(), unknown.Code, unknown.Body.String())
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

func TestLogin_LegacyFailureThresholdNoLongerLocksAccount(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)

	maxAttempts := env.Cfg.LockoutMaxAttempts
	for i := 0; i < maxAttempts; i++ {
		env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
			"email": "jdoe@example.com", "password": "wrong-password",
		})
	}

	// Phase 2 no longer lets the legacy shared threshold lock password and MFA
	// together. Observation state is tracked independently until enforcement.
	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": "jdoe@example.com", "password": "Sup3rSecret!Pass",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected correct password to remain usable after legacy threshold, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := env.Repo.failureStates[userID+":password"].FailureCount; got != maxAttempts {
		t.Fatalf("observed password failures=%d, want %d", got, maxAttempts)
	}
}
