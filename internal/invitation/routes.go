package invitation

import (
	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
)

// RegisterRoutes attaches the /users/invitations/* endpoints from the
// contract to r. Validate/accept are unauthenticated (the invitee has no
// account yet); invite/resend require an authenticated admin.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/users/invitations/{token}", h.ValidateToken)
	r.Post("/users/invitations/{token}/accept", h.Accept)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))

		r.With(middleware.RequirePermission("users:invite")).Post("/users/invitations", h.Invite)
		r.With(middleware.RequirePermission("users:invite")).Post("/users/invitations/{token}/resend", h.Resend)
	})
}
