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
	r.Post("/auth/login/mfa/webauthn/options", h.WebAuthnLoginOptions)
	r.Post("/auth/login/mfa/webauthn/verify", h.WebAuthnLoginVerify)
	r.Post("/auth/refresh", h.Refresh)
	r.Get("/auth/password/policy", h.GetPasswordPolicy)
	r.Post("/auth/password/reset/request", h.RequestPasswordReset)
	r.Post("/auth/password/reset/confirm", h.ConfirmPasswordReset)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthenticateSessionOrEnrollment(h.jwtSecret, h.service, h.service))
		r.Post("/auth/mfa/totp/setup", h.SetupTOTP)
		r.Post("/auth/mfa/totp/confirm", h.ConfirmTOTP)
		r.Post("/auth/mfa/webauthn/register/options", h.WebAuthnRegistrationOptions)
		r.Post("/auth/mfa/webauthn/register/verify", h.WebAuthnRegistrationVerify)
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.service))

		r.Post("/auth/logout", h.Logout)
		r.Get("/auth/me", h.Me)

		r.Get("/auth/mfa/factors", h.ListFactors)
		r.Post("/auth/mfa/factors/{factorId}/remove", h.RemoveFactor)
		r.Get("/auth/mfa/recovery-codes/status", h.RecoveryCodeStatus)
		r.Post("/auth/mfa/recovery-codes/regenerate", h.RegenerateRecoveryCodes)

		r.Post("/auth/password/change", h.ChangePassword)

		r.Get("/auth/sessions", h.ListSessions)
		r.Delete("/auth/sessions/{sessionId}", h.RevokeSession)
	})
}
