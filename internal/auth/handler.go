package auth

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
)

type Handler struct {
	service   *Service
	jwtSecret string
	log       *logger.Logger
}

func NewHandler(service *Service, jwtSecret string, log *logger.Logger) *Handler {
	return &Handler{service: service, jwtSecret: jwtSecret, log: log}
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func deviceLabelFromUserAgent(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	const maxLen = 120
	if len(ua) > maxLen {
		return ua[:maxLen]
	}
	return ua
}

func authContext(w http.ResponseWriter, r *http.Request) (middleware.AuthContext, bool) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
	}
	return ac, ok
}

// writeError translates a domain error into the appropriate pkg/response
// call. Unmapped errors never leak internals to the client — they fall
// through to a generic 500, logged here for investigation.
func (h *Handler) writeError(w http.ResponseWriter, err error) {
	var locked *LockedError
	var policy *PolicyViolationError

	switch {
	case errors.As(err, &locked):
		response.ErrorWithDetails(w, http.StatusLocked, "ACCOUNT_LOCKED", "account is locked", AccountLockedResponse{
			LockedUntil: locked.LockedUntil,
			Reason:      "Exceeded maximum failed login attempts",
		})
	case errors.As(err, &policy):
		response.Validation(w, policy.Violations)
	case errors.Is(err, ErrChallengeExpired):
		response.Error(w, http.StatusGone, "CHALLENGE_EXPIRED", err.Error())
	case errors.Is(err, ErrResetTokenInvalid), errors.Is(err, ErrInvalidPublicKey):
		response.BadRequest(w, err.Error())
	case errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, ErrMFANotEnrolled),
		errors.Is(err, ErrMFACodeInvalid),
		errors.Is(err, ErrCurrentPasswordInvalid),
		errors.Is(err, ErrChallengeInvalid),
		errors.Is(err, ErrRefreshTokenInvalid),
		errors.Is(err, ErrRefreshTokenRevoked):
		response.Unauthorized(w, err.Error())
	case errors.Is(err, ErrFactorNotFound), errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrUserNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrLastFactorRemaining):
		response.Conflict(w, err.Error())
	case errors.Is(err, ErrRateLimitExceeded):
		response.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error())
	case errors.Is(err, ErrPermissionDenied):
		response.Forbidden(w, err.Error())
	default:
		h.log.Error("unexpected auth domain error", "error", err.Error())
		response.Internal(w)
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[LoginRequest](w, r)
	if !ok {
		return
	}
	resp, err := h.service.Login(r.Context(), req, clientIP(r))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, resp)
}

func (h *Handler) LoginMFA(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[VerifyMFARequest](w, r)
	if !ok {
		return
	}
	pair, err := h.service.VerifyMFA(r.Context(), req, clientIP(r), r.UserAgent(), deviceLabelFromUserAgent(r.UserAgent()))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, pair)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RefreshRequest](w, r)
	if !ok {
		return
	}
	pair, err := h.service.Refresh(r.Context(), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, pair)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	if err := h.service.Logout(r.Context(), ac.UserID, ac.SessionID, clientIP(r)); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	profile, err := h.service.Me(r.Context(), ac.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, profile)
}

func (h *Handler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	resp, err := h.service.SetupTOTP(r.Context(), ac.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, resp)
}

func (h *Handler) ConfirmTOTP(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	req, ok := bindJSON[ConfirmTOTPRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.ConfirmTOTP(r.Context(), ac.UserID, req.Code); err != nil {
		if errors.Is(err, ErrMFACodeInvalid) {
			response.BadRequest(w, err.Error())
			return
		}
		h.writeError(w, err)
		return
	}
	if ac.EnrollmentTokenID != "" {
		if err := h.service.ConsumeEnrollmentToken(r.Context(), ac.EnrollmentTokenID); err != nil {
			h.writeError(w, err)
			return
		}
	}
	response.OK(w, map[string]string{"status": "enabled"})
}

func (h *Handler) RegisterBiometric(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	req, ok := bindJSON[RegisterBiometricRequest](w, r)
	if !ok {
		return
	}
	factor, err := h.service.RegisterBiometric(r.Context(), ac.UserID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, MFAFactorResponse{ID: factor.ID, Type: string(factor.Type), Label: factor.Label, CreatedAt: factor.CreatedAt})
}

func (h *Handler) ListFactors(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	factors, err := h.service.ListFactors(r.Context(), ac.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, factors)
}

func (h *Handler) RemoveFactor(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	factorID := chi.URLParam(r, "factorId")
	if err := h.service.RemoveFactor(r.Context(), ac.UserID, factorID); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	req, ok := bindJSON[ChangePasswordRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.ChangePassword(r.Context(), ac.UserID, ac.SessionID, req); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) GetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.service.GetPasswordPolicy())
}

func (h *Handler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RequestPasswordResetRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.RequestPasswordReset(r.Context(), req, clientIP(r)); err != nil {
		h.writeError(w, err)
		return
	}
	response.Accepted(w, map[string]string{"status": "accepted"})
}

func (h *Handler) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[ConfirmPasswordResetRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.ConfirmPasswordReset(r.Context(), req); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	sessions, err := h.service.ListSessions(r.Context(), ac.UserID, ac.SessionID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, sessions)
}

func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "sessionId")
	if err := h.service.RevokeSession(r.Context(), ac.UserID, sessionID, clientIP(r)); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}
