package users

import "time"

type Status string

const (
	StatusInvited       Status = "invited"
	StatusActive        Status = "active"
	StatusSuspended     Status = "suspended"
	StatusPendingReview Status = "pending_review"
	StatusDeactivated   Status = "deactivated"
)

// Role is the subset of a role's shape this domain needs to validate a
// role assignment: whether it's superuser-restricted and whether it
// requires the assignee to carry a provider identifier.
type Role struct {
	ID                         string
	Name                       string
	IsSuperuserRole            bool
	RequiresProviderIdentifier bool
}

type User struct {
	ID                  string
	TenantID            string
	FullName            string
	Username            string
	Email               string
	ProviderIdentifier  *string
	Status              Status
	LockedUntil         *time.Time
	LastAccessReviewAt  *time.Time
	NextAccessReviewDue *time.Time
	DeactivatedAt       *time.Time
	InvitationExpiresAt *time.Time
	InvitationUsedAt    *time.Time
	MFAEnrolled         bool
	RoleNames           []string
	Permissions         []string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (u *User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}
