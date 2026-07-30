package clinical

import (
	"context"
	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
	"strings"
)

type AuditRecorder interface {
	Record(context.Context, audit.Entry) error
}
type Service struct {
	repo  Repository
	audit AuditRecorder
}

func NewService(r Repository, a AuditRecorder) *Service { return &Service{repo: r, audit: a} }

var resourceKinds = map[string]bool{"departments": true, "service-points": true, "wards": true, "rooms": true, "beds": true, "queues": true}

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
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	r := Resource{ID: id, Code: strings.TrimSpace(q.Code), Name: strings.TrimSpace(q.Name), Active: true, DepartmentID: q.DepartmentID, WardID: q.WardID, RoomID: q.RoomID, ServicePointID: q.ServicePointID, Kind: q.Kind}
	x, e := s.repo.CreateResource(c, k, r)
	if e == nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_configuration_created", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"kind": k}})
	}
	return x, e
}
func (s *Service) CreateVisit(c context.Context, actor string, q CreateVisitRequest) (*Visit, error) {
	if q.PatientID == "" || !map[string]bool{"test": true, "outpatient": true, "emergency": true, "specialty": true}[q.VisitType] {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	v, e := s.repo.CreateVisit(c, Visit{ID: id, PatientID: q.PatientID, VisitType: q.VisitType, Status: "active", Reason: q.Reason, CreatedBy: actor})
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

var allowed = map[string]map[string]bool{"waiting": {"called": true, "in_service": true, "paused": true, "transferred": true, "cancelled": true, "no_show": true}, "called": {"waiting": true, "in_service": true, "paused": true, "transferred": true, "cancelled": true, "no_show": true}, "in_service": {"waiting": true, "paused": true, "transferred": true, "completed": true, "cancelled": true}, "paused": {"waiting": true, "called": true, "in_service": true, "transferred": true, "cancelled": true}}

func (s *Service) Transition(c context.Context, actor, id string, q TransitionRequest) (*QueueEntry, error) {
	x, e := s.repo.GetQueueEntry(c, id)
	if e != nil {
		return nil, e
	}
	if !allowed[x.Status][q.Status] {
		return nil, ErrInvalidTransition
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
	if strings.TrimSpace(q.Name) == "" || q.EventType != "visit.created" || q.TargetQueueID == "" || q.Priority < 0 || q.Priority > 100 {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	x, e := s.repo.CreateRoutingRule(c, RoutingRule{ID: id, Name: q.Name, EventType: q.EventType, VisitType: q.VisitType, TargetQueueID: q.TargetQueueID, Priority: q.Priority, Active: true})
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
