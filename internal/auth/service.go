package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	wa "github.com/go-webauthn/webauthn/webauthn"
	"github.com/pquerna/otp"
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

	TOTPIssuer            string
	MFABackupCodeCount    int
	MFAEncryptionKey      [32]byte
	WebAuthnRPDisplayName string
	WebAuthnRPID          string
	WebAuthnOrigins       []string
	WebAuthnCeremonyTTL   time.Duration

	LockoutMaxAttempts int
	LockoutDuration    time.Duration

	PasswordResetMaxPerUsernamePerHour int
	PasswordResetMaxPerIPPerHour       int

	PasswordPolicy PasswordPolicy
}

type Service struct {
	repo        Repository
	audit       AuditRecorder
	mailer      Mailer
	log         *logger.Logger
	cfg         Config
	webauthn    *wa.WebAuthn
	webauthnErr error
}

func NewService(repo Repository, audit AuditRecorder, mailer Mailer, log *logger.Logger, cfg Config) *Service {
	if cfg.WebAuthnRPDisplayName == "" {
		cfg.WebAuthnRPDisplayName = "Nodus Health"
	}
	if cfg.WebAuthnRPID == "" {
		cfg.WebAuthnRPID = "localhost"
	}
	if len(cfg.WebAuthnOrigins) == 0 {
		cfg.WebAuthnOrigins = []string{"http://localhost:5173"}
	}
	if cfg.WebAuthnCeremonyTTL <= 0 {
		cfg.WebAuthnCeremonyTTL = 5 * time.Minute
	}
	w, werr := wa.New(&wa.Config{RPDisplayName: cfg.WebAuthnRPDisplayName, RPID: cfg.WebAuthnRPID, RPOrigins: cfg.WebAuthnOrigins, AttestationPreference: protocol.PreferNoAttestation, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationRequired}})
	return &Service{repo: repo, audit: audit, mailer: mailer, log: log, cfg: cfg, webauthn: w, webauthnErr: werr}
}

// dummyPasswordHash is compared against on a not-found username so that
// login timing is indistinguishable from a real user with a wrong password,
// mitigating username enumeration via response-time side channel.
var dummyPasswordHash, _ = security.HashPassword("nodus-health-dummy-timing-guard", 10)

// validateTOTP permits one 30-second interval of clock/network drift on each
// side of the current interval. This is the narrow interoperability window
// recommended for human-entered TOTP and still participates in normal
// failed-attempt lockout handling.
func validateTOTP(code, secret string, now time.Time) bool {
	valid, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && valid
}

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
	if remaining, countErr := s.repo.CountUnusedMFABackupCodes(ctx, user.ID); countErr != nil {
		return nil, countErr
	} else if remaining > 0 {
		methods = append(methods, "recovery_code")
	}
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

	var recoveryHash string
	if req.Method == "recovery_code" {
		recoveryHash = security.HashToken(security.NormalizeRecoveryCode(req.Code))
	}
	pair, err := s.establishSession(ctx, user, challenge, ip, userAgent, deviceLabel, req.RememberMe, recoveryHash, now)
	if err != nil {
		if errors.Is(err, ErrMFACodeInvalid) {
			return nil, s.handleFailedAttempt(ctx, user, ip, "mfa_code_invalid")
		}
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &user.ID, Action: "login_success", IPAddress: ip, Result: audit.ResultSuccess,
	})
	return pair, nil
}

func (s *Service) verifyMFACode(ctx context.Context, user *User, req VerifyMFARequest) (bool, error) {
	if req.Method == "recovery_code" {
		canonical := security.NormalizeRecoveryCode(req.Code)
		if len(canonical) != 16 {
			return false, nil
		}
		codeID, err := s.repo.GetUnusedMFABackupCodeIDByHash(ctx, user.ID, security.HashToken(canonical))
		return codeID != "", err
	}
	if req.Method != string(MFAFactorTOTP) || !regexp.MustCompile(`^[0-9]{6}$`).MatchString(req.Code) {
		return false, nil
	}
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
			if validateTOTP(req.Code, secret, time.Now()) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *Service) establishSession(ctx context.Context, user *User, challenge *LoginChallenge, ip, userAgent, deviceLabel string, rememberMe bool, recoveryHash string, now time.Time) (*TokenPairResponse, error) {
	var pair *TokenPairResponse
	err := s.repo.WithinTx(ctx, func(repo Repository) error {
		if recoveryHash != "" {
			codeID, err := repo.GetUnusedMFABackupCodeIDByHash(ctx, user.ID, recoveryHash)
			if err != nil || codeID == "" {
				return ErrMFACodeInvalid
			}
			if err := repo.ConsumeMFABackupCode(ctx, codeID); err != nil {
				return err
			}
		}
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

// SetupTOTP begins enrollment. Recovery credentials are intentionally not
// created until the factor has been cryptographically confirmed.
func (s *Service) SetupTOTP(ctx context.Context, userID string) (*TOTPSetupResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	factors, err := s.repo.ListMFAFactorsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, factor := range factors {
		if factor.Type == MFAFactorTOTP && factor.IsConfirmed() {
			return nil, ErrTOTPAlreadyEnrolled
		}
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
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.DeletePendingTOTPFactors(ctx, userID); err != nil {
			return err
		}
		_, err := repo.CreateMFAFactor(ctx, MFAFactor{ID: factorID, UserID: userID, Type: MFAFactorTOTP, Label: "Authenticator App", SecretEncrypted: &encryptedSecret})
		return err
	})
	if err != nil {
		return nil, err
	}

	return &TOTPSetupResponse{Secret: key.Secret(), QRCodeURI: key.URL()}, nil
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
func (s *Service) ConfirmTOTP(ctx context.Context, userID, code, enrollmentTokenID string) (*ConfirmTOTPResponse, error) {
	factors, err := s.repo.ListMFAFactorsByUser(ctx, userID)
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
		return nil, ErrFactorNotFound
	}

	secret, err := security.DecryptString(s.cfg.MFAEncryptionKey, *pending.SecretEncrypted)
	if err != nil {
		return nil, err
	}
	if !validateTOTP(code, secret, time.Now()) {
		return nil, ErrMFACodeInvalid
	}
	remaining, err := s.repo.CountUnusedMFABackupCodes(ctx, userID)
	if err != nil {
		return nil, err
	}
	response := &ConfirmTOTPResponse{Status: "enabled"}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConfirmMFAFactor(ctx, pending.ID); err != nil {
			return err
		}
		if remaining == 0 {
			for range s.cfg.MFABackupCodeCount {
				raw, err := security.GenerateBackupCode()
				if err != nil {
					return err
				}
				id, err := utility.GenerateUUID()
				if err != nil {
					return err
				}
				canonical := security.NormalizeRecoveryCode(raw)
				if err := repo.CreateMFABackupCode(ctx, id, userID, security.HashToken(canonical)); err != nil {
					return err
				}
				response.RecoveryCodes = append(response.RecoveryCodes, raw)
			}
		}
		if enrollmentTokenID != "" {
			return repo.ConsumeEnrollmentToken(ctx, enrollmentTokenID)
		}
		return nil
	})
	return response, err
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
		if !f.IsConfirmed() {
			continue
		}
		resp = append(resp, MFAFactorResponse{ID: f.ID, Type: string(f.Type), Label: f.Label, CreatedAt: f.CreatedAt})
	}
	return resp, nil
}

// RemoveFactor deletes an MFA factor, refusing to remove the last confirmed
// factor a user has.
func (s *Service) RemoveFactor(ctx context.Context, userID, factorID, currentPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !security.ComparePassword(user.PasswordHash, currentPassword) {
		return ErrCurrentPasswordInvalid
	}
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
	return s.repo.WithinTx(ctx, func(repo Repository) error {
		count, err := repo.CountConfirmedMFAFactors(ctx, userID)
		if err != nil {
			return err
		}
		if factor.IsConfirmed() && count <= 1 {
			return ErrLastFactorRemaining
		}
		return repo.DeleteMFAFactor(ctx, factorID)
	})
}

func (s *Service) RecoveryCodeStatus(ctx context.Context, userID string) (*RecoveryCodeStatusResponse, error) {
	n, err := s.repo.CountUnusedMFABackupCodes(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &RecoveryCodeStatusResponse{Remaining: n, Generated: n > 0}, nil
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID string, req RegenerateRecoveryCodesRequest) (*RecoveryCodesResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !security.ComparePassword(user.PasswordHash, req.CurrentPassword) {
		return nil, ErrCurrentPasswordInvalid
	}
	resp := &RecoveryCodesResponse{Remaining: s.cfg.MFABackupCodeCount}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.InvalidateMFABackupCodes(ctx, userID); err != nil {
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
			if err := repo.CreateMFABackupCode(ctx, id, userID, security.HashToken(security.NormalizeRecoveryCode(raw))); err != nil {
				return err
			}
			resp.RecoveryCodes = append(resp.RecoveryCodes, raw)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{UserID: &userID, Action: "mfa_recovery_codes_regenerated", Result: audit.ResultSuccess})
	if err := s.mailer.Send(ctx, user.Email, "Your Nodus Health recovery codes were replaced", "Your MFA recovery codes were replaced. If this was not you, contact your administrator immediately."); err != nil {
		s.log.Error("failed recovery-code notification", "error", err.Error())
	}
	return resp, nil
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
