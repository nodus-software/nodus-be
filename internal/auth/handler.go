package auth

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
)

type Handler struct {
	service       *Service
	jwtSecret     string
	log           *logger.Logger
	refreshCookie RefreshCookieConfig
}

type RefreshCookieConfig struct {
	Name     string
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

func NewHandler(service *Service, jwtSecret string, log *logger.Logger, cookieConfigs ...RefreshCookieConfig) *Handler {
	cfg := RefreshCookieConfig{Name: "nodus_refresh", SameSite: http.SameSiteLaxMode}
	if len(cookieConfigs) > 0 {
		cfg = cookieConfigs[0]
	}
	return &Handler{service: service, jwtSecret: jwtSecret, log: log, refreshCookie: cfg}
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, pair *TokenPairResponse) {
	cookie := &http.Cookie{
		Name: h.refreshCookie.Name, Value: pair.RefreshToken, Path: "/",
		Domain: h.refreshCookie.Domain, HttpOnly: true, Secure: h.refreshCookie.Secure,
		SameSite: h.refreshCookie.SameSite,
	}
	if pair.RememberMe {
		cookie.Expires = pair.RefreshExpiresAt
		cookie.MaxAge = max(1, int(time.Until(pair.RefreshExpiresAt).Seconds()))
	}
	http.SetCookie(w, cookie)
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: h.refreshCookie.Name, Value: "", Path: "/", Domain: h.refreshCookie.Domain,
		HttpOnly: true, Secure: h.refreshCookie.Secure, SameSite: h.refreshCookie.SameSite,
		MaxAge: -1, Expires: time.Unix(1, 0).UTC(),
	})
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
	var retry *RetryError

	switch {
	case errors.As(err, &retry) && errors.Is(err, ErrAuthenticationChallenge):
		response.ErrorWithDetails(w, http.StatusTooManyRequests, "AUTH_CHALLENGE_REQUIRED", "additional verification required", AuthenticationChallengeResponse{Challenge: "turnstile", RetryAfter: int(retry.RetryAfter.Seconds())})
	case errors.As(err, &retry) && errors.Is(err, ErrAuthenticationDelayed):
		seconds := max(1, int((retry.RetryAfter+time.Second-1)/time.Second))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		response.ErrorWithDetails(w, http.StatusTooManyRequests, "AUTH_RETRY_LATER", "authentication temporarily unavailable", AuthenticationChallengeResponse{RetryAfter: seconds})
	case errors.Is(err, ErrAuthenticationUnavailable):
		response.Error(w, http.StatusServiceUnavailable, "AUTH_TEMPORARILY_UNAVAILABLE", "authentication temporarily unavailable")
	case errors.As(err, &locked):
		response.ErrorWithDetails(w, http.StatusLocked, "ACCOUNT_LOCKED", "account is locked", AccountLockedResponse{
			LockedUntil: locked.LockedUntil,
			Reason:      "Exceeded maximum failed login attempts",
		})
	case errors.As(err, &policy):
		response.Validation(w, policy.Violations)
	case errors.Is(err, ErrChallengeExpired):
		response.Error(w, http.StatusGone, "CHALLENGE_EXPIRED", err.Error())
	case errors.Is(err, ErrResetTokenInvalid), errors.Is(err, ErrRecoveryTokenInvalid), errors.Is(err, ErrInvalidPublicKey):
		response.BadRequest(w, err.Error())
	case errors.Is(err, ErrWebAuthnInvalid), errors.Is(err, ErrWebAuthnUnavailable):
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
	case errors.Is(err, ErrTOTPAlreadyEnrolled):
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

func (h *Handler) WebAuthnRegistrationOptions(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	req, ok := bindJSON[WebAuthnRegistrationOptionsRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.BeginWebAuthnRegistration(r.Context(), ac.UserID, ac.EnrollmentTokenID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
}
func (h *Handler) WebAuthnRegistrationVerify(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	req, ok := bindJSON[WebAuthnRegistrationVerifyRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.FinishWebAuthnRegistration(r.Context(), ac.UserID, ac.EnrollmentTokenID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.Created(w, result)
}
func (h *Handler) WebAuthnLoginOptions(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[WebAuthnLoginOptionsRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.BeginWebAuthnLogin(r.Context(), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
}
func (h *Handler) WebAuthnLoginVerify(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[WebAuthnLoginVerifyRequest](w, r)
	if !ok {
		return
	}
	pair, err := h.service.FinishWebAuthnLogin(r.Context(), req, clientIP(r), r.UserAgent(), deviceLabelFromUserAgent(r.UserAgent()))
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.setRefreshCookie(w, pair)
	response.OK(w, pair)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[LoginRequest](w, r)
	if !ok {
		return
	}
	resp, err := h.service.Login(r.Context(), req, clientIP(r), r.UserAgent())
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
	w.Header().Set("Cache-Control", "no-store")
	h.setRefreshCookie(w, pair)
	response.OK(w, pair)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	requestID, _ := middleware.RequestIDFromContext(r.Context())
	cookie, err := r.Cookie(h.refreshCookie.Name)
	if err != nil || cookie.Value == "" {
		h.log.Warning("refresh rejected", "reason", "cookie_missing", "request_id", requestID)
		h.clearRefreshCookie(w)
		h.writeError(w, ErrRefreshTokenInvalid)
		return
	}
	pair, err := h.service.Refresh(r.Context(), cookie.Value)
	if err != nil {
		reason := "invalid"
		if errors.Is(err, ErrRefreshTokenRevoked) {
			reason = "revoked"
		}
		h.log.Warning("refresh rejected", "reason", reason, "request_id", requestID)
		// A revoked token may be a harmless concurrent replay after another tab has
		// already rotated the shared cookie. Do not let the losing response erase the
		// newly issued cookie. Unknown/expired tokens are still cleared.
		if !errors.Is(err, ErrRefreshTokenRevoked) {
			h.clearRefreshCookie(w)
		}
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.setRefreshCookie(w, pair)
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
	h.clearRefreshCookie(w)
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
	result, err := h.service.ConfirmTOTP(r.Context(), ac.UserID, req.Code, ac.EnrollmentTokenID)
	if err != nil {
		if errors.Is(err, ErrMFACodeInvalid) {
			response.BadRequest(w, err.Error())
			return
		}
		h.writeError(w, err)
		return
	}
	response.OK(w, result)
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
	req, ok := bindJSON[RemoveMFAFactorRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.RemoveFactor(r.Context(), ac.UserID, factorID, req.CurrentPassword); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) RecoveryCodeStatus(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	result, err := h.service.RecoveryCodeStatus(r.Context(), ac.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, result)
}

func (h *Handler) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	ac, ok := authContext(w, r)
	if !ok {
		return
	}
	req, ok := bindJSON[RegenerateRecoveryCodesRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.RegenerateRecoveryCodes(r.Context(), ac.UserID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
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

func (h *Handler) RequestRecovery(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.RequestRecovery(r.Context(), req, clientIP(r)); err != nil {
		h.log.Error("account recovery request failed", "error", err.Error())
	}
	response.Accepted(w, map[string]string{"status": "accepted"})
}

func (h *Handler) VerifyRecovery(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryVerifyRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.VerifyRecovery(r.Context(), req.Token)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
}

func (h *Handler) RecoveryPassword(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryPasswordRequest](w, r)
	if !ok {
		return
	}
	if err := h.service.CompleteRecoveryPassword(r.Context(), req); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) RecoveryTOTPSetup(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryTOTPSetupRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.SetupRecoveryTOTP(r.Context(), req.RecoveryToken)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
}

func (h *Handler) RecoveryTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryTOTPConfirmRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.ConfirmRecoveryTOTP(r.Context(), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
}

func (h *Handler) RecoveryWebAuthnOptions(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryWebAuthnOptionsRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.BeginRecoveryWebAuthn(r.Context(), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
}

func (h *Handler) RecoveryWebAuthnVerify(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[RecoveryWebAuthnVerifyRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.FinishRecoveryWebAuthn(r.Context(), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.OK(w, result)
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
