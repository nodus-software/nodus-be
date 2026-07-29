package patients

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	var dupErr *DuplicateError
	switch {
	case errors.As(err, &dupErr):
		candidates := make([]DuplicateCandidateResponse, 0, len(dupErr.Candidates))
		for _, c := range dupErr.Candidates {
			candidates = append(candidates, toDuplicateCandidateResponse(c))
		}
		response.ErrorWithDetails(w, http.StatusConflict, "CONFLICT", err.Error(), map[string]any{"candidates": candidates})
	case errors.Is(err, ErrPatientNotFound), errors.Is(err, ErrGuardianNotFound),
		errors.Is(err, ErrIdentifierNotFound), errors.Is(err, ErrCorrectionNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, ErrCannotMergeSelf), errors.Is(err, ErrAlreadyMerged), errors.Is(err, ErrCorrectionAlreadyDecided):
		response.Conflict(w, err.Error())
	case errors.Is(err, ErrInvalidGuardianSelection), errors.Is(err, ErrInvalidDate):
		response.Validation(w, map[string]string{"error": err.Error()})
	default:
		h.log.Error("unexpected patients domain error", "error", err.Error())
		response.Internal(w)
	}
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (h *Handler) ListPatients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ListPatientsFilter{
		Status:    splitCSV(q.Get("status")),
		Gender:    splitCSV(q.Get("gender")),
		Insurance: splitCSV(q.Get("insurance")),
		Page:      1,
		PerPage:   20,
	}
	if v := q.Get("q"); v != "" {
		filter.Q = &v
	}
	if v := q.Get("regFrom"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			filter.RegFrom = &t
		}
	}
	if v := q.Get("regTo"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			filter.RegTo = &t
		}
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Page = n
		}
	}
	if v := q.Get("perPage"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.PerPage = n
		}
	}

	list, total, err := h.service.ListPatients(r.Context(), filter)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Paginated(w, list, response.NewMeta(filter.Page, filter.PerPage, total))
}

func (h *Handler) DuplicateCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fullName := q.Get("full_name")
	if fullName == "" {
		response.Validation(w, map[string]string{"full_name": "full_name is required"})
		return
	}
	var dob *time.Time
	if v := q.Get("dob"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			dob = &t
		}
	}
	var nationalID, phone *string
	if v := q.Get("national_id"); v != "" {
		nationalID = &v
	}
	if v := q.Get("phone"); v != "" {
		phone = &v
	}

	candidates, err := h.service.DuplicateCheck(r.Context(), fullName, dob, nationalID, phone)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, candidates)
}

func (h *Handler) RegisterPatient(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[RegisterPatientRequest](w, r)
	if !ok {
		return
	}
	patient, err := h.service.RegisterPatient(r.Context(), ac.UserID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, patient)
}

func (h *Handler) GetPatient(w http.ResponseWriter, r *http.Request) {
	patient, err := h.service.GetPatient(r.Context(), chi.URLParam(r, "patientId"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, patient)
}

func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[struct {
		Phone   string `json:"phone"`
		Address string `json:"address"`
	}](w, r)
	if !ok {
		return
	}
	patient, err := h.service.UpdateContact(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), req.Phone, req.Address)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, patient)
}

func (h *Handler) MarkDeceased(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[MarkDeceasedRequest](w, r)
	if !ok {
		return
	}
	patient, err := h.service.MarkDeceased(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, patient)
}

func (h *Handler) SubmitCorrection(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[SubmitCorrectionRequest](w, r)
	if !ok {
		return
	}
	correction, err := h.service.SubmitCorrection(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, correction)
}

func (h *Handler) ListCorrections(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListCorrections(r.Context(), chi.URLParam(r, "patientId"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, list)
}

func (h *Handler) DecideCorrection(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[DecideCorrectionRequest](w, r)
	if !ok {
		return
	}
	correction, err := h.service.DecideCorrection(r.Context(), ac.UserID, chi.URLParam(r, "correctionId"), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, correction)
}

func (h *Handler) ListIdentifiers(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListIdentifiers(r.Context(), chi.URLParam(r, "patientId"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, list)
}

func (h *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[AddIdentifierRequest](w, r)
	if !ok {
		return
	}
	identifier, err := h.service.AddIdentifier(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, identifier)
}

func (h *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	if err := h.service.RemoveIdentifier(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), chi.URLParam(r, "identifierId")); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) ListConsents(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListConsents(r.Context(), chi.URLParam(r, "patientId"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, list)
}

func (h *Handler) SetConsent(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[SetConsentRequest](w, r)
	if !ok {
		return
	}
	consent, err := h.service.SetConsent(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), chi.URLParam(r, "scope"), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, consent)
}

func (h *Handler) ListActivity(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListActivity(r.Context(), chi.URLParam(r, "patientId"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, list)
}

func (h *Handler) AddActivityNote(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[AddActivityNoteRequest](w, r)
	if !ok {
		return
	}
	entry, err := h.service.AddActivityNote(r.Context(), ac.UserID, chi.URLParam(r, "patientId"), req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Created(w, entry)
}

func (h *Handler) MergePatients(w http.ResponseWriter, r *http.Request) {
	ac, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}
	req, ok := bindJSON[MergePatientsRequest](w, r)
	if !ok {
		return
	}
	patient, err := h.service.MergePatients(r.Context(), ac.UserID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.OK(w, patient)
}

func (h *Handler) ExportPatients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("format")
	if format != "csv" {
		response.Validation(w, map[string]string{"format": "only format=csv is currently supported"})
		return
	}
	filter := ListPatientsFilter{
		Status: splitCSV(q.Get("status")), Gender: splitCSV(q.Get("gender")), Insurance: splitCSV(q.Get("insurance")),
		Page: 1, PerPage: exportRowCap,
	}
	if v := q.Get("q"); v != "" {
		filter.Q = &v
	}
	if v := q.Get("regFrom"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			filter.RegFrom = &t
		}
	}
	if v := q.Get("regTo"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			filter.RegTo = &t
		}
	}

	list, _, err := h.service.ListPatients(r.Context(), filter)
	if err != nil {
		h.writeError(w, err)
		return
	}

	ac, authed := middleware.AuthFromContext(r.Context())
	var actor *string
	if authed {
		actor = &ac.UserID
	}
	_ = h.service.audit.Record(r.Context(), auditExportEntry(actor, len(list)))

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="patients.csv"`)
	writeCSV(w, list)
}
