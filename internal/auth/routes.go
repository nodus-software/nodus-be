package auth

import (
	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
)

// RegisterRoutes attaches every Auth/MFA/Password/Sessions endpoint from the
// contract to r.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/auth/login", h.Login)
	r.Post("/auth/login/mfa", h.LoginMFA)
	r.Post("/auth/refresh", h.Refresh)
	r.Get("/auth/password/policy", h.GetPasswordPolicy)
	r.Post("/auth/password/reset/request", h.RequestPasswordReset)
	r.Post("/auth/password/reset/confirm", h.ConfirmPasswordReset)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthenticateSessionOrEnrollment(h.jwtSecret, h.service, h.service))
		r.Post("/auth/mfa/totp/setup", h.SetupTOTP)
		r.Post("/auth/mfa/totp/confirm", h.ConfirmTOTP)
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.service))

		r.Post("/auth/logout", h.Logout)
		r.Get("/auth/me", h.Me)

		r.Post("/auth/mfa/biometric/register", h.RegisterBiometric)
		r.Get("/auth/mfa/factors", h.ListFactors)
		r.Delete("/auth/mfa/factors/{factorId}", h.RemoveFactor)

		r.Post("/auth/password/change", h.ChangePassword)

		r.Get("/auth/sessions", h.ListSessions)
		r.Delete("/auth/sessions/{sessionId}", h.RevokeSession)
	})
}
