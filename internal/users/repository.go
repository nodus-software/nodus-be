package users

import (
	"context"
	"time"
)

// Repository persists the admin-facing view of a user: listing/filtering,
// role assignment, status, provider identifier, access review bookkeeping,
// and lockout clearing. It does not own credentials or session state —
// those belong to the Auth domain.
type Repository interface {
	ListUsers(ctx context.Context, filter ListUsersFilter) ([]User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	ReplaceUserRoles(ctx context.Context, userID string, roleIDs []string) error
	UpdateUserStatus(ctx context.Context, userID, status string) error
	SetProviderIdentifier(ctx context.Context, userID, identifier string) error
	RecordAccessReview(ctx context.Context, userID string, reviewedAt, nextDue time.Time) error
	UnlockUser(ctx context.Context, userID string) error
	GetRolesByIDs(ctx context.Context, ids []string) ([]Role, error)
	HasSuperuserRole(ctx context.Context, userID string) (bool, error)

	// WithinTx runs fn with a Repository bound to a single database
	// transaction, committing on nil and rolling back otherwise.
	WithinTx(ctx context.Context, fn func(Repository) error) error
}
