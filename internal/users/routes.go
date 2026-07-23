package users

import (
	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
)

// RegisterRoutes attaches the admin-facing /users endpoints from the
// contract to r. Invitation lifecycle endpoints (/users/invitations/*)
// belong to the Invitation domain, not here.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))

		r.With(middleware.RequirePermission("users:read")).Get("/users", h.ListUsers)
		r.With(middleware.RequirePermission("users:write")).Patch("/users/{userId}", h.UpdateUser)
		r.With(middleware.RequirePermission("users:review")).Post("/users/{userId}/access-review", h.RecordAccessReview)
		r.With(middleware.RequirePermission("users:unlock")).Post("/users/{userId}/unlock", h.UnlockUser)
	})
}
