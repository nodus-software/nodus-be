package invitation

import "time"

type UserStatus string

const (
	UserStatusInvited     UserStatus = "invited"
	UserStatusActive      UserStatus = "active"
	UserStatusSuspended   UserStatus = "suspended"
	UserStatusDeactivated UserStatus = "deactivated"
)

// Invitation is the single-use, hashed-at-rest token issued when an admin
// invites a new staff user. The raw token is only ever handed to the
// invitee out-of-band (email) — only its hash is persisted.
type Invitation struct {
	ID        string
	UserID    string
	InvitedBy string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (i *Invitation) IsExpired(now time.Time) bool { return now.After(i.ExpiresAt) }
func (i *Invitation) IsUsed() bool                 { return i.UsedAt != nil }

// EnrollmentToken is issued after an invite is accepted, scoped only to
// completing MFA setup — deliberately not a real session/refresh token.
type EnrollmentToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
}

type ReactivationToken struct {
	ID          string
	UserID      string
	RequestedBy string
	TokenHash   string
	ExpiresAt   time.Time
	UsedAt      *time.Time
	CreatedAt   time.Time
}

func (t *ReactivationToken) IsExpired(now time.Time) bool { return now.After(t.ExpiresAt) }
func (t *ReactivationToken) IsUsed() bool                 { return t.UsedAt != nil }

// Role is the subset of role data this domain needs to validate role_ids
// on invite: existence, and whether the role requires the invitee carry a
// provider identifier (Reg. 12(b)(v)).
type Role struct {
	ID                         string
	Name                       string
	RequiresProviderIdentifier bool
}

// PendingUser is the minimal user shape this domain reads: enough to
// preview/accept an invitation or resend one, without owning the user's
// credentials or full profile (that belongs to the Users/Auth domains).
type PendingUser struct {
	ID                 string
	FullName           string
	Email              string
	ProviderIdentifier *string
	Status             UserStatus
	PasswordSet        bool
}

// CreateInvitedUserParams creates the pending user record an invitation
// points at. No password exists yet — the invitee sets one on accept.
type CreateInvitedUserParams struct {
	ID                 string
	FullName           string
	Username           string
	Email              string
	ProviderIdentifier *string
}
