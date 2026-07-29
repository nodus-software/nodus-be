package test_auth

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

func TestRecoveryCodeIsExplicitAndSingleUse(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)
	raw := "K7XP-L2QZ-9MNT-4RSC"
	id, _ := utility.GenerateUUID()
	env.Repo.backup[id] = backupCode{userID, security.HashToken(security.NormalizeRecoveryCode(raw)), false}
	login := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{"email": "jdoe@example.com", "password": "Sup3rSecret!Pass"})
	var challenge struct {
		ChallengeToken string   `json:"challenge_token"`
		Methods        []string `json:"mfa_methods"`
	}
	Decode(t, login, &challenge)
	if !slices.Contains(challenge.Methods, "recovery_code") {
		t.Fatalf("expected explicit recovery_code method, got %v", challenge.Methods)
	}
	first := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{"challenge_token": challenge.ChallengeToken, "method": "recovery_code", "code": " k7xp-l2qz-9mnt-4rsc "})
	if first.Code != http.StatusOK {
		t.Fatalf("expected recovery login success: %d %s", first.Code, first.Body.String())
	}
	challenge2 := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")
	replay := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{"challenge_token": challenge2, "method": "recovery_code", "code": raw})
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay rejection: %d", replay.Code)
	}
}

func TestLoginMFA_GoldenPath_IssuesTokenPair(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)

	challenge := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")
	code := CurrentTOTPCode(t, secret)

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "totp", "code": code,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var pair struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	Decode(t, rec, &pair)
	if pair.AccessToken == "" || pair.ExpiresIn <= 0 {
		t.Fatalf("expected a populated token pair, got %+v", pair)
	}
	if strings.Contains(rec.Body.String(), "refresh_token") {
		t.Fatal("refresh token must not be exposed in the JSON response")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Value == "" {
		t.Fatalf("expected an HttpOnly refresh cookie, got %#v", cookies)
	}
}

func TestLoginMFA_WrongCode_Returns401(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	env.EnrollTOTP(t, userID)

	challenge := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")

	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{
		"challenge_token": challenge, "method": "totp", "code": "000000",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginMFA_AcceptsAdjacentTOTPInterval(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)
	challenge := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")
	code, err := totp.GenerateCodeCustom(secret, time.Now().Add(-30*time.Second), totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	rec := env.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]string{"challenge_token": challenge, "method": "totp", "code": code})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected adjacent interval to be accepted, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginMFA_ChallengeIsSingleUse(t *testing.T) {
	env := Setup(t)
	userID := env.CreateUser(t, "jdoe", "jdoe@example.com", "Sup3rSecret!Pass")
	secret := env.EnrollTOTP(t, userID)

	challenge := loginToChallenge(t, env, "jdoe@example.com", "Sup3rSecret!Pass")
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
		Secret    string `json:"secret"`
		QRCodeURI string `json:"qr_code_uri"`
	}
	Decode(t, setupRec, &setup)
	if setup.Secret == "" || setup.QRCodeURI == "" {
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

func TestWebAuthnRegistrationOptionsRequireCurrentPassword(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)
	bad := env.JSON(t, http.MethodPost, "/auth/mfa/webauthn/register/options", user.AccessToken, map[string]string{"label": "Work laptop", "current_password": "wrong"})
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d: %s", bad.Code, bad.Body.String())
	}
	good := env.JSON(t, http.MethodPost, "/auth/mfa/webauthn/register/options", user.AccessToken, map[string]string{"label": "Work laptop", "current_password": "Sup3rSecret!Pass"})
	if good.Code != http.StatusOK {
		t.Fatalf("expected options, got %d: %s", good.Code, good.Body.String())
	}
	var result struct {
		CeremonyID string `json:"ceremony_id"`
		PublicKey  struct {
			Challenge string `json:"challenge"`
		} `json:"public_key"`
	}
	Decode(t, good, &result)
	if result.CeremonyID == "" || result.PublicKey.Challenge == "" {
		t.Fatalf("missing ceremony/options: %+v", result)
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

	rec := env.JSON(t, http.MethodPost, "/auth/mfa/factors/"+factors[0].ID+"/remove", user.AccessToken, map[string]string{"current_password": "Sup3rSecret!Pass"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 removing the last factor, got %d: %s", rec.Code, rec.Body.String())
	}
}
