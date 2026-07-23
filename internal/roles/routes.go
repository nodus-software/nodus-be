package roles

import (
	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
)

// RegisterRoutes attaches the /roles endpoints from the contract to r. Every
// route requires authentication; least-privilege access is then enforced by
// permission code (roles:read / roles:write), with the superuser-only rule
// for role creation enforced in the service layer itself.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))

		r.With(middleware.RequirePermission("roles:read")).Get("/roles", h.ListRoles)
		r.With(middleware.RequirePermission("roles:write")).Post("/roles", h.CreateRole)
	})
}
