package audit

import (
	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
)

// RegisterRoutes attaches GET /audit-logs from the contract to r.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))
		r.With(middleware.RequirePermission("audit:read")).Get("/audit-logs", h.ListAuditLogs)
	})
}
