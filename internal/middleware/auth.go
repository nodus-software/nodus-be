package middleware

import (
	"context"
	"net/http"
	"strings"

	"nodus-health/internal/tenant"
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

type EnrollmentTokenResolver interface {
	ResolveEnrollmentToken(ctx context.Context, rawToken string) (tokenID, userID string, err error)
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
			resolved, tenantResolved := tenant.FromContext(r.Context())
			if tenantResolved && (claims.TenantID == "" || claims.TenantID != resolved.ID) {
				response.Unauthorized(w, "access token is not valid for this tenant")
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
				TenantID:    claims.TenantID,
				Permissions: permissions,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticateSessionOrEnrollment permits either a normal access JWT or the
// narrowly-scoped token issued after accepting an organization/invitation.
// It is intended only for initial MFA setup and confirmation routes.
func AuthenticateSessionOrEnrollment(jwtSecret string, authorizer Authorizer, resolver EnrollmentTokenResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				response.Unauthorized(w, "missing or invalid authorization header")
				return
			}
			if claims, err := security.ParseAccessToken(jwtSecret, raw); err == nil {
				permissions, err := authorizer.Authorize(r.Context(), claims.Subject, claims.SessionID)
				if err != nil {
					response.Unauthorized(w, "session no longer valid")
					return
				}
				ctx := withAuth(r.Context(), AuthContext{
					UserID: claims.Subject, SessionID: claims.SessionID,
					TenantID: claims.TenantID, Permissions: permissions,
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			tokenID, userID, err := resolver.ResolveEnrollmentToken(r.Context(), raw)
			if err != nil {
				response.Unauthorized(w, "invalid or expired enrollment token")
				return
			}
			ctx := withAuth(r.Context(), AuthContext{UserID: userID, EnrollmentTokenID: tokenID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
