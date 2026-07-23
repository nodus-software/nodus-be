package auth

import "time"

type UserStatus string

const (
	UserStatusInvited       UserStatus = "invited"
	UserStatusActive        UserStatus = "active"
	UserStatusSuspended     UserStatus = "suspended"
	UserStatusPendingReview UserStatus = "pending_review"
)

type User struct {
	ID                  string
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
	MFAFactorTOTP      MFAFactorType = "totp"
	MFAFactorBiometric MFAFactorType = "biometric"
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
	UserID       string
	DeviceLabel  string
	IPAddress    string
	UserAgent    string
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
