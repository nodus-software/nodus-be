package test_auth

import (
	"net/http"
	"testing"
)

func TestLoginMFA_GoldenPath_IssuesTokenPair(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)

	challenge := loginToChallenge(t, env, "jdoe", "Sup3rSecret!Pass")
	code := CurrentTOTPCode(t, secret)

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "totp", "code": code,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var pair struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	Decode(t, rec, &pair)
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.ExpiresIn <= 0 {
		t.Fatalf("expected a populated token pair, got %+v", pair)
	}
}

func TestLoginMFA_WrongCode_Returns401(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)

	challenge := loginToChallenge(t, env, "jdoe", "Sup3rSecret!Pass")

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "totp", "code": "000000",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginMFA_ChallengeIsSingleUse(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)

	challenge := loginToChallenge(t, env, "jdoe", "Sup3rSecret!Pass")
	code := CurrentTOTPCode(t, secret)

	first := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "totp", "code": code,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("expected first attempt to succeed, got %d: %s", first.Code, first.Body.String())
	}

	second := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "totp", "code": code,
	})
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay to be rejected with 401, got %d: %s", second.Code, second.Body.String())
	}
}

func TestLoginMFA_UnknownChallenge_Returns401(t *testing.T) {
	env := Setup(t)

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": "not-a-real-token", "method": "totp", "code": "123456",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMFA_SetupAndConfirmTOTP(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)

	setupRec := env.JSON(t, http.MethodPost, "/auth/mfa/totp/setup", user.AccessToken, nil)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from setup, got %d: %s", setupRec.Code, setupRec.Body.String())
	}
	var setup struct {
		Secret      string   `json:"secret"`
		QRCodeURI   string   `json:"qr_code_uri"`
		BackupCodes []string `json:"backup_codes"`
	}
	Decode(t, setupRec, &setup)
	if setup.Secret == "" || setup.QRCodeURI == "" || len(setup.BackupCodes) == 0 {
		t.Fatalf("expected populated totp setup response, got %+v", setup)
	}

	code := CurrentTOTPCode(t, setup.Secret)
	confirmRec := env.JSON(t, http.MethodPost, "/auth/mfa/totp/confirm", user.AccessToken, map[string]string{"code": code})
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from confirm, got %d: %s", confirmRec.Code, confirmRec.Body.String())
	}

	factorsRec := env.JSON(t, http.MethodGet, "/auth/mfa/factors", user.AccessToken, nil)
	var factors []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	Decode(t, factorsRec, &factors)
	if len(factors) != 1 || factors[0].Type != "totp" {
		t.Fatalf("expected one confirmed totp factor, got %+v", factors)
	}
}

func TestMFA_ConfirmTOTP_WrongCode_Returns400(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)

	setupRec := env.JSON(t, http.MethodPost, "/auth/mfa/totp/setup", user.AccessToken, nil)
	var setup struct {
		Secret string `json:"secret"`
	}
	Decode(t, setupRec, &setup)

	rec := env.JSON(t, http.MethodPost, "/auth/mfa/totp/confirm", user.AccessToken, map[string]string{"code": "000000"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMFA_RemoveFactor_RefusesLastRemaining(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)
	env.EnrollTOTP(t, user.UserID)

	factorsRec := env.JSON(t, http.MethodGet, "/auth/mfa/factors", user.AccessToken, nil)
	var factors []struct {
		ID string `json:"id"`
	}
	Decode(t, factorsRec, &factors)
	if len(factors) != 1 {
		t.Fatalf("expected exactly one enrolled factor, got %d", len(factors))
	}

	rec := env.JSON(t, http.MethodDelete, "/auth/mfa/factors/"+factors[0].ID, user.AccessToken, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 removing the last factor, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginMFA_Biometric_GoldenPath(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	pub, priv := generateEd25519Keypair(t)
	env.EnrollBiometric(t, userID, pub)

	challenge := loginToChallenge(t, env, "jdoe", "Sup3rSecret!Pass")
	signature := signChallenge(priv, challenge)

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "biometric", "code": signature,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginMFA_Biometric_WrongSignature_Returns401(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	pub, _ := generateEd25519Keypair(t)
	_, otherPriv := generateEd25519Keypair(t)
	env.EnrollBiometric(t, userID, pub)

	challenge := loginToChallenge(t, env, "jdoe", "Sup3rSecret!Pass")
	signature := signChallenge(otherPriv, challenge) // signed with the WRONG key

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "biometric", "code": signature,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMFA_RegisterBiometric_AddsFactorImmediately(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)

	pub, _ := generateEd25519Keypair(t)
	rec := env.JSON(t, http.MethodPost, "/auth/mfa/biometric/register", user.AccessToken, map[string]string{
		"device_public_key": pub, "device_label": "Test Phone",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	factorsRec := env.JSON(t, http.MethodGet, "/auth/mfa/factors", user.AccessToken, nil)
	var factors []struct {
		Type string `json:"type"`
	}
	Decode(t, factorsRec, &factors)
	if len(factors) != 1 || factors[0].Type != "biometric" {
		t.Fatalf("expected one biometric factor, got %+v", factors)
	}
}
