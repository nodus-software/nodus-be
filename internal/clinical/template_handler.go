package clinical

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"nodus-health/pkg/response"
)

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListTemplates(r.Context(), strings.TrimSpace(r.URL.Query().Get("encounter_type")), strings.TrimSpace(r.URL.Query().Get("status")))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.GetTemplate(r.Context(), chi.URLParam(r, "templateId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateTemplateRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateTemplate(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CreateTemplateDraft(w http.ResponseWriter, r *http.Request) {
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateTemplateDraft(r.Context(), a, chi.URLParam(r, "templateId"))
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) UpdateTemplateDraft(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[UpdateTemplateDraftRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.UpdateTemplateDraft(r.Context(), a, chi.URLParam(r, "templateId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) PublishTemplate(w http.ResponseWriter, r *http.Request) {
	h.withActor(w, r, func(a string) error {
		x, e := h.service.PublishTemplate(r.Context(), a, chi.URLParam(r, "templateId"))
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) SetDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	h.withActor(w, r, func(a string) error {
		x, e := h.service.SetDefaultTemplate(r.Context(), a, chi.URLParam(r, "templateId"))
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) ArchiveTemplate(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[ArchiveTemplateRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.ArchiveTemplate(r.Context(), a, chi.URLParam(r, "templateId"), q.Reason)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) GetEncounterForm(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.GetEncounterForm(r.Context(), chi.URLParam(r, "encounterId"))
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) SaveEncounterForm(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[SaveEncounterFormRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.SaveEncounterForm(r.Context(), a, chi.URLParam(r, "encounterId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) SubmitEncounterForm(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[SubmitEncounterFormRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.SubmitEncounterForm(r.Context(), a, chi.URLParam(r, "encounterId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
