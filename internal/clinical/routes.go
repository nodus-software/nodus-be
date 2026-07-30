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
		r.With(middleware.RequirePermission("clinical:write")).Post("/clinical/visits", h.CreateVisit)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/visits/{visitId}", h.GetVisit)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/queues/{queueId}/entries", h.ListEntries)
		r.With(middleware.RequirePermission("queues:manage")).Post("/clinical/queues/{queueId}/entries", h.Enqueue)
		r.With(middleware.RequirePermission("queues:manage")).Post("/clinical/queue-entries/{entryId}/transition", h.Transition)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/queue-entries/{entryId}/history", h.History)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/routing-rules", h.ListRules)
		r.With(middleware.RequirePermission("queues:manage")).Post("/clinical/routing-rules", h.CreateRule)
		r.With(middleware.RequirePermission("clinical:read")).Get("/clinical/terminology/icd10", h.ICD10)
	})
}
