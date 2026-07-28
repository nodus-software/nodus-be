package users

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
	switch {
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrRoleNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrNotLocked), errors.Is(err, ErrInvitationPending):
		response.Conflict(w, err.Error())
	case errors.Is(err, ErrInvalidStatusTransition), errors.Is(err, ErrSelfDeactivation), errors.Is(err, ErrLastSuperuser):
		response.Conflict(w, err.Error())
	case errors.Is(err, ErrProviderIdentifierRequired):
		response.Validation(w, map[string]string{"provider_identifier": err.Error()})
	case errors.Is(err, ErrSuperuserRequired), errors.Is(err, ErrPermissionDenied):
		response.Forbidden(w, err.Error())
	default:
		h.log.Error("unexpected users domain error", "error", err.Error())
		response.Internal(w)
	}
}

func (h *Handler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[LifecycleReasonRequest](w, r)
	if !ok {
		return
	}
	profile, err := h.service.DeactivateUser(r.Context(), ac.UserID, chi.URLParam(r, "userId"), req.Reason)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, profile)
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	filter := ListUsersFilter{}
	q := r.URL.Query()
	if v := q.Get("role"); v != "" {
		filter.Role = &v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = &v
	}
	if v := q.Get("stale_access"); v != "" {
		stale := v == "true"
		filter.StaleAccess = &stale
	}

	list, err := h.service.ListUsers(r.Context(), filter)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, list)
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[UpdateUserRequest](w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userId")

	profile, err := h.service.UpdateUser(r.Context(), ac.UserID, userID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, profile)
}

func (h *Handler) RecordAccessReview(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[AccessReviewRequest](w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userId")

	profile, err := h.service.RecordAccessReview(r.Context(), ac.UserID, userID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, profile)
}

func (h *Handler) UnlockUser(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[UnlockRequest](w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userId")

	if err := h.service.UnlockUser(r.Context(), ac.UserID, userID, req.Reason); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}
