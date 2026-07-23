package middleware

import (
	"net/http"
	"slices"

	"nodus-health/pkg/response"
)

// RequirePermission builds a middleware that 403s unless the authenticated
// caller (injected by Authenticate) has the given permission code. Not
// exercised by any route in this pass — every Auth/MFA/Password/Sessions
// endpoint only requires "is authenticated" — but ready for the Users/Roles
// domains, where permission checks belong here or in the service layer, not
// in handlers.
func RequirePermission(code string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, ok := AuthFromContext(r.Context())
			if !ok || !slices.Contains(ac.Permissions, code) {
				response.Forbidden(w, "you do not have permission to perform this action")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
