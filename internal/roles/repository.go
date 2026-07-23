package roles

import "context"

// Repository persists roles and their permission assignments, and answers
// the one authorization question this domain needs of its own callers:
// whether a given user currently holds a superuser role.
type Repository interface {
	ListRolesWithPermissions(ctx context.Context) ([]Role, error)
	GetRoleByID(ctx context.Context, id string) (*Role, error)
	CreateRole(ctx context.Context, role Role) (*Role, error)
	GetPermissionsByCodes(ctx context.Context, codes []string) ([]Permission, error)
	AddRolePermission(ctx context.Context, roleID, permissionID string) error
	HasSuperuserRole(ctx context.Context, userID string) (bool, error)

	// WithinTx runs fn with a Repository bound to a single database
	// transaction, committing on nil and rolling back otherwise.
	WithinTx(ctx context.Context, fn func(Repository) error) error
}

type Permission struct {
	ID   string
	Code string
}
