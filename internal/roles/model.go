package roles

import "time"

// Role is a named, assignable set of permissions. Superuser roles and
// clinical (provider-identifier-requiring) roles are flagged explicitly
// rather than inferred from name, since both flags gate real authorization
// decisions elsewhere (role creation, user invitation/role assignment).
type Role struct {
	ID                         string
	Name                       string
	Description                string
	IsSuperuserRole            bool
	RequiresProviderIdentifier bool
	Permissions                []string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}
