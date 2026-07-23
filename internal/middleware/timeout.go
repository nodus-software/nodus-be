package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout bounds every request's context to d, so a slow downstream
// dependency can't hold a handler goroutine open indefinitely.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
