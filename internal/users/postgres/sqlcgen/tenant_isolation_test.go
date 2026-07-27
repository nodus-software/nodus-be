package sqlcgen

import (
	"strings"
	"testing"
)

// RLS remains defense in depth, but these administration queries must also
// scope themselves explicitly because a privileged database role can bypass
// RLS and expose or mutate another organization's users.
func TestUserAdministrationQueriesAreExplicitlyTenantScoped(t *testing.T) {
	queries := map[string]string{
		"list users":              listUsers,
		"get user profile":        getUserWithRolesByID,
		"get user":                getUserByID,
		"get roles":               getRolesByIDs,
		"check superuser":         hasSuperuserRole,
		"delete user roles":       deleteUserRoles,
		"insert user role":        insertUserRole,
		"update user status":      updateUserStatus,
		"set provider identifier": setProviderIdentifier,
		"record access review":    recordAccessReview,
		"unlock user":             unlockUser,
	}

	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			normalized := strings.Join(strings.Fields(query), " ")
			if !strings.Contains(normalized, "current_setting('app.tenant_id', true)") {
				t.Fatalf("query is not explicitly tenant-scoped: %s", normalized)
			}
		})
	}
}
