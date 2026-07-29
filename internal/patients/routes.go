package patients

import (
	"github.com/go-chi/chi/v5"

	"nodus-health/internal/middleware"
)

// RegisterRoutes attaches the Patient Management endpoints from the
// contract to r. Cross-facility/cross-tenant search is a separate feature
// and is not part of this domain.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))

		r.With(middleware.RequirePermission("patients:read")).Get("/patients", h.ListPatients)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/export", h.ExportPatients)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/duplicate-check", h.DuplicateCheck)
		r.With(middleware.RequirePermission("patients:write")).Post("/patients", h.RegisterPatient)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/{patientId}", h.GetPatient)
		r.With(middleware.RequirePermission("patients:write")).Patch("/patients/{patientId}/contact", h.UpdateContact)
		r.With(middleware.RequirePermission("patients:write")).Post("/patients/{patientId}/mark-deceased", h.MarkDeceased)
		r.With(middleware.RequirePermission("patients:write")).Post("/patients/{patientId}/corrections", h.SubmitCorrection)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/{patientId}/corrections", h.ListCorrections)
		r.With(middleware.RequirePermission("patients:write")).Post("/patients/corrections/{correctionId}/action", h.DecideCorrection)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/{patientId}/identifiers", h.ListIdentifiers)
		r.With(middleware.RequirePermission("patients:write")).Post("/patients/{patientId}/identifiers", h.AddIdentifier)
		r.With(middleware.RequirePermission("patients:write")).Delete("/patients/{patientId}/identifiers/{identifierId}", h.RemoveIdentifier)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/{patientId}/consents", h.ListConsents)
		r.With(middleware.RequirePermission("patients:write")).Put("/patients/{patientId}/consents/{scope}", h.SetConsent)
		r.With(middleware.RequirePermission("patients:read")).Get("/patients/{patientId}/activity", h.ListActivity)
		r.With(middleware.RequirePermission("patients:write")).Post("/patients/{patientId}/activity", h.AddActivityNote)
		r.With(middleware.RequirePermission("patients:merge")).Post("/patients/merge", h.MergePatients)
	})
}
