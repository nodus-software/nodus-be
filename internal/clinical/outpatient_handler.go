package clinical

import (
	"net/http"
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
	x, e := h.service.ListOutpatientVisits(r.Context(), strings.TrimSpace(r.URL.Query().Get("status")))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CreateEncounter(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateEncounterRequest](w, r)
	if !ok {
		return
	}
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
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CompleteVisit(r.Context(), a, chi.URLParam(r, "visitId"))
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) VisitSummary(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.VisitSummary(r.Context(), chi.URLParam(r, "visitId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
