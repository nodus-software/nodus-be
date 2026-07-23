package roles

import (
	"errors"
	"net/http"

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
	case errors.Is(err, ErrRoleNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrRoleNameTaken):
		response.Conflict(w, err.Error())
	case errors.Is(err, ErrUnknownPermissions):
		response.Validation(w, map[string]string{"permissions": err.Error()})
	case errors.Is(err, ErrSuperuserRequired), errors.Is(err, ErrPermissionDenied):
		response.Forbidden(w, err.Error())
	default:
		h.log.Error("unexpected roles domain error", "error", err.Error())
		response.Internal(w)
	}
}

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.service.ListRoles(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, roles)
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[CreateRoleRequest](w, r)
	if !ok {
		return
	}
	role, err := h.service.CreateRole(r.Context(), ac.UserID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, role)
}
