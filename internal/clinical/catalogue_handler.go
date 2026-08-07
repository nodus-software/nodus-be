package clinical

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"nodus-health/pkg/response"
)

func catalogueFilters(r *http.Request) CatalogueFilters {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	return CatalogueFilters{Query: q.Get("q"), Status: q.Get("status"), Category: q.Get("category"), DepartmentID: q.Get("department_id"), Prescription: q.Get("prescription"), Page: page, PerPage: perPage}
}
func pageValues(f CatalogueFilters) (int, int) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 {
		f.PerPage = 25
	}
	if f.PerPage > 100 {
		f.PerPage = 100
	}
	return f.Page, f.PerPage
}
func (h *Handler) ListCatalogueServices(w http.ResponseWriter, r *http.Request) {
	f := catalogueFilters(r)
	x, total, e := h.service.ListCatalogueServices(r.Context(), f)
	if e != nil {
		h.fail(w, e)
		return
	}
	page, perPage := pageValues(f)
	response.Paginated(w, x, response.NewMeta(page, perPage, total))
}
func (h *Handler) CreateCatalogueService(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[ServiceCatalogueInput](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateCatalogueService(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) UpdateCatalogueService(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[ServiceCatalogueInput](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.UpdateCatalogueService(r.Context(), a, chi.URLParam(r, "itemId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) CatalogueServiceLifecycle(w http.ResponseWriter, r *http.Request) {
	active := chi.URLParam(r, "action") == "reactivate"
	var q struct {
		Reason string `json:"reason"`
	}
	if !active {
		var ok bool
		q, ok = bind[struct {
			Reason string `json:"reason"`
		}](w, r)
		if !ok {
			return
		}
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.SetCatalogueServiceActive(r.Context(), a, chi.URLParam(r, "itemId"), active, q.Reason)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) ListMedicationCatalogue(w http.ResponseWriter, r *http.Request) {
	f := catalogueFilters(r)
	x, total, e := h.service.ListMedicationCatalogue(r.Context(), f)
	if e != nil {
		h.fail(w, e)
		return
	}
	page, perPage := pageValues(f)
	response.Paginated(w, x, response.NewMeta(page, perPage, total))
}
func (h *Handler) CreateMedicationDefinition(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[MedicationCatalogueInput](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CreateMedicationDefinition(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) UpdateMedicationDefinition(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[MedicationCatalogueInput](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.UpdateMedicationDefinition(r.Context(), a, chi.URLParam(r, "itemId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) MedicationLifecycle(w http.ResponseWriter, r *http.Request) {
	active := chi.URLParam(r, "action") == "reactivate"
	var q struct {
		Reason string `json:"reason"`
	}
	if !active {
		var ok bool
		q, ok = bind[struct {
			Reason string `json:"reason"`
		}](w, r)
		if !ok {
			return
		}
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.SetMedicationDefinitionActive(r.Context(), a, chi.URLParam(r, "itemId"), active, q.Reason)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) ListCatalogueReferences(w http.ResponseWriter, r *http.Request) {
	f := catalogueFilters(r)
	x, total, e := h.service.ListCatalogueReferences(r.Context(), chi.URLParam(r, "catalogue"), f)
	if e != nil {
		h.fail(w, e)
		return
	}
	page, perPage := pageValues(f)
	response.Paginated(w, x, response.NewMeta(page, perPage, total))
}
func (h *Handler) AdoptServiceReferences(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[[]ServiceAdoption](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.AdoptServiceReferences(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) AdoptMedicationReferences(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[[]MedicationAdoption](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(a string) error {
		x, e := h.service.AdoptMedicationReferences(r.Context(), a, q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CatalogueImportTemplate(w http.ResponseWriter, r *http.Request) {
	body, e := CatalogueTemplate(chi.URLParam(r, "catalogue"))
	if e != nil {
		h.fail(w, e)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-catalogue-template.csv"`, chi.URLParam(r, "catalogue")))
	_, _ = w.Write([]byte(body))
}
func (h *Handler) PreviewCatalogueImport(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if e := r.ParseMultipartForm(5 << 20); e != nil {
		response.BadRequest(w, "CSV file must be 5 MB or smaller")
		return
	}
	file, _, e := r.FormFile("file")
	if e != nil {
		response.BadRequest(w, "file is required")
		return
	}
	defer file.Close()
	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		mode = "create_only"
	}
	x, e := h.service.PreviewCatalogueImport(r.Context(), a, chi.URLParam(r, "catalogue"), mode, file)
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x)
}
func (h *Handler) CommitCatalogueImport(w http.ResponseWriter, r *http.Request) {
	h.withActor(w, r, func(a string) error {
		x, e := h.service.CommitCatalogueImport(r.Context(), a, chi.URLParam(r, "catalogue"), chi.URLParam(r, "importId"))
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
