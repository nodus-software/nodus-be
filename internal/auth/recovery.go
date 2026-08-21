package auth

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	"nodus-health/internal/audit"
	"nodus-health/internal/email"
	"nodus-health/internal/tenant"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

func (s *Service) RequestRecovery(ctx context.Context, req RecoveryRequest, ip string) error {
	emailAddress := strings.ToLower(strings.TrimSpace(req.Email))
	now := time.Now()
	attemptID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	count, err := s.repo.CountPasswordResetAttemptsByUsername(ctx, emailAddress+":"+string(req.Intent), now.Add(-time.Hour))
	if err != nil {
		return err
	}
	ipCount, err := s.repo.CountPasswordResetAttemptsByIP(ctx, ip, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if err := s.repo.RecordPasswordResetAttempt(ctx, attemptID, emailAddress+":"+string(req.Intent), ip); err != nil {
		return err
	}
	allowed := count < s.cfg.PasswordResetMaxPerUsernamePerHour && ipCount < s.cfg.PasswordResetMaxPerIPPerHour
	// A repeated request must prove it is human. The endpoint remains generic,
	// so callers cannot use this transition to discover whether an account exists.
	if count > 0 && (req.TurnstileToken == "" || s.controls.Turnstile == nil) {
		allowed = false
	} else if req.TurnstileToken != "" && s.controls.Turnstile != nil {
		verified, verifyErr := s.controls.Turnstile.Verify(ctx, req.TurnstileToken, ip)
		allowed = allowed && verifyErr == nil && verified
	}
	if s.controls.RateLimits != nil {
		limits := []struct {
			scope, value string
			maximum      int
		}{
			{"recovery:identifier", emailAddress + ":" + string(req.Intent), s.cfg.PasswordResetMaxPerUsernamePerHour},
			{"recovery:recipient", emailAddress, s.cfg.PasswordResetMaxPerUsernamePerHour},
			{"recovery:ip", ip, s.cfg.PasswordResetMaxPerIPPerHour},
		}
		if tenantID, tenantErr := tenant.ID(ctx); tenantErr == nil {
			limits = append(limits, struct {
				scope, value string
				maximum      int
			}{"recovery:tenant", tenantID, s.cfg.TenantLimit})
		}
		for _, limit := range limits {
			n, limitErr := s.controls.RateLimits.Increment(ctx, limit.scope, limit.value, time.Hour)
			if limitErr != nil {
				s.log.Error("recovery supplemental rate limit unavailable", "scope", limit.scope, "error", limitErr.Error())
				continue
			}
			if limit.maximum > 0 && n > int64(limit.maximum) {
				allowed = false
			}
		}
	}
	_ = s.audit.Record(ctx, audit.Entry{Action: "account_recovery_requested", IPAddress: ip, Result: audit.ResultSuccess, Metadata: map[string]any{"intent": req.Intent, "issued": allowed}})
	if !allowed {
		return nil
	}
	user, err := s.repo.GetUserByEmail(ctx, emailAddress)
	if err != nil {
		return nil
	}
	raw, err := security.GenerateToken()
	if err != nil {
		return err
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	expires := now.Add(s.cfg.RecoveryEmailTokenTTL)
	link := s.tenantFrontendURL(ctx, "/account-recovery?token="+url.QueryEscape(raw))
	var tenantID *string
	if id, tenantErr := tenant.ID(ctx); tenantErr == nil {
		tenantID = &id
	}
	text := fmt.Sprintf("Use this link to continue account recovery: %s\nThis link expires at %s.", link, expires.UTC().Format(time.RFC3339))
	htmlBody := "<p>Use this link to continue account recovery:</p><p><a href=\"" + html.EscapeString(link) + "\">Continue account recovery</a></p>"
	return s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.InvalidateRecoveryEmailTokens(ctx, user.ID, req.Intent); err != nil {
			return err
		}
		if err := repo.CreateRecoveryEmailToken(ctx, RecoveryEmailToken{ID: id, UserID: user.ID, Intent: req.Intent, ExpiresAt: expires}, security.HashToken(raw)); err != nil {
			return err
		}
		return repo.QueueEmail(ctx, email.Message{TenantID: tenantID, Kind: email.SecurityNotification, To: user.Email, Subject: "Continue your Nodus Health account recovery", Text: text, HTML: htmlBody, ExpiresAt: &expires})
	})
}

func (s *Service) VerifyRecovery(ctx context.Context, raw string) (*RecoveryVerifyResponse, error) {
	var response *RecoveryVerifyResponse
	var userID string
	err := s.repo.WithinTx(ctx, func(repo Repository) error {
		emailToken, err := repo.ConsumeRecoveryEmailToken(ctx, security.HashToken(raw))
		if err != nil {
			return ErrRecoveryTokenInvalid
		}
		sessionRaw, err := security.GenerateToken()
		if err != nil {
			return err
		}
		id, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		expires := time.Now().Add(s.cfg.RecoverySessionTTL)
		session := RecoverySession{ID: id, UserID: emailToken.UserID, CanResetPassword: emailToken.Intent != RecoveryIntentMFA, CanReplaceMFA: emailToken.Intent != RecoveryIntentPassword, ExpiresAt: expires}
		userID = session.UserID
		if err := repo.CreateRecoverySession(ctx, session, security.HashToken(sessionRaw)); err != nil {
			return err
		}
		if err := repo.InvalidateRecoverySessionsByUser(ctx, session.UserID, session.ID); err != nil {
			return err
		}
		caps := []string{}
		if session.CanResetPassword {
			caps = append(caps, "password")
		}
		if session.CanReplaceMFA {
			caps = append(caps, "mfa")
		}
		response = &RecoveryVerifyResponse{RecoveryToken: sessionRaw, Capabilities: caps, ExpiresAt: expires}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{TargetUserID: &userID, Action: "account_recovery_verified", Result: audit.ResultSuccess})
	return response, nil
}

func (s *Service) validRecoverySession(ctx context.Context, raw string, capability string) (*RecoverySession, error) {
	session, err := s.repo.GetRecoverySessionByHash(ctx, security.HashToken(raw))
	if err != nil || session.ConsumedAt != nil || !session.ExpiresAt.After(time.Now()) || session.FailedAttempts >= s.cfg.RecoveryMaxAttempts {
		return nil, ErrRecoveryTokenInvalid
	}
	if capability == "password" && (!session.CanResetPassword || session.PasswordCompletedAt != nil) {
		return nil, ErrRecoveryTokenInvalid
	}
	if capability == "mfa" && (!session.CanReplaceMFA || session.MFACompletedAt != nil) {
		return nil, ErrRecoveryTokenInvalid
	}
	return session, nil
}

func (s *Service) CompleteRecoveryPassword(ctx context.Context, req RecoveryPasswordRequest) error {
	session, err := s.validRecoverySession(ctx, req.RecoveryToken, "password")
	if err != nil {
		return err
	}
	if violations := ValidatePasswordPolicy(req.NewPassword, s.cfg.PasswordPolicy); len(violations) > 0 {
		return &PolicyViolationError{Violations: violations}
	}
	hash, err := security.HashPassword(req.NewPassword, s.cfg.BcryptCost)
	if err != nil {
		return err
	}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.UpdatePasswordHash(ctx, session.UserID, hash); err != nil {
			return err
		}
		if err := repo.CompleteRecoveryPassword(ctx, session.ID); err != nil {
			return err
		}
		if err := repo.RevokeSessionsByUser(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.RevokeRefreshTokensByUser(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.ResetAuthenticationFailures(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.ResetFailedLoginAttempts(ctx, session.UserID); err != nil {
			return err
		}
		user, err := repo.GetUserByID(ctx, session.UserID)
		if err != nil {
			return err
		}
		body := "Your account password was changed through account recovery. If this was not you, contact your administrator immediately."
		return repo.QueueEmail(ctx, email.Message{Kind: email.SecurityNotification, To: user.Email, Subject: "Your Nodus Health password was recovered", Text: body, HTML: "<p>" + body + "</p>", DedupeKey: "recovery-password:" + user.ID + ":" + time.Now().UTC().Format("2006-01-02")})
	})
	if err == nil {
		_ = s.audit.Record(ctx, audit.Entry{TargetUserID: &session.UserID, Action: "account_recovery_password_completed", Result: audit.ResultSuccess})
	}
	return err
}

func (s *Service) SetupRecoveryTOTP(ctx context.Context, raw string) (*TOTPSetupResponse, error) {
	session, err := s.validRecoverySession(ctx, raw, "mfa")
	if err != nil {
		return nil, err
	}
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.cfg.TOTPIssuer, AccountName: user.Username})
	if err != nil {
		return nil, err
	}
	encrypted, err := security.EncryptString(s.cfg.MFAEncryptionKey, key.Secret())
	if err != nil {
		return nil, err
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.DeletePendingTOTPFactors(ctx, user.ID); err != nil {
			return err
		}
		_, err := repo.CreateMFAFactor(ctx, MFAFactor{ID: id, UserID: user.ID, Type: MFAFactorTOTP, Label: "Authenticator App", SecretEncrypted: &encrypted})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &TOTPSetupResponse{Secret: key.Secret(), QRCodeURI: key.URL()}, nil
}

func (s *Service) ConfirmRecoveryTOTP(ctx context.Context, req RecoveryTOTPConfirmRequest) (*ConfirmTOTPResponse, error) {
	session, err := s.validRecoverySession(ctx, req.RecoveryToken, "mfa")
	if err != nil {
		return nil, err
	}
	factors, err := s.repo.ListMFAFactorsByUser(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	var pending *MFAFactor
	for i := range factors {
		if factors[i].Type == MFAFactorTOTP && !factors[i].IsConfirmed() {
			pending = &factors[i]
			break
		}
	}
	if pending == nil {
		return nil, ErrRecoveryTokenInvalid
	}
	secret, err := security.DecryptString(s.cfg.MFAEncryptionKey, *pending.SecretEncrypted)
	if err != nil {
		return nil, err
	}
	if !validateTOTP(req.Code, secret, time.Now()) {
		_, _ = s.repo.IncrementRecoverySessionFailure(ctx, session.ID)
		return nil, ErrRecoveryTokenInvalid
	}
	result := &ConfirmTOTPResponse{Status: "enabled"}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.DeleteSupersededMFAFactors(ctx, session.UserID, pending.ID); err != nil {
			return err
		}
		if err := repo.ConfirmMFAFactor(ctx, pending.ID); err != nil {
			return err
		}
		if err := repo.InvalidateMFABackupCodes(ctx, session.UserID); err != nil {
			return err
		}
		for range s.cfg.MFABackupCodeCount {
			raw, err := security.GenerateBackupCode()
			if err != nil {
				return err
			}
			id, err := utility.GenerateUUID()
			if err != nil {
				return err
			}
			if err := repo.CreateMFABackupCode(ctx, id, session.UserID, security.HashToken(security.NormalizeRecoveryCode(raw))); err != nil {
				return err
			}
			result.RecoveryCodes = append(result.RecoveryCodes, raw)
		}
		if err := repo.CompleteRecoveryMFA(ctx, session.ID); err != nil {
			return err
		}
		if err := repo.RevokeSessionsByUser(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.RevokeRefreshTokensByUser(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.ResetAuthenticationFailures(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.ResetFailedLoginAttempts(ctx, session.UserID); err != nil {
			return err
		}
		user, err := repo.GetUserByID(ctx, session.UserID)
		if err != nil {
			return err
		}
		body := "Your MFA factors and recovery codes were replaced through account recovery. If this was not you, contact your administrator immediately."
		return repo.QueueEmail(ctx, email.Message{Kind: email.SecurityNotification, To: user.Email, Subject: "Your Nodus Health MFA was recovered", Text: body, HTML: "<p>" + body + "</p>", DedupeKey: "recovery-mfa:" + user.ID + ":" + time.Now().UTC().Format("2006-01-02")})
	})
	if err == nil {
		_ = s.audit.Record(ctx, audit.Entry{TargetUserID: &session.UserID, Action: "account_recovery_mfa_replaced", Result: audit.ResultSuccess})
	}
	return result, err
}

func (s *Service) BeginRecoveryWebAuthn(ctx context.Context, req RecoveryWebAuthnOptionsRequest) (*WebAuthnRegistrationOptionsResponse, error) {
	session, err := s.validRecoverySession(ctx, req.RecoveryToken, "mfa")
	if err != nil {
		return nil, err
	}
	return s.BeginWebAuthnRegistration(ctx, session.UserID, "", WebAuthnRegistrationOptionsRequest{Label: req.Label})
}

func (s *Service) FinishRecoveryWebAuthn(ctx context.Context, req RecoveryWebAuthnVerifyRequest) (*WebAuthnRegistrationVerifyResponse, error) {
	session, err := s.validRecoverySession(ctx, req.RecoveryToken, "mfa")
	if err != nil {
		return nil, err
	}
	result, err := s.FinishWebAuthnRegistration(ctx, session.UserID, "", WebAuthnRegistrationVerifyRequest{CeremonyID: req.CeremonyID, Credential: req.Credential})
	if err != nil {
		_, _ = s.repo.IncrementRecoverySessionFailure(ctx, session.ID)
		return nil, ErrRecoveryTokenInvalid
	}
	result.RecoveryCodes = nil
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.DeleteSupersededMFAFactors(ctx, session.UserID, result.Factor.ID); err != nil {
			return err
		}
		if err := repo.InvalidateMFABackupCodes(ctx, session.UserID); err != nil {
			return err
		}
		for range s.cfg.MFABackupCodeCount {
			raw, err := security.GenerateBackupCode()
			if err != nil {
				return err
			}
			id, err := utility.GenerateUUID()
			if err != nil {
				return err
			}
			if err := repo.CreateMFABackupCode(ctx, id, session.UserID, security.HashToken(security.NormalizeRecoveryCode(raw))); err != nil {
				return err
			}
			result.RecoveryCodes = append(result.RecoveryCodes, raw)
		}
		if err := repo.CompleteRecoveryMFA(ctx, session.ID); err != nil {
			return err
		}
		if err := repo.RevokeSessionsByUser(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.RevokeRefreshTokensByUser(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.ResetAuthenticationFailures(ctx, session.UserID); err != nil {
			return err
		}
		if err := repo.ResetFailedLoginAttempts(ctx, session.UserID); err != nil {
			return err
		}
		user, err := repo.GetUserByID(ctx, session.UserID)
		if err != nil {
			return err
		}
		body := "Your MFA factors and recovery codes were replaced through account recovery. If this was not you, contact your administrator immediately."
		return repo.QueueEmail(ctx, email.Message{Kind: email.SecurityNotification, To: user.Email, Subject: "Your Nodus Health MFA was recovered", Text: body, HTML: "<p>" + body + "</p>", DedupeKey: "recovery-mfa:" + user.ID + ":" + time.Now().UTC().Format("2006-01-02")})
	})
	if err == nil {
		_ = s.audit.Record(ctx, audit.Entry{TargetUserID: &session.UserID, Action: "account_recovery_mfa_replaced", Result: audit.ResultSuccess})
	}
	return result, err
}
