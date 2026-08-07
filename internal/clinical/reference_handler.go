package clinical

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"nodus-health/pkg/response"
)

func intQuery(r *http.Request, key string, def int) int {
	x, e := strconv.Atoi(r.URL.Query().Get(key))
	if e != nil || x < 1 {
		return def
	}
	return x
}
func (h *Handler) ICD11(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.SearchICD11(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) ListDiagnosisConfiguration(w http.ResponseWriter, r *http.Request) {
	f := DiagnosisFilters{Query: r.URL.Query().Get("q"), Chapter: r.URL.Query().Get("chapter"), Availability: r.URL.Query().Get("availability"), Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 50)}
	x, e := h.service.ListDiagnosisConcepts(r.Context(), f)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) SetDiagnosisAvailability(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[SetDiagnosisAvailabilityRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.SetDiagnosisConceptEnabled(r.Context(), a, chi.URLParam(r, "conceptId"), q.Enabled)
	if e != nil {
		h.fail(w, e)
		return
	}
	h.log.Info("diagnosis configuration updated", "concept_id", x.ID, "enabled", x.Enabled, "actor_id", a)
	response.OK(w, x)
}
func (h *Handler) ListAllergenConfiguration(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListAllergens(r.Context())
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CreateAllergenConfiguration(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[CreateAllergenRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.CreateAllergen(r.Context(), a, q)
	if e != nil {
		h.fail(w, e)
		return
	}
	h.log.Info("allergen configuration created", "allergen_id", x.ID, "actor_id", a)
	response.Created(w, x)
}
func (h *Handler) UpdateAllergenConfiguration(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	q, ok := bind[UpdateAllergenRequest](w, r)
	if !ok {
		return
	}
	x, e := h.service.UpdateAllergen(r.Context(), a, chi.URLParam(r, "allergenId"), q)
	if e != nil {
		h.fail(w, e)
		return
	}
	h.log.Info("allergen configuration updated", "allergen_id", x.ID, "actor_id", a)
	response.OK(w, x)
}
func (h *Handler) AllergenLifecycle(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	active := chi.URLParam(r, "action") == "reactivate"
	x, e := h.service.SetAllergenActive(r.Context(), a, chi.URLParam(r, "allergenId"), active)
	if e != nil {
		h.fail(w, e)
		return
	}
	h.log.Info("allergen configuration lifecycle changed", "allergen_id", x.ID, "active", active, "actor_id", a)
	response.OK(w, x)
}
