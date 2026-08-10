package clinical

import (
	"context"
	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
	"strings"
	"time"
)

type AuditRecorder interface {
	Record(context.Context, audit.Entry) error
}
type Service struct {
	repo  Repository
	audit AuditRecorder
}

func NewService(r Repository, a AuditRecorder) *Service { return &Service{repo: r, audit: a} }

// Facility structure and workflow resources, plus the prescribing vocabularies
// under Reference data. The last three carry no parent and nothing hangs off
// them — they are code lists the medication catalogue picks from.
var resourceKinds = map[string]bool{"departments": true, "service-points": true, "wards": true, "rooms": true, "beds": true, "queues": true, DosageFormKind: true, RouteKind: true, UnitOfMeasureKind: true, PrescriptionFrequencyKind: true, SpecimenTypeKind: true}

func (s *Service) ListResources(c context.Context, k string) ([]Resource, error) {
	if !resourceKinds[k] {
		return nil, ErrInvalidInput
	}
	return s.repo.ListResources(c, k)
}
func (s *Service) CreateResource(c context.Context, actor, k string, q CreateResourceRequest) (*Resource, error) {
	if !resourceKinds[k] || strings.TrimSpace(q.Code) == "" || strings.TrimSpace(q.Name) == "" {
		return nil, ErrInvalidInput
	}
	// A bed is always a child of a room. Its ward is derived from that room by
	// the repository so callers cannot create a mismatched ward/room hierarchy.
	if k == "beds" && (q.RoomID == nil || strings.TrimSpace(*q.RoomID) == "") {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	code := strings.TrimSpace(q.Code)
	if k == PrescriptionFrequencyKind {
		code = strings.ToUpper(code)
	} else if isVocabularyKind(k) {
		code = strings.ToLower(code)
	}
	r := Resource{ID: id, Code: code, Name: strings.TrimSpace(q.Name), Active: true, DepartmentID: q.DepartmentID, WardID: q.WardID, RoomID: q.RoomID, ServicePointID: q.ServicePointID, Kind: q.Kind}
	x, e := s.repo.CreateResource(c, k, r)
	if e == nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_configuration_created", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"kind": k}})
	}
	return x, e
}

func (s *Service) UpdateResource(c context.Context, actor, k, id string, q UpdateResourceRequest) (*Resource, error) {
	if !resourceKinds[k] || id == "" || (q.Code == nil && q.Name == nil && q.Kind == nil) {
		return nil, ErrInvalidInput
	}
	if q.Code != nil {
		v := strings.TrimSpace(*q.Code)
		if v == "" {
			return nil, ErrInvalidInput
		}
		if k == PrescriptionFrequencyKind {
			v = strings.ToUpper(v)
		} else if isVocabularyKind(k) {
			v = strings.ToLower(v)
		}
		q.Code = &v
	}
	if q.Name != nil {
		v := strings.TrimSpace(*q.Name)
		if v == "" {
			return nil, ErrInvalidInput
		}
		q.Name = &v
	}
	if q.Kind != nil {
		if k != "service-points" || strings.TrimSpace(*q.Kind) == "" {
			return nil, ErrInvalidInput
		}
		v := strings.TrimSpace(*q.Kind)
		q.Kind = &v
	}
	x, err := s.repo.UpdateResource(c, k, id, q)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_configuration_updated", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"kind": k}})
	}
	return x, err
}

func isVocabularyKind(k string) bool {
	return k == DosageFormKind || k == RouteKind || k == UnitOfMeasureKind || k == PrescriptionFrequencyKind || k == SpecimenTypeKind
}

func (s *Service) DeactivationImpact(c context.Context, k, id string) (*DeactivationImpact, error) {
	if !resourceKinds[k] || id == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.DeactivationImpact(c, k, id)
}

func (s *Service) DeactivateResource(c context.Context, actor, k, id string, q DeactivateResourceRequest) (*ResourceLifecycleResult, error) {
	if !resourceKinds[k] || id == "" {
		return nil, ErrInvalidInput
	}
	q.Reason = strings.TrimSpace(q.Reason)
	if q.Reason == "" {
		return nil, ErrReasonRequired
	}
	impact, err := s.repo.DeactivationImpact(c, k, id)
	if err != nil {
		return nil, err
	}
	if len(impact.OperationalBlockers) > 0 {
		return nil, &LifecycleConflictError{Cause: ErrOperationalUse, Impact: impact}
	}
	if len(impact.ActiveDescendants) > 0 && !q.Cascade {
		return nil, &LifecycleConflictError{Cause: ErrActiveDescendants, Impact: impact}
	}
	x, err := s.repo.DeactivateResource(c, k, id, q.Cascade)
	if err == nil && s.audit != nil {
		for _, ref := range x.Affected {
			_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_configuration_deactivated", Result: audit.ResultSuccess, TargetResource: ref.ID, Metadata: map[string]any{"kind": ref.Kind, "reason": q.Reason, "cascade": q.Cascade, "root_id": id, "root_kind": k}})
		}
	}
	return x, err
}

func (s *Service) ReactivateResource(c context.Context, actor, k, id string) (*Resource, error) {
	if !resourceKinds[k] || id == "" {
		return nil, ErrInvalidInput
	}
	x, err := s.repo.ReactivateResource(c, k, id)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_configuration_reactivated", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"kind": k}})
	}
	return x, err
}
func (s *Service) CreateVisit(c context.Context, actor string, q CreateVisitRequest) (*Visit, error) {
	if q.PatientID == "" || !map[string]bool{"test": true, "outpatient": true, "emergency": true, "specialty": true}[q.VisitType] {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	v, e := s.repo.CreateVisit(c, Visit{ID: id, PatientID: q.PatientID, VisitType: q.VisitType, Status: "active", Reason: q.Reason, ServicePointID: q.ServicePointID, CreatedBy: actor})
	if e != nil {
		return nil, e
	}
	if e = s.repo.ApplyVisitRouting(c, *v); e != nil {
		return nil, e
	}
	_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_visit_created", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"visit_type": q.VisitType}})
	return v, nil
}
func (s *Service) GetVisit(c context.Context, id string) (*Visit, error) {
	return s.repo.GetVisit(c, id)
}
func (s *Service) Enqueue(c context.Context, actor, qid string, q EnqueueRequest) (*QueueEntry, error) {
	if qid == "" || q.SubjectID == "" || q.PatientID == "" || !map[string]bool{"visit": true, "admission": true, "order": true}[q.SubjectType] || q.Priority < 0 || q.Priority > 100 {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	var reason *string
	if q.Reason != "" {
		reason = &q.Reason
	}
	x, e := s.repo.CreateQueueEntry(c, QueueEntry{ID: id, QueueID: qid, SubjectType: q.SubjectType, SubjectID: q.SubjectID, PatientID: q.PatientID, Status: "waiting", Priority: q.Priority, Acuity: q.Acuity}, reason, false)
	if e == nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "queue_entry_created", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}

// encounterStartQueueKinds are the service point kinds whose queues take a
// patient into service through a start endpoint rather than a bare transition.
var encounterStartQueueKinds = map[string]bool{"triage": true, "consultation": true}

var allowed = map[string]map[string]bool{"waiting": {"called": true, "in_service": true, "paused": true, "transferred": true, "cancelled": true, "no_show": true}, "called": {"waiting": true, "in_service": true, "paused": true, "transferred": true, "cancelled": true, "no_show": true}, "in_service": {"waiting": true, "paused": true, "transferred": true, "completed": true, "cancelled": true}, "paused": {"waiting": true, "called": true, "in_service": true, "transferred": true, "cancelled": true}}

func (s *Service) Transition(c context.Context, actor, id string, q TransitionRequest) (*QueueEntry, error) {
	x, e := s.repo.GetQueueEntry(c, id)
	if e != nil {
		return nil, e
	}
	if !allowed[x.Status][q.Status] {
		return nil, ErrInvalidTransition
	}
	// Triage and consultation are staffed off an encounter and its pinned form.
	// Flipping the entry to in_service here would take the patient off the board
	// without creating either, so those queues must use their start endpoint.
	if q.Status == "in_service" && encounterStartQueueKinds[x.ServicePointKind] {
		return nil, ErrEncounterStartRequired
	}
	target := x.QueueID
	if q.QueueID != nil && *q.QueueID != "" {
		target = *q.QueueID
	}
	if q.Status == "transferred" && target == x.QueueID {
		return nil, ErrInvalidInput
	}
	if (q.Status == "transferred" || q.Status == "cancelled" || q.Status == "no_show" || q.Priority != nil || q.Position != nil) && strings.TrimSpace(q.Reason) == "" {
		return nil, ErrReasonRequired
	}
	if q.Priority != nil {
		if *q.Priority < 0 || *q.Priority > 100 {
			return nil, ErrInvalidInput
		}
		x.Priority = *q.Priority
	}
	x.PositionOverride = q.Position
	var reason *string
	if q.Reason != "" {
		reason = &q.Reason
	}
	y, e := s.repo.TransitionQueueEntry(c, *x, q.Status, target, &actor, reason, false)
	if e == nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "queue_entry_transitioned", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"from": x.Status, "to": q.Status, "reason": q.Reason}})
	}
	return y, e
}

func (s *Service) StartTriage(c context.Context, actor, entryID string) (*EncounterStart, error) {
	return s.startEncounter(c, actor, entryID, "triage")
}

func (s *Service) StartConsultation(c context.Context, actor, entryID string) (*EncounterStart, error) {
	return s.startEncounter(c, actor, entryID, "consultation")
}

// startEncounter takes a called patient into service. The encounter, its form
// and the queue transition are created together by the repository so a clinician
// never ends up with an entry in service and no page to work on.
func (s *Service) startEncounter(c context.Context, actor, entryID, encounterType string) (*EncounterStart, error) {
	if strings.TrimSpace(entryID) == "" {
		return nil, ErrInvalidInput
	}
	encounterID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	formID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	x, err := s.repo.StartEncounter(c, entryID, Encounter{
		ID:            encounterID,
		EncounterType: encounterType,
		Status:        "in_progress",
		ClinicianID:   &actor,
		StartedAt:     &now,
	}, formID, actor)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "outpatient_" + encounterType + "_started", Result: audit.ResultSuccess, TargetResource: encounterID, Metadata: map[string]any{"queue_entry_id": entryID, "visit_id": x.Encounter.VisitID}})
	}
	return x, err
}
func (s *Service) ListQueueEntries(c context.Context, q string) ([]QueueEntry, error) {
	return s.repo.ListQueueEntries(c, q)
}
func (s *Service) ListQueueHistory(c context.Context, id string) ([]QueueHistory, error) {
	return s.repo.ListQueueHistory(c, id)
}
func (s *Service) ListRoutingRules(c context.Context) ([]RoutingRule, error) {
	return s.repo.ListRoutingRules(c)
}
func (s *Service) CreateRoutingRule(c context.Context, actor string, q CreateRoutingRuleRequest) (*RoutingRule, error) {
	if strings.TrimSpace(q.Name) == "" || !map[string]bool{"visit.created": true, "encounter.completed": true, "order.created": true, "order.review_ready": true}[q.EventType] || q.TargetQueueID == "" || q.Priority < 0 || q.Priority > 100 {
		return nil, ErrInvalidInput
	}
	if q.VisitType != nil && !map[string]bool{"test": true, "outpatient": true, "emergency": true, "specialty": true}[*q.VisitType] {
		return nil, ErrInvalidInput
	}
	if q.EncounterType != nil && !map[string]bool{"triage": true, "consultation": true, "ward_round": true, "nursing": true, "other": true}[*q.EncounterType] {
		return nil, ErrInvalidInput
	}
	if q.OrderKind != nil && !map[string]bool{"service": true, "medication": true}[*q.OrderKind] {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	x, e := s.repo.CreateRoutingRule(c, RoutingRule{ID: id, Name: q.Name, EventType: q.EventType, VisitType: q.VisitType, EncounterType: q.EncounterType, OrderKind: q.OrderKind, ServiceCategory: q.ServiceCategory, TargetQueueID: q.TargetQueueID, Priority: q.Priority, Active: true})
	if e == nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "queue_routing_rule_created", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}
func (s *Service) SearchICD10(c context.Context, q string) ([]Concept, error) {
	if len(strings.TrimSpace(q)) < 2 {
		return []Concept{}, nil
	}
	return s.repo.SearchConcepts(c, q, 20)
}
