package sqlcgen

import (
	"strings"
	"testing"
)

// RLS is defense in depth. Role administration queries must remain safe even
// when the database connection role can bypass row-level security.
func TestRoleAdministrationQueriesAreExplicitlyTenantScoped(t *testing.T) {
	queries := map[string]string{
		"list roles":          listRolesWithPermissions,
		"get role":            getRoleByID,
		"get roles":           getRolesByIDs,
		"add role permission": addRolePermission,
		"check superuser":     hasSuperuserRole,
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
