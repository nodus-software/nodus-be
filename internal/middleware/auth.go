package middleware

import (
	"context"
	"net/http"
	"strings"

	"nodus-health/pkg/response"
	"nodus-health/pkg/security"
)

// Authorizer validates that a user/session pair is still allowed to act
// (user active, session not revoked) and resolves their current effective
// permissions. Permissions are deliberately re-resolved from the database on
// every request rather than trusted from the JWT, so a role change or
// session revocation takes effect on the very next request instead of
// waiting for the access token to expire.
type Authorizer interface {
	Authorize(ctx context.Context, userID, sessionID string) ([]string, error)
}

// Authenticate validates the bearer JWT on every request, re-resolves the
// caller's current permissions, and injects an AuthContext. Requests with a
// missing/invalid/expired token, or whose session/user is no longer valid,
// get 401.
func Authenticate(jwtSecret string, authorizer Authorizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				response.Unauthorized(w, "missing or invalid authorization header")
				return
			}

			claims, err := security.ParseAccessToken(jwtSecret, token)
			if err != nil {
				response.Unauthorized(w, "invalid or expired access token")
				return
			}

			permissions, err := authorizer.Authorize(r.Context(), claims.Subject, claims.SessionID)
			if err != nil {
				response.Unauthorized(w, "session no longer valid")
				return
			}

			ctx := withAuth(r.Context(), AuthContext{
				UserID:      claims.Subject,
				SessionID:   claims.SessionID,
				Permissions: permissions,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
