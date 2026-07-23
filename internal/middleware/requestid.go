package middleware

import (
	"net/http"

	"nodus-health/pkg/utility"
)

const RequestIDHeader = "X-Request-ID"

// RequestID assigns a request ID (reusing an inbound one if the caller
// already supplied one) and propagates it via context and response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			generated, err := utility.GenerateUUID()
			if err == nil {
				id = generated
			}
		}

		w.Header().Set(RequestIDHeader, id)
		ctx := withRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
