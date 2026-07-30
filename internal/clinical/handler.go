package clinical

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"nodus-health/internal/middleware"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
	"strings"
)

type Handler struct {
	service    *Service
	authorizer middleware.Authorizer
	jwtSecret  string
	log        *logger.Logger
}

func NewHandler(s *Service, a middleware.Authorizer, j string, l *logger.Logger) *Handler {
	return &Handler{service: s, authorizer: a, jwtSecret: j, log: l}
}
func bind[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var q T
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&q); e != nil {
		response.BadRequest(w, "invalid JSON request body")
		return q, false
	}
	return q, true
}
func (h *Handler) fail(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, ErrNotFound):
		response.NotFound(w, e.Error())
	case errors.Is(e, ErrConflict):
		response.Conflict(w, e.Error())
	case errors.Is(e, ErrInvalidInput), errors.Is(e, ErrInvalidTransition), errors.Is(e, ErrReasonRequired):
		response.Validation(w, map[string]string{"error": e.Error()})
	default:
		h.log.Error("clinical request failed", "error", e.Error())
		response.Internal(w)
	}
}
func actor(r *http.Request) (string, bool) {
	a, ok := middleware.AuthFromContext(r.Context())
	return a.UserID, ok
}
func (h *Handler) ListResources(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListResources(r.Context(), chi.URLParam(r, "kind"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CreateResource(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[CreateResourceRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.CreateResource(r.Context(), a, chi.URLParam(r, "kind"), q)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.Created(w, x)
}
func (h *Handler) CreateVisit(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[CreateVisitRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.CreateVisit(r.Context(), a, q)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.Created(w, x)
}
func (h *Handler) GetVisit(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.GetVisit(r.Context(), chi.URLParam(r, "visitId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListQueueEntries(r.Context(), chi.URLParam(r, "queueId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) Enqueue(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[EnqueueRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.Enqueue(r.Context(), a, chi.URLParam(r, "queueId"), q)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.Created(w, x)
}
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[TransitionRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.Transition(r.Context(), a, chi.URLParam(r, "entryId"), q)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListQueueHistory(r.Context(), chi.URLParam(r, "entryId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListRoutingRules(r.Context())
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[CreateRoutingRuleRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.CreateRoutingRule(r.Context(), a, q)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.Created(w, x)
}
func (h *Handler) ICD10(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.SearchICD10(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
