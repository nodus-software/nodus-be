package sqlcgen

import (
	"strings"
	"testing"
)

// The application database role may bypass RLS in local development. These
// lookups therefore must remain explicitly tenant-scoped: relying on the RLS
// policy alone can return a same-named user from another organization.
func TestUserLookupsExplicitlyScopeByTenant(t *testing.T) {
	tests := map[string]string{
		"username": getUserByUsername,
		"id":       getUserByID,
	}

	for name, query := range tests {
		t.Run(name, func(t *testing.T) {
			normalized := strings.Join(strings.Fields(query), " ")
			if !strings.Contains(normalized, "FROM users WHERE tenant_id = $1 AND") {
				t.Fatalf("user lookup is not explicitly tenant-scoped: %s", normalized)
			}
		})
	}
}
