package clinical

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"nodus-health/pkg/response"
)

func (h *Handler) withActor(w http.ResponseWriter, r *http.Request, fn func(string) error) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	if e := fn(a); e != nil {
		h.fail(w, e)
	}
}
func (h *Handler) OutpatientCheckIn(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[OutpatientCheckInRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		if q.Override {
			return ErrInvalidInput
		}
		x, e := h.service.OutpatientCheckIn(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) OutpatientCheckInOverride(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[OutpatientCheckInRequest](w, r)
	if !ok {
		return
	}
	q.Override = true
	h.withActor(w, r, func(a string) error {
		x, e := h.service.OutpatientCheckIn(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) ListOutpatientVisits(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	f := OutpatientVisitFilters{Date: strings.TrimSpace(r.URL.Query().Get("date")), Status: strings.TrimSpace(r.URL.Query().Get("status")), Stage: strings.TrimSpace(r.URL.Query().Get("stage")), ServicePointID: strings.TrimSpace(r.URL.Query().Get("service_point_id")), ClinicianID: strings.TrimSpace(r.URL.Query().Get("clinician_id")), Query: strings.TrimSpace(r.URL.Query().Get("q")), Page: page, PerPage: per}
	x, total, e := h.service.ListOutpatientVisits(r.Context(), f)
	if e != nil {
		h.fail(w, e)
		return
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 25
	}
	response.Paginated(w, x, response.NewMeta(page, per, total))
}
func (h *Handler) CreateEncounter(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateEncounterRequest](w, r)
	if !ok {
		return
	}
	if q.EncounterType == "triage" || q.EncounterType == "consultation" {
		permission := "outpatient:consult"
		if q.EncounterType == "triage" {
			permission = "outpatient:triage"
		}
		if !hasPermission(r, permission) {
			response.Forbidden(w, "you do not have permission to perform this action")
			return
		}
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateEncounter(r.Context(), a, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CreateEncounterOverride(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateEncounterRequest](w, r)
	if !ok {
		return
	}
	q.Override = true
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateEncounter(r.Context(), a, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) ListEncounters(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListEncounters(r.Context(), chi.URLParam(r, "visitId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) RecordObservations(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[RecordObservationsRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.RecordObservations(r.Context(), a, chi.URLParam(r, "encounterId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CompleteEncounter(w http.ResponseWriter, r *http.Request) {
	if !h.requireEncounterPermission(w, r, chi.URLParam(r, "encounterId")) {
		return
	}
	q, ok := bind[CompleteEncounterRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CompleteEncounter(r.Context(), a, chi.URLParam(r, "encounterId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) CreateNote(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateNoteRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateNote(r.Context(), a, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CreateDiagnosis(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateDiagnosisRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateDiagnosis(r.Context(), a, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CreateAllergy(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateAllergyRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateAllergy(r.Context(), a, chi.URLParam(r, "patientId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) ListAllergies(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListAllergies(r.Context(), chi.URLParam(r, "patientId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CompleteVisit(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CompleteVisitRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CompleteVisit(r.Context(), a, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) CompleteVisitOverride(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CompleteVisitRequest](w, r)
	if !ok {
		return
	}
	q.Override = true
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CompleteVisit(r.Context(), a, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) VisitContext(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.OutpatientVisitContext(r.Context(), chi.URLParam(r, "visitId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) VisitSummary(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.VisitSummary(r.Context(), chi.URLParam(r, "visitId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
