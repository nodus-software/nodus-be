package auth

import (
	"context"
	"time"

	"nodus-health/internal/email"
)

// Repository persists everything the Auth domain owns: users' credentials
// and lockout state, login challenges, sessions, refresh tokens, password
// reset tokens/attempts, MFA factors and backup codes, and role/permission
// resolution for a user. It intentionally does not own role/permission CRUD
// (that belongs to a future Roles domain) — only read access needed to
// resolve a user's effective permissions.
type Repository interface {
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error)
	LockUser(ctx context.Context, userID string, until time.Time) error
	ResetFailedLoginAttempts(ctx context.Context, userID string) error

	CreateLoginChallenge(ctx context.Context, challenge LoginChallenge) error
	GetLoginChallengeByHash(ctx context.Context, tokenHash string) (*LoginChallenge, error)
	ConsumeLoginChallenge(ctx context.Context, id string) error

	CreateSession(ctx context.Context, session Session) error
	GetSessionByID(ctx context.Context, id string) (*Session, error)
	ListActiveSessionsByUser(ctx context.Context, userID string) ([]Session, error)
	RevokeSession(ctx context.Context, id string) error
	RevokeSessionsByUser(ctx context.Context, userID string) error
	RevokeSessionsByUserExceptSession(ctx context.Context, userID, exceptSessionID string) error
	TouchSessionLastActive(ctx context.Context, id string) error

	CreateRefreshToken(ctx context.Context, token RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string) error
	RevokeRefreshTokensByUser(ctx context.Context, userID string) error
	RevokeRefreshTokensByUserExceptSession(ctx context.Context, userID, exceptSessionID string) error
	RevokeRefreshTokensBySession(ctx context.Context, sessionID string) error

	UpdatePasswordHash(ctx context.Context, userID, hash string) error
	CreatePasswordResetToken(ctx context.Context, token PasswordResetToken) error
	GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	ConsumePasswordResetToken(ctx context.Context, id string) error
	InvalidateOtherPasswordResetTokens(ctx context.Context, userID, exceptID string) error
	RecordPasswordResetAttempt(ctx context.Context, id, usernameAttempted, ip string) error
	CountPasswordResetAttemptsByUsername(ctx context.Context, username string, since time.Time) (int, error)
	CountPasswordResetAttemptsByIP(ctx context.Context, ip string, since time.Time) (int, error)
	QueueEmail(ctx context.Context, message email.Message) error

	CreateMFAFactor(ctx context.Context, factor MFAFactor) (*MFAFactor, error)
	GetMFAFactorByID(ctx context.Context, id string) (*MFAFactor, error)
	ListMFAFactorsByUser(ctx context.Context, userID string) ([]MFAFactor, error)
	ConfirmMFAFactor(ctx context.Context, id string) error
	DeleteMFAFactor(ctx context.Context, id string) error
	CountConfirmedMFAFactors(ctx context.Context, userID string) (int, error)
	CreateMFABackupCode(ctx context.Context, id, userID, codeHash string) error
	GetUnusedMFABackupCodeIDByHash(ctx context.Context, userID, codeHash string) (string, error)
	ConsumeMFABackupCode(ctx context.Context, id string) error
	CountUnusedMFABackupCodes(ctx context.Context, userID string) (int, error)
	InvalidateMFABackupCodes(ctx context.Context, userID string) error
	GetEnrollmentTokenByHash(ctx context.Context, tokenHash string) (id, userID string, expiresAt time.Time, consumed bool, err error)
	ConsumeEnrollmentToken(ctx context.Context, id string) error
	CreateWebAuthnCredential(ctx context.Context, credential WebAuthnCredential) error
	ListWebAuthnCredentialsByUser(ctx context.Context, userID string) ([]WebAuthnCredential, error)
	UpdateWebAuthnCredential(ctx context.Context, userID string, credentialID, credentialJSON []byte) error
	CreateWebAuthnCeremony(ctx context.Context, ceremony WebAuthnCeremony) error
	GetWebAuthnCeremonyByID(ctx context.Context, id string) (*WebAuthnCeremony, error)
	ConsumeWebAuthnCeremony(ctx context.Context, id string) error
	DeletePendingTOTPFactors(ctx context.Context, userID string) error

	GetRolesByUser(ctx context.Context, userID string) ([]Role, error)
	GetEffectivePermissionsByUser(ctx context.Context, userID string) ([]string, error)

	// WithinTx runs fn with a Repository bound to a single database
	// transaction, committing on nil and rolling back otherwise, so a
	// service can compose several of the operations above atomically.
	WithinTx(ctx context.Context, fn func(Repository) error) error
}
