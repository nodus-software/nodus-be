package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
)

// Recovery catches panics from downstream handlers, logs them with a stack
// trace, and returns a clean 500 via pkg/response instead of crashing the
// process or leaking a raw stack trace to the client.
func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					requestID, _ := RequestIDFromContext(r.Context())
					log.Error("panic recovered",
						"panic", fmt.Sprintf("%v", rec),
						"stack", string(debug.Stack()),
						"request_id", requestID,
						"path", r.URL.Path,
					)
					response.Internal(w)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
