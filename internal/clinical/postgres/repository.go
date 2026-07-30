package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/clinical"
	"nodus-health/internal/platform/db"
	"nodus-health/pkg/utility"
)

type Repository struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Repository { return &Repository{pool: p} }
func (r *Repository) exec(c context.Context) db.DBTX {
	if x, ok := db.Executor(c); ok {
		return x
	}
	return r.pool
}

func resourceSpec(k string) (table, selectCols string, err error) {
	switch k {
	case "departments":
		return "departments", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "service-points":
		return "service_points", "id,code,name,active,department_id,NULL::uuid,NULL::uuid,NULL::uuid,kind,NULL::text,created_at", nil
	case "wards":
		return "wards", "id,code,name,active,department_id,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "rooms":
		return "rooms", "id,code,name,active,NULL::uuid,ward_id,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "beds":
		return "beds", "id,code,name,active,NULL::uuid,ward_id,room_id,NULL::uuid,NULL::text,status,created_at", nil
	case "queues":
		return "queues", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,service_point_id,NULL::text,NULL::text,created_at", nil
	default:
		return "", "", clinical.ErrInvalidInput
	}
}
func scanResource(row pgx.Row) (*clinical.Resource, error) {
	var x clinical.Resource
	err := row.Scan(&x.ID, &x.Code, &x.Name, &x.Active, &x.DepartmentID, &x.WardID, &x.RoomID, &x.ServicePointID, &x.Kind, &x.Status, &x.CreatedAt)
	return &x, err
}
func (r *Repository) ListResources(c context.Context, k string) ([]clinical.Resource, error) {
	t, cols, e := resourceSpec(k)
	if e != nil {
		return nil, e
	}
	rows, e := r.exec(c).Query(c, "SELECT "+cols+" FROM "+t+" ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Resource{}
	for rows.Next() {
		x, e := scanResource(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}
func (r *Repository) CreateResource(c context.Context, k string, x clinical.Resource) (*clinical.Resource, error) {
	_, cols, e := resourceSpec(k)
	if e != nil {
		return nil, e
	}
	var q string
	var args []any
	switch k {
	case "departments":
		q = "INSERT INTO departments(id,code,name) VALUES($1,$2,$3) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name}
	case "service-points":
		if x.DepartmentID == nil {
			return nil, clinical.ErrInvalidInput
		}
		kind := "other"
		if x.Kind != nil {
			kind = *x.Kind
		}
		q = "INSERT INTO service_points(id,code,name,department_id,kind) VALUES($1,$2,$3,$4,$5) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.DepartmentID, kind}
	case "wards":
		if x.DepartmentID == nil {
			return nil, clinical.ErrInvalidInput
		}
		q = "INSERT INTO wards(id,code,name,department_id) VALUES($1,$2,$3,$4) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.DepartmentID}
	case "rooms":
		if x.WardID == nil {
			return nil, clinical.ErrInvalidInput
		}
		q = "INSERT INTO rooms(id,code,name,ward_id) VALUES($1,$2,$3,$4) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.WardID}
	case "beds":
		if x.WardID == nil {
			return nil, clinical.ErrInvalidInput
		}
		q = "INSERT INTO beds(id,code,name,ward_id,room_id) VALUES($1,$2,$3,$4,$5) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.WardID, x.RoomID}
	case "queues":
		if x.ServicePointID == nil {
			return nil, clinical.ErrInvalidInput
		}
		q = "INSERT INTO queues(id,code,name,service_point_id) VALUES($1,$2,$3,$4) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.ServicePointID}
	default:
		return nil, clinical.ErrInvalidInput
	}
	return scanResource(r.exec(c).QueryRow(c, q, args...))
}

func scanVisit(row pgx.Row) (*clinical.Visit, error) {
	var x clinical.Visit
	err := row.Scan(&x.ID, &x.PatientID, &x.VisitType, &x.Status, &x.Reason, &x.StartedAt, &x.EndedAt, &x.CreatedBy, &x.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, err
}

const visitCols = "id,patient_id,visit_type::text,status::text,reason,started_at,ended_at,created_by,created_at"

func (r *Repository) CreateVisit(c context.Context, x clinical.Visit) (*clinical.Visit, error) {
	return scanVisit(r.exec(c).QueryRow(c, "INSERT INTO clinical_visits(id,patient_id,visit_type,status,reason,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+visitCols, x.ID, x.PatientID, x.VisitType, x.Status, x.Reason, x.CreatedBy))
}
func (r *Repository) GetVisit(c context.Context, id string) (*clinical.Visit, error) {
	return scanVisit(r.exec(c).QueryRow(c, "SELECT "+visitCols+" FROM clinical_visits WHERE id=$1", id))
}
func (r *Repository) ApplyVisitRouting(c context.Context, v clinical.Visit) error {
	outboxID, e := utility.GenerateUUID()
	if e != nil {
		return e
	}
	_, e = r.exec(c).Exec(c, "INSERT INTO clinical_outbox(id,event_type,aggregate_type,aggregate_id,payload,status,processed_at) VALUES($1,'visit.created','visit',$2,jsonb_build_object('patient_id',$3),'processed',now())", outboxID, v.ID, v.PatientID)
	if e != nil {
		return e
	}
	rows, e := r.exec(c).Query(c, "SELECT target_queue_id,priority FROM queue_routing_rules WHERE active AND event_type='visit.created' AND (visit_type IS NULL OR visit_type=$1::clinical_visit_type)", v.VisitType)
	if e != nil {
		return e
	}
	defer rows.Close()
	type rule struct {
		q string
		p int16
	}
	var rules []rule
	for rows.Next() {
		var x rule
		if e = rows.Scan(&x.q, &x.p); e != nil {
			return e
		}
		rules = append(rules, x)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	for _, rule := range rules {
		id, _ := utility.GenerateUUID()
		hist, _ := utility.GenerateUUID()
		tag, e := r.exec(c).Exec(c, "INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,priority) VALUES($1,$2,'visit',$3,$4,'waiting',$5) ON CONFLICT DO NOTHING", id, rule.q, v.ID, v.PatientID, rule.p)
		if e != nil {
			return e
		}
		if tag.RowsAffected() > 0 {
			_, e = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,to_status,to_queue_id,automated,reason) VALUES($1,$2,'waiting',$3,true,'Automatic visit routing')", hist, id, rule.q)
			if e != nil {
				return e
			}
		}
	}
	return nil
}

const entrySelect = "SELECT e.id,e.queue_id,q.name,e.subject_type::text,e.subject_id,e.patient_id,p.full_name,p.mrn,e.status::text,e.priority,e.acuity,e.position_override,e.joined_at,e.updated_at FROM queue_entries e JOIN queues q ON q.id=e.queue_id JOIN patients p ON p.id=e.patient_id"

func scanEntry(row pgx.Row) (*clinical.QueueEntry, error) {
	var x clinical.QueueEntry
	e := row.Scan(&x.ID, &x.QueueID, &x.QueueName, &x.SubjectType, &x.SubjectID, &x.PatientID, &x.PatientName, &x.PatientMRN, &x.Status, &x.Priority, &x.Acuity, &x.PositionOverride, &x.JoinedAt, &x.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, e
}
func (r *Repository) ListQueueEntries(c context.Context, q string) ([]clinical.QueueEntry, error) {
	rows, e := r.exec(c).Query(c, entrySelect+" WHERE e.queue_id=$1 AND e.status IN ('waiting','called','in_service','paused') ORDER BY e.priority DESC,e.position_override NULLS LAST,e.joined_at", q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.QueueEntry{}
	for rows.Next() {
		x, e := scanEntry(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}
func (r *Repository) GetQueueEntry(c context.Context, id string) (*clinical.QueueEntry, error) {
	return scanEntry(r.exec(c).QueryRow(c, entrySelect+" WHERE e.id=$1", id))
}
func (r *Repository) CreateQueueEntry(c context.Context, x clinical.QueueEntry, reason *string, auto bool) (*clinical.QueueEntry, error) {
	_, e := r.exec(c).Exec(c, "INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,priority,acuity) VALUES($1,$2,$3,$4,$5,'waiting',$6,$7)", x.ID, x.QueueID, x.SubjectType, x.SubjectID, x.PatientID, x.Priority, x.Acuity)
	if e != nil {
		if strings.Contains(e.Error(), "uq_active_queue_subject") {
			return nil, clinical.ErrConflict
		}
		return nil, e
	}
	h, _ := utility.GenerateUUID()
	_, e = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,to_status,to_queue_id,reason,automated) VALUES($1,$2,'waiting',$3,$4,$5)", h, x.ID, x.QueueID, reason, auto)
	if e != nil {
		return nil, e
	}
	return r.GetQueueEntry(c, x.ID)
}
func (r *Repository) TransitionQueueEntry(c context.Context, x clinical.QueueEntry, status, target string, actor, reason *string, auto bool) (*clinical.QueueEntry, error) {
	persist := status
	if status == "transferred" {
		persist = "waiting"
	}
	tag, e := r.exec(c).Exec(c, "UPDATE queue_entries SET queue_id=$2,status=$3,priority=$4,position_override=$5,called_at=CASE WHEN $3='called' THEN now() ELSE called_at END,service_started_at=CASE WHEN $3='in_service' THEN now() ELSE service_started_at END,completed_at=CASE WHEN $3='completed' THEN now() ELSE completed_at END WHERE id=$1 AND status=$6", x.ID, target, persist, x.Priority, x.PositionOverride, x.Status)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	h, _ := utility.GenerateUUID()
	_, e = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,from_status,to_status,from_queue_id,to_queue_id,actor_id,reason,automated) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", h, x.ID, x.Status, persist, x.QueueID, target, actor, reason, auto)
	if e != nil {
		return nil, e
	}
	return r.GetQueueEntry(c, x.ID)
}
func (r *Repository) ListQueueHistory(c context.Context, id string) ([]clinical.QueueHistory, error) {
	rows, e := r.exec(c).Query(c, "SELECT id,from_status::text,to_status::text,from_queue_id,to_queue_id,actor_id,reason,automated,occurred_at FROM queue_entry_history WHERE queue_entry_id=$1 ORDER BY occurred_at", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.QueueHistory{}
	for rows.Next() {
		var x clinical.QueueHistory
		if e = rows.Scan(&x.ID, &x.FromStatus, &x.ToStatus, &x.FromQueueID, &x.ToQueueID, &x.ActorID, &x.Reason, &x.Automated, &x.OccurredAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) ListRoutingRules(c context.Context) ([]clinical.RoutingRule, error) {
	rows, e := r.exec(c).Query(c, "SELECT id,name,event_type,visit_type::text,target_queue_id,priority,active FROM queue_routing_rules ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.RoutingRule{}
	for rows.Next() {
		var x clinical.RoutingRule
		if e = rows.Scan(&x.ID, &x.Name, &x.EventType, &x.VisitType, &x.TargetQueueID, &x.Priority, &x.Active); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) CreateRoutingRule(c context.Context, x clinical.RoutingRule) (*clinical.RoutingRule, error) {
	e := r.exec(c).QueryRow(c, "INSERT INTO queue_routing_rules(id,name,event_type,visit_type,target_queue_id,priority) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,event_type,visit_type::text,target_queue_id,priority,active", x.ID, x.Name, x.EventType, x.VisitType, x.TargetQueueID, x.Priority).Scan(&x.ID, &x.Name, &x.EventType, &x.VisitType, &x.TargetQueueID, &x.Priority, &x.Active)
	return &x, e
}
func (r *Repository) SearchConcepts(c context.Context, q string, limit int) ([]clinical.Concept, error) {
	like := "%" + q + "%"
	rows, e := r.exec(c).Query(c, "SELECT c.code,c.display,r.version FROM terminology_concepts c JOIN terminology_releases r ON r.id=c.release_id WHERE r.system='ICD-10' AND r.active AND c.active AND (c.code ILIKE $1 OR c.searchable_text ILIKE $1) ORDER BY CASE WHEN c.code ILIKE $2 THEN 0 ELSE 1 END,c.code LIMIT $3", like, q+"%", limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Concept{}
	for rows.Next() {
		var x clinical.Concept
		if e = rows.Scan(&x.Code, &x.Display, &x.Version); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
