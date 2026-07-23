package audit

import (
	"net/http"
	"time"

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

// ListAuditLogs handles GET /audit-logs. This is a read-only view over an
// append-only store — there is intentionally no update or delete route
// anywhere for audit records.
func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := Filter{}
	if v := q.Get("user_id"); v != "" {
		filter.UserID = &v
	}
	if v := q.Get("action"); v != "" {
		filter.Action = &v
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = &t
		} else {
			response.BadRequest(w, "from must be an RFC3339 timestamp")
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = &t
		} else {
			response.BadRequest(w, "to must be an RFC3339 timestamp")
			return
		}
	}

	entries, err := h.service.Query(r.Context(), filter)
	if err != nil {
		h.log.Error("unexpected audit domain error", "error", err.Error())
		response.Internal(w)
		return
	}

	resp := make([]AuditLogEntryResponse, 0, len(entries))
	for _, e := range entries {
		resp = append(resp, toAuditLogEntryResponse(e))
	}
	response.OK(w, resp)
}
