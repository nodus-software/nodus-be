package invitation

import "context"

// Repository persists everything the Invitation domain owns: the pending
// user record created by an invite, role assignment at invite time,
// invitation tokens, and the enrollment token issued on accept. It reads
// (but does not own) the roles table, the same way the Auth domain reads
// roles/permissions without owning their CRUD.
type Repository interface {
	GetRolesByIDs(ctx context.Context, ids []string) ([]Role, error)
	GetUserByEmail(ctx context.Context, tenantID, email string) (*PendingUser, error)
	GetUserByID(ctx context.Context, id string) (*PendingUser, error)
	CreateInvitedUser(ctx context.Context, params CreateInvitedUserParams) error
	AssignUserRole(ctx context.Context, userID, roleID string) error
	GetUserRoleNames(ctx context.Context, userID string) ([]string, error)

	CreateInvitation(ctx context.Context, inv Invitation) error
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*Invitation, error)
	GetLatestInvitationByUserID(ctx context.Context, userID string) (*Invitation, error)
	ConsumeInvitation(ctx context.Context, id string) error

	ActivateUserWithPassword(ctx context.Context, userID, passwordHash string) error
	CreateEnrollmentToken(ctx context.Context, token EnrollmentToken) error

	// WithinTx runs fn with a Repository bound to a single database
	// transaction, committing on nil and rolling back otherwise.
	WithinTx(ctx context.Context, fn func(Repository) error) error
}

// Mailer delivers out-of-band invitation messages. Structurally identical
// to the Auth domain's Mailer interface — the same SMTP implementation
// satisfies both without either domain importing the other.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
