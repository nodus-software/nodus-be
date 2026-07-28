package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/pquerna/otp/totp"

	"nodus-health/internal/audit"
	"nodus-health/internal/tenant"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

// AuditRecorder is the narrow slice of the audit domain this service depends
// on. Defined here (the consumer), not in the audit package, per Go
// convention.
type AuditRecorder interface {
	Record(ctx context.Context, entry audit.Entry) error
}

type PasswordPolicy struct {
	MinLength             int
	RequireUppercase      bool
	RequireNumber         bool
	RequireSymbol         bool
	RejectCommonPasswords bool
	MaxAgeDays            int
}

// Config is the subset of application configuration the auth service needs.
// Defined in this package (rather than depending on the top-level config
// package) so the domain stays decoupled from how the app is wired together;
// cmd/api/main.go maps config.Config into this shape.
type Config struct {
	BaseURL string

	JWTSecret              string
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
	SessionRefreshTokenTTL time.Duration
	ChallengeTokenTTL      time.Duration
	PasswordResetTokenTTL  time.Duration

	BcryptCost int

	TOTPIssuer         string
	MFABackupCodeCount int
	MFAEncryptionKey   [32]byte

	LockoutMaxAttempts int
	LockoutDuration    time.Duration

	PasswordResetMaxPerUsernamePerHour int
	PasswordResetMaxPerIPPerHour       int

	PasswordPolicy PasswordPolicy
}

type Service struct {
	repo   Repository
	audit  AuditRecorder
	mailer Mailer
	log    *logger.Logger
	cfg    Config
}

func NewService(repo Repository, audit AuditRecorder, mailer Mailer, log *logger.Logger, cfg Config) *Service {
	return &Service{repo: repo, audit: audit, mailer: mailer, log: log, cfg: cfg}
}

// dummyPasswordHash is compared against on a not-found username so that
// login timing is indistinguishable from a real user with a wrong password,
// mitigating username enumeration via response-time side channel.
var dummyPasswordHash, _ = security.HashPassword("nodus-health-dummy-timing-guard", 10)

func ptr(s string) *string { return &s }

// Login is step 1: validate credentials and, if valid, issue an MFA
// challenge. It never establishes a session by itself.
func (s *Service) Login(ctx context.Context, req LoginRequest, ip string) (*LoginChallengeResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
		security.ComparePassword(dummyPasswordHash, req.Password)
		s.recordLoginFailure(ctx, nil, ip, "user_not_found")
		return nil, ErrInvalidCredentials
	}

	now := time.Now()
	if user.IsLocked(now) {
		s.recordLoginFailure(ctx, &user.ID, ip, "account_locked")
		return nil, &LockedError{LockedUntil: *user.LockedUntil}
	}

	if user.Status != UserStatusActive {
		s.recordLoginFailure(ctx, &user.ID, ip, "account_not_active")
		return nil, ErrInvalidCredentials
	}

	if !security.ComparePassword(user.PasswordHash, req.Password) {
		return nil, s.handleFailedAttempt(ctx, user, ip, "bad_password")
	}

	factors, err := s.repo.ListMFAFactorsByUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	methods := confirmedFactorMethods(factors)
	if len(methods) == 0 {
		s.recordLoginFailure(ctx, &user.ID, ip, "mfa_not_enrolled")
		return nil, ErrMFANotEnrolled
	}

	rawChallenge, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	challengeID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateLoginChallenge(ctx, LoginChallenge{
		ID:                 challengeID,
		UserID:             user.ID,
		ChallengeTokenHash: security.HashToken(rawChallenge),
		ExpiresAt:          now.Add(s.cfg.ChallengeTokenTTL),
	}); err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &user.ID, Action: "login_credentials_verified",
		IPAddress: ip, Result: audit.ResultSuccess,
	})

	return &LoginChallengeResponse{ChallengeToken: rawChallenge, MFAMethods: methods}, nil
}

func (s *Service) handleFailedAttempt(ctx context.Context, user *User, ip, reason string) error {
	attempts, err := s.repo.IncrementFailedLoginAttempts(ctx, user.ID)
	if err != nil {
		return err
	}
	if attempts >= s.cfg.LockoutMaxAttempts {
		lockedUntil := time.Now().Add(s.cfg.LockoutDuration)
		if err := s.repo.LockUser(ctx, user.ID, lockedUntil); err != nil {
			return err
		}
		s.recordLoginFailure(ctx, &user.ID, ip, reason+"_locked_out")
		return &LockedError{LockedUntil: lockedUntil}
	}
	s.recordLoginFailure(ctx, &user.ID, ip, reason)
	return ErrInvalidCredentials
}

func (s *Service) recordLoginFailure(ctx context.Context, userID *string, ip, reason string) {
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: userID, Action: "login_failed", IPAddress: ip,
		Result: audit.ResultFailure, Metadata: map[string]any{"reason": reason},
	})
}

func confirmedFactorMethods(factors []MFAFactor) []string {
	seen := map[MFAFactorType]bool{}
	var methods []string
	for _, f := range factors {
		if f.IsConfirmed() && !seen[f.Type] {
			seen[f.Type] = true
			methods = append(methods, string(f.Type))
		}
	}
	return methods
}

// VerifyMFA is step 2: verify the MFA code against the challenge and, on
// success, establish a real session.
func (s *Service) VerifyMFA(ctx context.Context, req VerifyMFARequest, ip, userAgent, deviceLabel string) (*TokenPairResponse, error) {
	challenge, err := s.repo.GetLoginChallengeByHash(ctx, security.HashToken(req.ChallengeToken))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if challenge.IsConsumed() {
		return nil, ErrChallengeInvalid
	}
	if challenge.IsExpired(now) {
		return nil, ErrChallengeExpired
	}

	user, err := s.repo.GetUserByID(ctx, challenge.UserID)
	if err != nil {
		return nil, err
	}

	valid, err := s.verifyMFACode(ctx, user, req)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, s.handleFailedAttempt(ctx, user, ip, "mfa_code_invalid")
	}

	pair, err := s.establishSession(ctx, user, challenge, ip, userAgent, deviceLabel, req.RememberMe, now)
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &user.ID, Action: "login_success", IPAddress: ip, Result: audit.ResultSuccess,
	})
	return pair, nil
}

func (s *Service) verifyMFACode(ctx context.Context, user *User, req VerifyMFARequest) (bool, error) {
	factors, err := s.repo.ListMFAFactorsByUser(ctx, user.ID)
	if err != nil {
		return false, err
	}

	for _, f := range factors {
		if !f.IsConfirmed() || string(f.Type) != req.Method {
			continue
		}
		switch f.Type {
		case MFAFactorTOTP:
			secret, err := security.DecryptString(s.cfg.MFAEncryptionKey, *f.SecretEncrypted)
			if err != nil {
				return false, err
			}
			if totp.Validate(req.Code, secret) {
				return true, nil
			}
		case MFAFactorBiometric:
			if verifyBiometricAssertion(*f.PublicKey, req.ChallengeToken, req.Code) {
				return true, nil
			}
		}
	}

	if req.Method == string(MFAFactorTOTP) {
		codeID, err := s.repo.GetUnusedMFABackupCodeIDByHash(ctx, user.ID, security.HashToken(req.Code))
		if err == nil && codeID != "" {
			if err := s.repo.ConsumeMFABackupCode(ctx, codeID); err != nil {
				return false, err
			}
			return true, nil
		}
	}

	return false, nil
}

func (s *Service) establishSession(ctx context.Context, user *User, challenge *LoginChallenge, ip, userAgent, deviceLabel string, rememberMe bool, now time.Time) (*TokenPairResponse, error) {
	var pair *TokenPairResponse
	err := s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConsumeLoginChallenge(ctx, challenge.ID); err != nil {
			return err
		}
		if err := repo.ResetFailedLoginAttempts(ctx, user.ID); err != nil {
			return err
		}

		sessionID, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		if err := repo.CreateSession(ctx, Session{
			ID: sessionID, UserID: user.ID, DeviceLabel: deviceLabel, IPAddress: ip, UserAgent: userAgent,
			RememberMe: rememberMe,
		}); err != nil {
			return err
		}

		rawRefresh, err := security.GenerateToken()
		if err != nil {
			return err
		}
		refreshID, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		refreshTTL := s.cfg.SessionRefreshTokenTTL
		if rememberMe {
			refreshTTL = s.cfg.RefreshTokenTTL
		}
		refreshExpiresAt := now.Add(refreshTTL)
		if err := repo.CreateRefreshToken(ctx, RefreshToken{
			ID: refreshID, SessionID: sessionID, UserID: user.ID,
			TokenHash: security.HashToken(rawRefresh), ExpiresAt: refreshExpiresAt,
		}); err != nil {
			return err
		}

		tenantID, _ := tenant.ID(ctx)
		accessToken, accessExpiry, err := security.IssueAccessToken(s.cfg.JWTSecret, s.cfg.AccessTokenTTL, user.ID, sessionID, tenantID)
		if err != nil {
			return err
		}

		pair = &TokenPairResponse{
			AccessToken: accessToken, RefreshToken: rawRefresh, RefreshExpiresAt: refreshExpiresAt,
			RememberMe: rememberMe, ExpiresIn: int(time.Until(accessExpiry).Seconds()),
		}
		return nil
	})
	return pair, err
}

// verifyBiometricAssertion checks an Ed25519 signature over the challenge
// token using the registered device public key. This proves possession of
// the private key that produced the registered public key, but — unlike a
// full WebAuthn ceremony — does not bind to an RP ID/origin or verify
// attestation. See the biometric MFA caveat in the project plan.
func verifyBiometricAssertion(publicKeyB64, challengeToken, signatureB64 string) bool {
	pubKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false
	}
	return ed25519.Verify(pubKey, []byte(challengeToken), sig)
}

// Refresh exchanges a valid refresh token for a new access/refresh pair,
// rotating the refresh token.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPairResponse, error) {
	if rawRefreshToken == "" {
		return nil, ErrRefreshTokenInvalid
	}
	token, err := s.repo.GetRefreshTokenByHash(ctx, security.HashToken(rawRefreshToken))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if token.RevokedAt != nil {
		return nil, ErrRefreshTokenRevoked
	}
	if !now.Before(token.ExpiresAt) {
		return nil, ErrRefreshTokenInvalid
	}
	session, err := s.repo.GetSessionByID(ctx, token.SessionID)
	if err != nil || session.UserID != token.UserID || session.IsRevoked() {
		return nil, ErrRefreshTokenInvalid
	}
	user, err := s.repo.GetUserByID(ctx, token.UserID)
	if err != nil || user.Status != UserStatusActive {
		return nil, ErrRefreshTokenInvalid
	}

	var pair *TokenPairResponse
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.RevokeRefreshToken(ctx, token.ID); err != nil {
			return err
		}

		newRaw, err := security.GenerateToken()
		if err != nil {
			return err
		}
		newID, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		refreshTTL := s.cfg.SessionRefreshTokenTTL
		if session.RememberMe {
			refreshTTL = s.cfg.RefreshTokenTTL
		}
		refreshExpiresAt := now.Add(refreshTTL)
		if err := repo.CreateRefreshToken(ctx, RefreshToken{
			ID: newID, SessionID: token.SessionID, UserID: token.UserID,
			TokenHash: security.HashToken(newRaw), ExpiresAt: refreshExpiresAt,
		}); err != nil {
			return err
		}
		if err := repo.TouchSessionLastActive(ctx, token.SessionID); err != nil {
			return err
		}

		tenantID, _ := tenant.ID(ctx)
		access, expiry, err := security.IssueAccessToken(s.cfg.JWTSecret, s.cfg.AccessTokenTTL, token.UserID, token.SessionID, tenantID)
		if err != nil {
			return err
		}
		pair = &TokenPairResponse{AccessToken: access, RefreshToken: newRaw, RefreshExpiresAt: refreshExpiresAt, RememberMe: session.RememberMe, ExpiresIn: int(time.Until(expiry).Seconds())}
		return nil
	})
	return pair, err
}

// Logout revokes the current session and its refresh token.
func (s *Service) Logout(ctx context.Context, userID, sessionID, ip string) error {
	err := s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.RevokeSession(ctx, sessionID); err != nil {
			return err
		}
		return repo.RevokeRefreshTokensBySession(ctx, sessionID)
	})
	if err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Entry{UserID: &userID, Action: "logout", IPAddress: ip, Result: audit.ResultSuccess})
	return nil
}

// Me resolves the authenticated user's profile and effective permissions.
func (s *Service) Me(ctx context.Context, userID string) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.GetRolesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Superuser roles carry no explicit role_permissions rows — they're authorized via
	// the "*" wildcard (see Authorize) instead. Mirror that here so the profile reflects
	// the same effective access as the rest of the API, not an empty permissions list.
	var perms []string
	if slices.ContainsFunc(roles, func(r Role) bool { return r.IsSuperuserRole }) {
		perms = []string{"*"}
	} else {
		perms, err = s.repo.GetEffectivePermissionsByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	factors, err := s.repo.ListMFAFactorsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		roleNames = append(roleNames, r.Name)
	}

	mfaEnrolled := false
	for _, f := range factors {
		if f.IsConfirmed() {
			mfaEnrolled = true
			break
		}
	}

	return &UserProfileResponse{
		ID: user.ID, TenantID: user.TenantID, FullName: user.FullName, Username: user.Username, Email: user.Email,
		ProviderIdentifier: user.ProviderIdentifier, Roles: roleNames, Permissions: perms,
		Status: string(user.Status), MFAEnrolled: mfaEnrolled,
		LastAccessReviewAt: user.LastAccessReviewAt, NextAccessReviewDue: user.NextAccessReviewDue,
	}, nil
}

// SetupTOTP begins TOTP enrollment: generates a secret, stores it encrypted
// as an unconfirmed factor, and returns one-time backup codes.
func (s *Service) SetupTOTP(ctx context.Context, userID string) (*TOTPSetupResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.cfg.TOTPIssuer, AccountName: user.Username})
	if err != nil {
		return nil, fmt.Errorf("generate totp key: %w", err)
	}

	encryptedSecret, err := security.EncryptString(s.cfg.MFAEncryptionKey, key.Secret())
	if err != nil {
		return nil, err
	}

	factorID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.CreateMFAFactor(ctx, MFAFactor{
		ID: factorID, UserID: userID, Type: MFAFactorTOTP, Label: "Authenticator App",
		SecretEncrypted: &encryptedSecret,
	}); err != nil {
		return nil, err
	}

	backupCodes := make([]string, 0, s.cfg.MFABackupCodeCount)
	for range s.cfg.MFABackupCodeCount {
		raw, err := security.GenerateBackupCode()
		if err != nil {
			return nil, err
		}
		codeID, err := utility.GenerateUUID()
		if err != nil {
			return nil, err
		}
		if err := s.repo.CreateMFABackupCode(ctx, codeID, userID, security.HashToken(raw)); err != nil {
			return nil, err
		}
		backupCodes = append(backupCodes, raw)
	}

	return &TOTPSetupResponse{Secret: key.Secret(), QRCodeURI: key.URL(), BackupCodes: backupCodes}, nil
}

func (s *Service) ResolveEnrollmentToken(ctx context.Context, rawToken string) (string, string, error) {
	id, userID, expiresAt, consumed, err := s.repo.GetEnrollmentTokenByHash(ctx, security.HashToken(rawToken))
	if err != nil || consumed || !expiresAt.After(time.Now()) {
		return "", "", ErrEnrollmentTokenInvalid
	}
	return id, userID, nil
}

func (s *Service) ConsumeEnrollmentToken(ctx context.Context, id string) error {
	return s.repo.ConsumeEnrollmentToken(ctx, id)
}

// ConfirmTOTP verifies the initial code and activates the pending factor.
func (s *Service) ConfirmTOTP(ctx context.Context, userID, code string) error {
	factors, err := s.repo.ListMFAFactorsByUser(ctx, userID)
	if err != nil {
		return err
	}

	var pending *MFAFactor
	for i := range factors {
		if factors[i].Type == MFAFactorTOTP && !factors[i].IsConfirmed() {
			pending = &factors[i]
			break
		}
	}
	if pending == nil {
		return ErrFactorNotFound
	}

	secret, err := security.DecryptString(s.cfg.MFAEncryptionKey, *pending.SecretEncrypted)
	if err != nil {
		return err
	}
	if !totp.Validate(code, secret) {
		return ErrMFACodeInvalid
	}
	return s.repo.ConfirmMFAFactor(ctx, pending.ID)
}

// RegisterBiometric stores a device public key as an immediately-active
// secondary MFA factor (no separate confirm step, per the contract).
func (s *Service) RegisterBiometric(ctx context.Context, userID string, req RegisterBiometricRequest) (*MFAFactor, error) {
	pubKey, err := base64.StdEncoding.DecodeString(req.DevicePublicKey)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		return nil, ErrInvalidPublicKey
	}

	factorID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return s.repo.CreateMFAFactor(ctx, MFAFactor{
		ID: factorID, UserID: userID, Type: MFAFactorBiometric,
		Label: req.DeviceLabel, PublicKey: ptr(req.DevicePublicKey), ConfirmedAt: &now,
	})
}

// ListFactors returns every enrolled MFA factor (confirmed or pending) for
// the user.
func (s *Service) ListFactors(ctx context.Context, userID string) ([]MFAFactorResponse, error) {
	factors, err := s.repo.ListMFAFactorsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	resp := make([]MFAFactorResponse, 0, len(factors))
	for _, f := range factors {
		resp = append(resp, MFAFactorResponse{ID: f.ID, Type: string(f.Type), Label: f.Label, CreatedAt: f.CreatedAt})
	}
	return resp, nil
}

// RemoveFactor deletes an MFA factor, refusing to remove the last confirmed
// factor a user has.
func (s *Service) RemoveFactor(ctx context.Context, userID, factorID string) error {
	factor, err := s.repo.GetMFAFactorByID(ctx, factorID)
	if err != nil {
		if errors.Is(err, ErrFactorNotFound) {
			return ErrFactorNotFound
		}
		return err
	}
	if factor.UserID != userID {
		return ErrFactorNotFound
	}

	if factor.IsConfirmed() {
		count, err := s.repo.CountConfirmedMFAFactors(ctx, userID)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastFactorRemaining
		}
	}
	return s.repo.DeleteMFAFactor(ctx, factorID)
}

// ChangePassword verifies the current password, enforces the complexity
// policy on the new one, and revokes every other session.
func (s *Service) ChangePassword(ctx context.Context, userID, sessionID string, req ChangePasswordRequest) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !security.ComparePassword(user.PasswordHash, req.CurrentPassword) {
		return ErrCurrentPasswordInvalid
	}
	if violations := ValidatePasswordPolicy(req.NewPassword, s.cfg.PasswordPolicy); len(violations) > 0 {
		return &PolicyViolationError{Violations: violations}
	}

	hash, err := security.HashPassword(req.NewPassword, s.cfg.BcryptCost)
	if err != nil {
		return err
	}

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.UpdatePasswordHash(ctx, userID, hash); err != nil {
			return err
		}
		if err := repo.RevokeSessionsByUserExceptSession(ctx, userID, sessionID); err != nil {
			return err
		}
		return repo.RevokeRefreshTokensByUserExceptSession(ctx, userID, sessionID)
	})
	if err != nil {
		return err
	}

	_ = s.audit.Record(ctx, audit.Entry{UserID: &userID, Action: "password_changed", Result: audit.ResultSuccess})
	return nil
}

// GetPasswordPolicy returns the currently configured complexity policy.
func (s *Service) GetPasswordPolicy() PasswordPolicyResponse {
	p := s.cfg.PasswordPolicy
	return PasswordPolicyResponse{
		MinLength: p.MinLength, RequireUppercase: p.RequireUppercase, RequireNumber: p.RequireNumber,
		RequireSymbol: p.RequireSymbol, RejectCommonPasswords: p.RejectCommonPasswords, MaxAgeDays: p.MaxAgeDays,
	}
}

// RequestPasswordReset always records the attempt (for audit + rate
// limiting) and, unless rate-limited, returns nil regardless of whether the
// username exists — never revealing account existence to the caller.
func (s *Service) RequestPasswordReset(ctx context.Context, req RequestPasswordResetRequest, ip string) error {
	now := time.Now()
	windowStart := now.Add(-time.Hour)

	usernameCount, err := s.repo.CountPasswordResetAttemptsByUsername(ctx, req.Username, windowStart)
	if err != nil {
		return err
	}
	ipCount, err := s.repo.CountPasswordResetAttemptsByIP(ctx, ip, windowStart)
	if err != nil {
		return err
	}

	attemptID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	if err := s.repo.RecordPasswordResetAttempt(ctx, attemptID, req.Username, ip); err != nil {
		return err
	}

	rateLimited := usernameCount >= s.cfg.PasswordResetMaxPerUsernamePerHour || ipCount >= s.cfg.PasswordResetMaxPerIPPerHour
	result := audit.ResultSuccess
	if rateLimited {
		result = audit.ResultFailure
	}
	_ = s.audit.Record(ctx, audit.Entry{
		Action: "password_reset_requested", IPAddress: ip, Result: result,
		Metadata: map[string]any{"username": req.Username, "rate_limited": rateLimited},
	})

	if rateLimited {
		return ErrRateLimitExceeded
	}

	user, err := s.repo.GetUserByUsername(ctx, req.Username)
	if err != nil {
		return nil
	}

	rawToken, err := security.GenerateToken()
	if err != nil {
		return err
	}
	tokenID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	if err := s.repo.CreatePasswordResetToken(ctx, PasswordResetToken{
		ID: tokenID, UserID: user.ID, TokenHash: security.HashToken(rawToken),
		ExpiresAt: now.Add(s.cfg.PasswordResetTokenTTL),
	}); err != nil {
		return err
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.cfg.BaseURL, rawToken)
	if err := s.mailer.Send(ctx, user.Email, "Reset your Nodus Health password", resetLink); err != nil {
		s.log.Error("failed to send password reset email", "error", err.Error())
	}
	return nil
}

// ConfirmPasswordReset completes a reset using the out-of-band token. The
// token is invalidated on first use regardless of whether this attempt
// ultimately succeeds, to prevent replay.
func (s *Service) ConfirmPasswordReset(ctx context.Context, req ConfirmPasswordResetRequest) error {
	token, err := s.repo.GetPasswordResetTokenByHash(ctx, security.HashToken(req.ResetToken))
	if err != nil {
		return err
	}
	if token.UsedAt != nil {
		return ErrResetTokenInvalid
	}

	now := time.Now()
	if now.After(token.ExpiresAt) {
		_ = s.repo.ConsumePasswordResetToken(ctx, token.ID)
		return ErrResetTokenInvalid
	}

	// Burn the token now — first use, whether or not what follows succeeds.
	if err := s.repo.ConsumePasswordResetToken(ctx, token.ID); err != nil {
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
		if err := repo.UpdatePasswordHash(ctx, token.UserID, hash); err != nil {
			return err
		}
		if err := repo.RevokeSessionsByUser(ctx, token.UserID); err != nil {
			return err
		}
		if err := repo.RevokeRefreshTokensByUser(ctx, token.UserID); err != nil {
			return err
		}
		return repo.InvalidateOtherPasswordResetTokens(ctx, token.UserID, token.ID)
	})
	if err != nil {
		return err
	}

	_ = s.audit.Record(ctx, audit.Entry{UserID: &token.UserID, Action: "password_reset_completed", Result: audit.ResultSuccess})
	return nil
}

// ListSessions lists the user's active sessions, flagging which one issued
// the current request.
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionResponse, error) {
	sessions, err := s.repo.ListActiveSessionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	resp := make([]SessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		resp = append(resp, SessionResponse{
			ID: sess.ID, TenantID: sess.TenantID, DeviceLabel: sess.DeviceLabel, IPAddress: sess.IPAddress,
			CreatedAt: sess.CreatedAt, LastActiveAt: sess.LastActiveAt, Current: sess.ID == currentSessionID,
		})
	}
	return resp, nil
}

// Authorize implements middleware.Authorizer: it checks that the session is
// still active and belongs to the given user, that the user is still
// active, and returns the user's current effective permissions. Called on
// every authenticated request so a revoked session or role change takes
// effect immediately.
func (s *Service) Authorize(ctx context.Context, userID, sessionID string) ([]string, error) {
	sess, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil || sess.UserID != userID || sess.IsRevoked() {
		return nil, ErrSessionNotFound
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil || user.Status != UserStatusActive {
		return nil, ErrUserNotFound
	}
	roles, err := s.repo.GetRolesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		if role.IsSuperuserRole {
			return []string{"*"}, nil
		}
	}

	return s.repo.GetEffectivePermissionsByUser(ctx, userID)
}

// RevokeSession revokes one specific session belonging to the user.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID, ip string) error {
	sess, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ErrSessionNotFound
		}
		return err
	}
	if sess.UserID != userID || sess.IsRevoked() {
		return ErrSessionNotFound
	}

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.RevokeSession(ctx, sessionID); err != nil {
			return err
		}
		return repo.RevokeRefreshTokensBySession(ctx, sessionID)
	})
	if err != nil {
		return err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &userID, Action: "session_revoked", IPAddress: ip,
		Result: audit.ResultSuccess, TargetResource: sessionID,
	})
	return nil
}
