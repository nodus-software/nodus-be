package auth

import "time"

type AuthenticationMechanism string

const (
	AuthenticationMechanismPassword AuthenticationMechanism = "password"
	AuthenticationMechanismMFA      AuthenticationMechanism = "mfa"
	AuthenticationMechanismRecovery AuthenticationMechanism = "recovery"
	AuthenticationMechanismCaptcha  AuthenticationMechanism = "captcha"
)

type AuthenticationFailureState struct {
	Mechanism        AuthenticationMechanism
	FailureCount     int
	WindowStartedAt  time.Time
	LastFailureAt    time.Time
	NextAttemptAt    *time.Time
	LockedUntil      *time.Time
	LockCycleCount   int
	CycleWindowStart *time.Time
}

type UserStatus string

const (
	UserStatusInvited       UserStatus = "invited"
	UserStatusActive        UserStatus = "active"
	UserStatusSuspended     UserStatus = "suspended"
	UserStatusPendingReview UserStatus = "pending_review"
)

type User struct {
	ID                  string
	TenantID            string
	FullName            string
	Username            string
	Email               string
	PasswordHash        string
	ProviderIdentifier  *string
	Status              UserStatus
	FailedLoginAttempts int
	LockedUntil         *time.Time
	PasswordChangedAt   time.Time
	LastAccessReviewAt  *time.Time
	NextAccessReviewDue *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (u *User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

type Role struct {
	ID                         string
	Name                       string
	Description                string
	IsSuperuserRole            bool
	RequiresProviderIdentifier bool
}

type MFAFactorType string

const (
	MFAFactorTOTP     MFAFactorType = "totp"
	MFAFactorWebAuthn MFAFactorType = "webauthn"
)

type MFAFactor struct {
	ID              string
	UserID          string
	Type            MFAFactorType
	Label           string
	SecretEncrypted *string
	PublicKey       *string
	ConfirmedAt     *time.Time
	CreatedAt       time.Time
}

type WebAuthnCredential struct {
	ID, UserID, FactorID string
	CredentialID         []byte
	CredentialJSON       []byte
	CreatedAt            time.Time
}

type WebAuthnCeremony struct {
	ID, UserID, Purpose, Label          string
	LoginChallengeID, EnrollmentTokenID *string
	SessionData                         []byte
	ExpiresAt                           time.Time
	ConsumedAt                          *time.Time
}

func (f *MFAFactor) IsConfirmed() bool { return f.ConfirmedAt != nil }

type LoginChallenge struct {
	ID                 string
	UserID             string
	ChallengeTokenHash string
	ExpiresAt          time.Time
	ConsumedAt         *time.Time
	CreatedAt          time.Time
}

func (c *LoginChallenge) IsExpired(now time.Time) bool { return now.After(c.ExpiresAt) }
func (c *LoginChallenge) IsConsumed() bool             { return c.ConsumedAt != nil }

type Session struct {
	ID           string
	TenantID     string
	UserID       string
	DeviceLabel  string
	IPAddress    string
	UserAgent    string
	RememberMe   bool
	CreatedAt    time.Time
	LastActiveAt time.Time
	RevokedAt    *time.Time
}

func (s *Session) IsRevoked() bool { return s.RevokedAt != nil }

type RefreshToken struct {
	ID        string
	SessionID string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (t *RefreshToken) IsValid(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type RecoveryIntent string

const (
	RecoveryIntentPassword RecoveryIntent = "password"
	RecoveryIntentMFA      RecoveryIntent = "mfa"
	RecoveryIntentBoth     RecoveryIntent = "both"
)

type RecoveryEmailToken struct {
	ID, UserID string
	Intent     RecoveryIntent
	ExpiresAt  time.Time
}

type RecoverySession struct {
	ID, UserID                                      string
	CanResetPassword, CanReplaceMFA                 bool
	PasswordCompletedAt, MFACompletedAt, ConsumedAt *time.Time
	FailedAttempts                                  int
	ExpiresAt                                       time.Time
}
