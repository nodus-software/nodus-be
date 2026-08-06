package clinical

import (
	"github.com/go-chi/chi/v5"
	"nodus-health/internal/middleware"
)

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/config/{kind}", h.ListResources)
		r.With(middleware.RequirePermission("facilities:manage")).Post("/clinical/config/{kind}", h.CreateResource)
		r.With(middleware.RequirePermission("facilities:manage")).Patch("/clinical/config/{kind}/{resourceId}", h.UpdateResource)
		r.With(middleware.RequirePermission("facilities:manage")).Get("/clinical/config/{kind}/{resourceId}/deactivation-impact", h.DeactivationImpact)
		r.With(middleware.RequirePermission("facilities:manage")).Post("/clinical/config/{kind}/{resourceId}/deactivate", h.DeactivateResource)
		r.With(middleware.RequirePermission("facilities:manage")).Post("/clinical/config/{kind}/{resourceId}/reactivate", h.ReactivateResource)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/visits", h.CreateVisit)
		r.With(middleware.RequirePermission("outpatient:check-in")).Post("/clinical/outpatient/check-ins", h.OutpatientCheckIn)
		r.With(middleware.RequirePermission("outpatient:check-in"), middleware.RequirePermission("outpatient:duplicate-override")).Post("/clinical/outpatient/check-ins/override", h.OutpatientCheckInOverride)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/outpatient/visits", h.ListOutpatientVisits)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/visits/{visitId}", h.GetVisit)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/visits/{visitId}/summary", h.VisitSummary)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/visits/{visitId}/encounters", h.CreateEncounter)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/visits/{visitId}/encounters", h.ListEncounters)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/encounters/{encounterId}/observations", h.RecordObservations)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/encounters/{encounterId}/complete", h.CompleteEncounter)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/visits/{visitId}/notes", h.CreateNote)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/visits/{visitId}/diagnoses", h.CreateDiagnosis)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/patients/{patientId}/allergies", h.CreateAllergy)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/patients/{patientId}/allergies", h.ListAllergies)
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/visits/{visitId}/complete", h.CompleteVisit)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/queues/{queueId}/entries", h.ListEntries)
		r.With(middleware.RequirePermission("queues:manage")).Post("/clinical/queues/{queueId}/entries", h.Enqueue)
		r.With(middleware.RequirePermission("queues:manage")).Post("/clinical/queue-entries/{entryId}/transition", h.Transition)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/queue-entries/{entryId}/history", h.History)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/routing-rules", h.ListRules)
		r.With(middleware.RequirePermission("queues:manage")).Post("/clinical/routing-rules", h.CreateRule)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/terminology/icd10", h.ICD10)
	})
}
