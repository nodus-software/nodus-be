package clinical

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"nodus-health/internal/middleware"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
	"slices"
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
	var lifecycle *LifecycleConflictError
	var formValidation *FormValidationError
	var activeVisit *ActiveVisitConflictError
	switch {
	case errors.Is(e, ErrNotFound):
		response.NotFound(w, e.Error())
	case errors.As(e, &lifecycle):
		response.ErrorWithDetails(w, http.StatusConflict, "CONFIGURATION_CONFLICT", e.Error(), lifecycle.Impact)
	case errors.As(e, &formValidation):
		response.ErrorWithDetails(w, http.StatusUnprocessableEntity, "FORM_VALIDATION_FAILED", e.Error(), formValidation.Errors)
	case errors.As(e, &activeVisit):
		response.ErrorWithDetails(w, http.StatusConflict, "ACTIVE_VISIT_CONFLICT", e.Error(), activeVisit)
	case errors.Is(e, ErrConflict), errors.Is(e, ErrActiveVisit), errors.Is(e, ErrInactiveParent), errors.Is(e, ErrRoutingMissing):
		response.Conflict(w, e.Error())
	case errors.Is(e, ErrInvalidInput), errors.Is(e, ErrInvalidTransition), errors.Is(e, ErrReasonRequired), errors.Is(e, ErrVisitIncomplete), errors.Is(e, ErrFormIncomplete), errors.Is(e, ErrEncounterStartRequired):
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
func hasPermission(r *http.Request, code string) bool {
	a, ok := middleware.AuthFromContext(r.Context())
	return ok && (slices.Contains(a.Permissions, code) || slices.Contains(a.Permissions, "*"))
}

func (h *Handler) requireEncounterPermission(w http.ResponseWriter, r *http.Request, encounterID string) bool {
	encounter, err := h.service.GetEncounter(r.Context(), encounterID)
	if err != nil {
		h.fail(w, err)
		return false
	}
	permission := "outpatient:consult"
	if encounter.EncounterType == "triage" {
		permission = "outpatient:triage"
	}
	if !hasPermission(r, permission) {
		response.Forbidden(w, "you do not have permission to perform this action")
		return false
	}
	return true
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
	h.log.Info("clinical configuration created", "kind", chi.URLParam(r, "kind"), "resource_id", x.ID, "actor_id", a)
	response.Created(w, x)
}

func (h *Handler) UpdateResource(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[UpdateResourceRequest](w, r)
	if !ok {
		return
	}
	x, err := h.service.UpdateResource(r.Context(), a, chi.URLParam(r, "kind"), chi.URLParam(r, "resourceId"), q)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.log.Info("clinical configuration updated", "kind", chi.URLParam(r, "kind"), "resource_id", x.ID, "actor_id", a)
	response.OK(w, x)
}

func (h *Handler) DeactivationImpact(w http.ResponseWriter, r *http.Request) {
	x, err := h.service.DeactivationImpact(r.Context(), chi.URLParam(r, "kind"), chi.URLParam(r, "resourceId"))
	if err != nil {
		h.fail(w, err)
		return
	}
	response.OK(w, x)
}

func (h *Handler) DeactivateResource(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[DeactivateResourceRequest](w, r)
	if !ok {
		return
	}
	x, err := h.service.DeactivateResource(r.Context(), a, chi.URLParam(r, "kind"), chi.URLParam(r, "resourceId"), q)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.log.Info("clinical configuration deactivated", "kind", chi.URLParam(r, "kind"), "resource_id", chi.URLParam(r, "resourceId"), "affected_count", len(x.Affected), "actor_id", a)
	response.OK(w, x)
}

func (h *Handler) ReactivateResource(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	x, err := h.service.ReactivateResource(r.Context(), a, chi.URLParam(r, "kind"), chi.URLParam(r, "resourceId"))
	if err != nil {
		h.fail(w, err)
		return
	}
	h.log.Info("clinical configuration reactivated", "kind", chi.URLParam(r, "kind"), "resource_id", x.ID, "actor_id", a)
	response.OK(w, x)
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
func (h *Handler) StartTriage(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	x, err := h.service.StartTriage(r.Context(), a, chi.URLParam(r, "entryId"))
	if err != nil {
		h.fail(w, err)
		return
	}
	response.Created(w, x)
}
func (h *Handler) StartConsultation(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	x, err := h.service.StartConsultation(r.Context(), a, chi.URLParam(r, "entryId"))
	if err != nil {
		h.fail(w, err)
		return
	}
	response.Created(w, x)
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
