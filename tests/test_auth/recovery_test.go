package test_auth

import (
	"net/http"
	"testing"

	"nodus-health/internal/auth"
)

func TestUnifiedRecoveryPasswordFlowAndTokenReplay(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "recover", "recover@example.com", "OldPassw0rd!")
	sessionID := env.CreateSession(t, userID)

	requested := env.JSON(t, http.MethodPost, "/auth/recovery/request", "", map[string]string{"email": "recover@example.com", "intent": "password"})
	if requested.Code != http.StatusAccepted {
		t.Fatalf("request: %d %s", requested.Code, requested.Body.String())
	}
	emailToken := env.Mailer.LastToken()
	if emailToken == "" {
		t.Fatal("expected recovery email token")
	}

	verified := env.JSON(t, http.MethodPost, "/auth/recovery/verify", "", map[string]string{"token": emailToken})
	if verified.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", verified.Code, verified.Body.String())
	}
	var result auth.RecoveryVerifyResponse
	Decode(t, verified, &result)
	if len(result.Capabilities) != 1 || result.Capabilities[0] != "password" {
		t.Fatalf("unexpected capabilities: %v", result.Capabilities)
	}

	replay := env.JSON(t, http.MethodPost, "/auth/recovery/verify", "", map[string]string{"token": emailToken})
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("email token replay: %d %s", replay.Code, replay.Body.String())
	}

	completed := env.JSON(t, http.MethodPost, "/auth/recovery/password", "", map[string]string{"recovery_token": result.RecoveryToken, "new_password": "NewPassw0rd!Safe"})
	if completed.Code != http.StatusNoContent {
		t.Fatalf("complete: %d %s", completed.Code, completed.Body.String())
	}
	if env.Repo.sessions[sessionID].RevokedAt == nil {
		t.Fatal("recovery must revoke existing sessions")
	}
	if env.Repo.users[userID].Status != auth.UserStatusActive {
		t.Fatal("recovery changed account status")
	}

	reused := env.JSON(t, http.MethodPost, "/auth/recovery/password", "", map[string]string{"recovery_token": result.RecoveryToken, "new_password": "AnotherPassw0rd!"})
	if reused.Code != http.StatusBadRequest {
		t.Fatalf("session replay: %d %s", reused.Code, reused.Body.String())
	}
}

func TestUnifiedRecoveryRequestDoesNotRevealAccount(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, "known", "known@example.com", "KnownPassw0rd!")
	known := env.JSON(t, http.MethodPost, "/auth/recovery/request", "", map[string]string{"email": "known@example.com", "intent": "both"})
	unknown := env.JSON(t, http.MethodPost, "/auth/recovery/request", "", map[string]string{"email": "missing@example.com", "intent": "both"})
	if known.Code != http.StatusAccepted || unknown.Code != known.Code || unknown.Body.String() != known.Body.String() {
		t.Fatalf("responses differ: known=%d %s unknown=%d %s", known.Code, known.Body.String(), unknown.Code, unknown.Body.String())
	}
}
