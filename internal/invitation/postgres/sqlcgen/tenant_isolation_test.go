package sqlcgen

import (
	"strings"
	"testing"
)

// Invitation administration must not be able to resolve users, invitations,
// or roles belonging to a different organization, even when RLS is bypassed.
func TestInvitationQueriesAreExplicitlyTenantScoped(t *testing.T) {
	queries := map[string]string{
		"get roles":             getRolesByIDs,
		"find user by email":    getUserByEmail,
		"find user by id":       getUserByID,
		"assign role":           assignUserRole,
		"list role names":       getUserRoleNames,
		"find invitation token": getInvitationByTokenHash,
		"find latest invite":    getLatestInvitationByUserID,
		"consume invite":        consumeInvitation,
		"activate invited user": activateUserWithPassword,
	}

	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			normalized := strings.Join(strings.Fields(query), " ")
			if name == "find user by email" {
				if !strings.Contains(normalized, "WHERE tenant_id = $1 AND email = $2") {
					t.Fatalf("query is not explicitly parameterized by tenant: %s", normalized)
				}
				return
			}
			if !strings.Contains(normalized, "current_setting('app.tenant_id', true)") {
				t.Fatalf("query is not explicitly tenant-scoped: %s", normalized)
			}
		})
	}
}
