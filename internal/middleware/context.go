package middleware

import "context"

type requestIDKey struct{}
type authContextKey struct{}

// AuthContext is what the Authenticate middleware injects into the request
// context once a bearer token has been validated.
type AuthContext struct {
	UserID      string
	SessionID   string
	Permissions []string
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the request ID injected by the RequestID
// middleware, if present.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey{}).(string)
	return id, ok
}

func withAuth(ctx context.Context, ac AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, ac)
}

// AuthFromContext returns the authenticated caller's identity, if the
// request passed through the Authenticate middleware.
func AuthFromContext(ctx context.Context) (AuthContext, bool) {
	ac, ok := ctx.Value(authContextKey{}).(AuthContext)
	return ac, ok
}
