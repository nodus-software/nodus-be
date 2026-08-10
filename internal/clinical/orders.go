package clinical

import (
	"context"
	"strings"

	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
)

func (s *Service) CreateOrder(c context.Context, actor, visitID string, q CreateOrderRequest) (*ClinicalOrder, error) {
	if visitID == "" || q.EncounterID == "" || !map[string]bool{"service": true, "medication": true}[q.Kind] || q.Priority < 0 || q.Priority > 100 {
		return nil, ErrInvalidInput
	}
	v, e := s.repo.GetVisit(c, visitID)
	if e != nil {
		return nil, e
	}
	if v.Status != "active" {
		return nil, ErrInvalidTransition
	}
	enc, e := s.repo.GetEncounter(c, q.EncounterID)
	if e != nil {
		return nil, e
	}
	if enc.VisitID != visitID || enc.Status != "in_progress" || enc.EncounterType != "consultation" {
		return nil, ErrInvalidTransition
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	x := ClinicalOrder{ID: id, PatientID: v.PatientID, VisitID: visitID, EncounterID: q.EncounterID, Kind: q.Kind, Priority: q.Priority, ReviewRequired: q.ReviewRequired, OrderedBy: actor}
	if q.Kind == "service" {
		if strings.TrimSpace(q.ServiceID) == "" {
			return nil, ErrInvalidInput
		}
		x.Service = &ServiceOrderDetail{ServiceID: q.ServiceID}
	}
	if q.Kind == "medication" {
		if q.MedicationID == "" || q.Dose == nil || *q.Dose <= 0 || strings.TrimSpace(q.DoseUnit) == "" || strings.TrimSpace(q.Route) == "" || strings.TrimSpace(q.Frequency) == "" || (q.DurationDays == nil && q.Quantity == nil) {
			return nil, ErrInvalidInput
		}
		allergic, e := s.repo.HasActiveMedicationAllergy(c, v.PatientID)
		if e != nil {
			return nil, e
		}
		reason := strings.TrimSpace(value(q.AllergyOverrideReason))
		if allergic && reason == "" {
			return nil, ErrReasonRequired
		}
		if reason != "" {
			q.AllergyOverrideReason = &reason
		}
		x.Medication = &MedicationOrderDetail{MedicationID: q.MedicationID, Dose: *q.Dose, DoseUnit: strings.TrimSpace(q.DoseUnit), Route: strings.TrimSpace(q.Route), Frequency: strings.TrimSpace(q.Frequency), DurationDays: q.DurationDays, Quantity: q.Quantity, Instructions: q.Instructions, AllergyOverrideReason: q.AllergyOverrideReason}
	}
	created, e := s.repo.CreateOrder(c, x)
	if e != nil {
		return nil, e
	}
	if e = s.repo.ApplyEventRouting(c, "order.created", "order", created.ID, created.PatientID, v.VisitType, "", &actor); e != nil {
		return nil, e
	}
	if s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_order_created", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"kind": created.Kind, "category": created.Category, "visit_id": visitID, "allergy_override": created.Medication != nil && created.Medication.AllergyOverrideReason != nil}})
	}
	return created, nil
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (s *Service) ListOrders(c context.Context, f OrderFilters) (*OrderPage, error) {
	items, total, e := s.repo.ListOrders(c, f)
	if e != nil {
		return nil, e
	}
	page, per := f.Page, f.PerPage
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 25
	}
	pages := (total + per - 1) / per
	if pages < 1 {
		pages = 1
	}
	return &OrderPage{Data: items, Page: page, PerPage: per, Total: total, TotalPages: pages}, nil
}
func (s *Service) TransitionOrder(c context.Context, actor, id string, q TransitionOrderRequest) (*ClinicalOrder, error) {
	x, e := s.repo.GetOrder(c, id)
	if e != nil {
		return nil, e
	}
	allowed := map[string]map[string]bool{"ordered": {"accepted": true, "cancelled": true, "rejected": true}, "accepted": {"in_progress": true, "cancelled": true, "rejected": true}, "in_progress": {"completed": true, "cancelled": true, "rejected": true}}
	if !allowed[x.Status][q.Status] || q.ExpectedVersion != x.Version {
		return nil, ErrConflict
	}
	reason := strings.TrimSpace(q.Reason)
	if (q.Status == "cancelled" || q.Status == "rejected") && reason == "" {
		return nil, ErrReasonRequired
	}
	y, e := s.repo.TransitionOrder(c, *x, q.Status, reason)
	if e != nil {
		return nil, e
	}
	if y.ReviewRequired && map[string]bool{"completed": true, "cancelled": true, "rejected": true}[y.Status] {
		pending, e := s.repo.HasUnresolvedReviewOrders(c, y.VisitID)
		if e != nil {
			return nil, e
		}
		if !pending {
			v, e := s.repo.GetVisit(c, y.VisitID)
			if e != nil {
				return nil, e
			}
			if e = s.repo.ApplyEventRouting(c, "order.review_ready", "visit", y.VisitID, y.PatientID, v.VisitType, "", &actor); e != nil {
				return nil, e
			}
		}
	}
	if s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_order_transitioned", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"from": x.Status, "to": q.Status, "reason": reason}})
	}
	return y, nil
}
