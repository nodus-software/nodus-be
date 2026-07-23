package invitation

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
)

type Handler struct {
	service    *Service
	authorizer middleware.Authorizer
	jwtSecret  string
	log        *logger.Logger
}

func NewHandler(service *Service, authorizer middleware.Authorizer, jwtSecret string, log *logger.Logger) *Handler {
	return &Handler{service: service, authorizer: authorizer, jwtSecret: jwtSecret, log: log}
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	var policy *PolicyViolationError

	switch {
	case errors.As(err, &policy):
		response.ErrorWithDetails(w, http.StatusBadRequest, "PASSWORD_POLICY_VIOLATION", policy.Error(), policy.Violations)
	case errors.Is(err, ErrUserNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrRoleNotFound):
		response.Validation(w, map[string]string{"role_ids": err.Error()})
	case errors.Is(err, ErrProviderIdentifierRequired):
		response.Validation(w, map[string]string{"provider_identifier": err.Error()})
	case errors.Is(err, ErrEmailAlreadyExists), errors.Is(err, ErrNotPending):
		response.Conflict(w, err.Error())
	case errors.Is(err, ErrTokenExpired):
		response.Error(w, http.StatusGone, "INVITATION_EXPIRED", err.Error())
	case errors.Is(err, ErrTokenInvalid):
		response.BadRequest(w, err.Error())
	default:
		h.log.Error("unexpected invitation domain error", "error", err.Error())
		response.Internal(w)
	}
}

// Invite handles POST /users/invitations (admin only).
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[InviteUserRequest](w, r)
	if !ok {
		return
	}
	profile, err := h.service.Invite(r.Context(), ac.UserID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, profile)
}

// ValidateToken handles GET /users/invitations/{token} (unauthenticated).
func (h *Handler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	preview, err := h.service.ValidateToken(r.Context(), token)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, preview)
}

// Accept handles POST /users/invitations/{token}/accept (unauthenticated).
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[AcceptInviteRequest](w, r)
	if !ok {
		return
	}
	enrollment, err := h.service.Accept(r.Context(), req.Token, req.Password)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, enrollment)
}

// Resend handles POST /users/invitations/{token}/resend (admin only). See
// Service.Resend for why the path segment here is treated as the invited
// user's ID rather than the invite's raw secret token.
func (h *Handler) Resend(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	userID := chi.URLParam(r, "token")
	if err := h.service.Resend(r.Context(), ac.UserID, userID); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}
