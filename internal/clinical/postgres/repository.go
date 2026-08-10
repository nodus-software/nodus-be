package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	case "dosage-forms":
		return "medication_dosage_forms", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "routes":
		return "administration_routes", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "units-of-measure":
		return "units_of_measure", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "prescription-frequencies":
		return "prescription_frequencies", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
	case "specimen-types":
		return "specimen_types", "id,code,name,active,NULL::uuid,NULL::uuid,NULL::uuid,NULL::uuid,NULL::text,NULL::text,created_at", nil
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
	table, cols, e := resourceSpec(k)
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
		if x.RoomID == nil {
			return nil, clinical.ErrInvalidInput
		}
		// Derive ward_id from the selected room. This makes room_id the single
		// source of truth and prevents a bed from referencing a room in another ward.
		q = "INSERT INTO beds(id,code,name,ward_id,room_id) SELECT $1,$2,$3,ward_id,id FROM rooms WHERE id=$4 RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.RoomID}
	case "queues":
		if x.ServicePointID == nil {
			return nil, clinical.ErrInvalidInput
		}
		q = "INSERT INTO queues(id,code,name,service_point_id) VALUES($1,$2,$3,$4) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name, *x.ServicePointID}
	case "dosage-forms", "routes", "units-of-measure", "prescription-frequencies", "specimen-types":
		// Prescribing vocabularies are flat code/name lists — no parent to resolve.
		q = "INSERT INTO " + table + "(id,code,name) VALUES($1,$2,$3) RETURNING " + cols
		args = []any{x.ID, x.Code, x.Name}
	default:
		return nil, clinical.ErrInvalidInput
	}
	created, err := scanResource(r.exec(c).QueryRow(c, q, args...))
	if k == "beds" && errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrInvalidInput
	}
	return created, err
}

func normalizeResourceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return clinical.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return clinical.ErrConflict
	}
	return err
}

func (r *Repository) UpdateResource(c context.Context, k, id string, in clinical.UpdateResourceRequest) (*clinical.Resource, error) {
	table, cols, err := resourceSpec(k)
	if err != nil {
		return nil, err
	}
	if in.Kind != nil && k != "service-points" {
		return nil, clinical.ErrInvalidInput
	}
	q := "UPDATE " + table + " SET code=COALESCE($2,code),name=COALESCE($3,name)"
	args := []any{id, in.Code, in.Name}
	if k == "service-points" {
		q += ",kind=COALESCE($4,kind)"
		args = append(args, in.Kind)
	}
	q += ",updated_at=now() WHERE id=$1 RETURNING " + cols
	x, err := scanResource(r.exec(c).QueryRow(c, q, args...))
	return x, normalizeResourceError(err)
}

func resourceRef(c context.Context, exec db.DBTX, k, id string) (clinical.ResourceReference, bool, error) {
	table, _, err := resourceSpec(k)
	if err != nil {
		return clinical.ResourceReference{}, false, err
	}
	var ref clinical.ResourceReference
	var active bool
	err = exec.QueryRow(c, "SELECT id,name,active FROM "+table+" WHERE id=$1 FOR UPDATE", id).Scan(&ref.ID, &ref.Name, &active)
	ref.Kind = k
	if errors.Is(err, pgx.ErrNoRows) {
		return ref, false, clinical.ErrNotFound
	}
	return ref, active, err
}

func descendantQuery(k string) string {
	switch k {
	case "departments":
		return `SELECT 'service-points',sp.id,sp.name FROM service_points sp WHERE sp.department_id=$1 AND sp.active
			UNION ALL SELECT 'wards',w.id,w.name FROM wards w WHERE w.department_id=$1 AND w.active
			UNION ALL SELECT 'rooms',r.id,r.name FROM rooms r JOIN wards w ON w.id=r.ward_id WHERE w.department_id=$1 AND r.active
			UNION ALL SELECT 'beds',b.id,b.name FROM beds b JOIN wards w ON w.id=b.ward_id WHERE w.department_id=$1 AND b.active
			UNION ALL SELECT 'queues',q.id,q.name FROM queues q JOIN service_points sp ON sp.id=q.service_point_id WHERE sp.department_id=$1 AND q.active`
	case "service-points":
		return `SELECT 'queues',q.id,q.name FROM queues q WHERE q.service_point_id=$1 AND q.active`
	case "wards":
		return `SELECT 'rooms',r.id,r.name FROM rooms r WHERE r.ward_id=$1 AND r.active
			UNION ALL SELECT 'beds',b.id,b.name FROM beds b WHERE b.ward_id=$1 AND b.active`
	case "rooms":
		return `SELECT 'beds',b.id,b.name FROM beds b WHERE b.room_id=$1 AND b.active`
	default:
		return ""
	}
}

func (r *Repository) DeactivationImpact(c context.Context, k, id string) (*clinical.DeactivationImpact, error) {
	exec := r.exec(c)
	root, _, err := resourceRef(c, exec, k, id)
	if err != nil {
		return nil, err
	}
	impact := &clinical.DeactivationImpact{Root: root, ActiveDescendants: []clinical.ResourceReference{}, DescendantCounts: map[string]int{}, OperationalBlockers: []clinical.OperationalBlocker{}}
	if q := descendantQuery(k); q != "" {
		rows, err := exec.Query(c, q, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ref clinical.ResourceReference
			if err = rows.Scan(&ref.Kind, &ref.ID, &ref.Name); err != nil {
				rows.Close()
				return nil, err
			}
			impact.ActiveDescendants = append(impact.ActiveDescendants, ref)
			impact.DescendantCounts[ref.Kind]++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	refs := append([]clinical.ResourceReference{root}, impact.ActiveDescendants...)
	for _, ref := range refs {
		switch ref.Kind {
		case "beds":
			var status string
			if err = exec.QueryRow(c, "SELECT status FROM beds WHERE id=$1", ref.ID).Scan(&status); err != nil {
				return nil, err
			}
			if status == "occupied" || status == "reserved" {
				impact.OperationalBlockers = append(impact.OperationalBlockers, clinical.OperationalBlocker{Type: "bed_" + status, Kind: ref.Kind, ResourceID: ref.ID, Name: ref.Name, Count: 1, Message: "Bed is " + status})
			}
		case "service-points":
			var count int
			err = exec.QueryRow(c, "SELECT count(*) FROM clinical_encounters WHERE service_point_id=$1 AND status IN ('planned','in_progress')", ref.ID).Scan(&count)
			if err != nil {
				return nil, err
			}
			if count > 0 {
				impact.OperationalBlockers = append(impact.OperationalBlockers, clinical.OperationalBlocker{Type: "active_encounters", Kind: ref.Kind, ResourceID: ref.ID, Name: ref.Name, Count: count, Message: "Service point has active encounters"})
			}
		case "queues":
			var entries, rules int
			err = exec.QueryRow(c, "SELECT count(*) FROM queue_entries WHERE queue_id=$1 AND status IN ('waiting','called','in_service','paused')", ref.ID).Scan(&entries)
			if err == nil {
				err = exec.QueryRow(c, "SELECT count(*) FROM queue_routing_rules WHERE target_queue_id=$1 AND active", ref.ID).Scan(&rules)
			}
			if err != nil {
				return nil, err
			}
			if entries > 0 {
				impact.OperationalBlockers = append(impact.OperationalBlockers, clinical.OperationalBlocker{Type: "active_queue_entries", Kind: ref.Kind, ResourceID: ref.ID, Name: ref.Name, Count: entries, Message: "Queue has active entries"})
			}
			if rules > 0 {
				impact.OperationalBlockers = append(impact.OperationalBlockers, clinical.OperationalBlocker{Type: "active_routing_rules", Kind: ref.Kind, ResourceID: ref.ID, Name: ref.Name, Count: rules, Message: "Queue is targeted by active routing rules"})
			}
		}
	}
	impact.CascadeAllowed = len(impact.OperationalBlockers) == 0
	return impact, nil
}

func (r *Repository) DeactivateResource(c context.Context, k, id string, cascade bool) (*clinical.ResourceLifecycleResult, error) {
	impact, err := r.DeactivationImpact(c, k, id)
	if err != nil {
		return nil, err
	}
	if len(impact.OperationalBlockers) > 0 {
		return nil, &clinical.LifecycleConflictError{Cause: clinical.ErrOperationalUse, Impact: impact}
	}
	if len(impact.ActiveDescendants) > 0 && !cascade {
		return nil, &clinical.LifecycleConflictError{Cause: clinical.ErrActiveDescendants, Impact: impact}
	}
	affected := []clinical.ResourceReference{}
	if cascade {
		for _, ref := range impact.ActiveDescendants {
			table, _, _ := resourceSpec(ref.Kind)
			if _, err = r.exec(c).Exec(c, "UPDATE "+table+" SET active=false,updated_at=now() WHERE id=$1 AND active", ref.ID); err != nil {
				return nil, err
			}
			affected = append(affected, ref)
		}
	}
	table, _, _ := resourceSpec(k)
	if _, err = r.exec(c).Exec(c, "UPDATE "+table+" SET active=false,updated_at=now() WHERE id=$1", id); err != nil {
		return nil, err
	}
	affected = append([]clinical.ResourceReference{impact.Root}, affected...)
	return &clinical.ResourceLifecycleResult{Root: impact.Root, Affected: affected}, nil
}

func parentActiveQuery(k string) string {
	switch k {
	case "service-points":
		return "SELECT d.active FROM service_points x JOIN departments d ON d.id=x.department_id WHERE x.id=$1"
	case "wards":
		return "SELECT d.active FROM wards x JOIN departments d ON d.id=x.department_id WHERE x.id=$1"
	case "rooms":
		return "SELECT w.active AND d.active FROM rooms x JOIN wards w ON w.id=x.ward_id JOIN departments d ON d.id=w.department_id WHERE x.id=$1"
	case "beds":
		return "SELECT r.active AND w.active AND d.active FROM beds x JOIN rooms r ON r.id=x.room_id JOIN wards w ON w.id=x.ward_id JOIN departments d ON d.id=w.department_id WHERE x.id=$1"
	case "queues":
		return "SELECT sp.active AND d.active FROM queues x JOIN service_points sp ON sp.id=x.service_point_id JOIN departments d ON d.id=sp.department_id WHERE x.id=$1"
	default:
		return ""
	}
}

func (r *Repository) ReactivateResource(c context.Context, k, id string) (*clinical.Resource, error) {
	table, cols, err := resourceSpec(k)
	if err != nil {
		return nil, err
	}
	if q := parentActiveQuery(k); q != "" {
		var active bool
		if err = r.exec(c).QueryRow(c, q, id).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
			return nil, clinical.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if !active {
			return nil, clinical.ErrInactiveParent
		}
	}
	x, err := scanResource(r.exec(c).QueryRow(c, "UPDATE "+table+" SET active=true,updated_at=now() WHERE id=$1 RETURNING "+cols, id))
	return x, normalizeResourceError(err)
}

func scanVisit(row pgx.Row) (*clinical.Visit, error) {
	var x clinical.Visit
	err := row.Scan(&x.ID, &x.PatientID, &x.VisitType, &x.Status, &x.Reason, &x.ServicePointID, &x.StartedAt, &x.EndedAt, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, err
}

const visitCols = "id,patient_id,visit_type::text,status::text,reason,service_point_id,started_at,ended_at,created_by,created_at,updated_at"

func (r *Repository) CreateVisit(c context.Context, x clinical.Visit) (*clinical.Visit, error) {
	return scanVisit(r.exec(c).QueryRow(c, "INSERT INTO clinical_visits(id,patient_id,visit_type,status,reason,service_point_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING "+visitCols, x.ID, x.PatientID, x.VisitType, x.Status, x.Reason, x.ServicePointID, x.CreatedBy))
}
func (r *Repository) GetVisit(c context.Context, id string) (*clinical.Visit, error) {
	return scanVisit(r.exec(c).QueryRow(c, "SELECT "+visitCols+" FROM clinical_visits WHERE id=$1", id))
}
func (r *Repository) ApplyVisitRouting(c context.Context, v clinical.Visit) error {
	outboxID, e := utility.GenerateUUID()
	if e != nil {
		return e
	}
	_, e = r.exec(c).Exec(c, "INSERT INTO clinical_outbox(id,event_type,aggregate_type,aggregate_id,payload,status,processed_at) VALUES($1,'visit.created','visit',$2,jsonb_build_object('patient_id',$3::uuid),'processed',now())", outboxID, v.ID, v.PatientID)
	if e != nil {
		return e
	}
	var queueID string
	var priority int16
	e = r.exec(c).QueryRow(c, `SELECT target_queue_id,priority FROM queue_routing_rules
		WHERE active AND event_type='visit.created' AND (visit_type IS NULL OR visit_type=$1::clinical_visit_type)
		ORDER BY (visit_type IS NOT NULL) DESC, priority DESC, id LIMIT 1`, v.VisitType).Scan(&queueID, &priority)
	if errors.Is(e, pgx.ErrNoRows) {
		return clinical.ErrRoutingMissing
	}
	if e != nil {
		return e
	}
	id, _ := utility.GenerateUUID()
	hist, _ := utility.GenerateUUID()
	tag, e := r.exec(c).Exec(c, "INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,priority) VALUES($1,$2,'visit',$3,$4,'waiting',$5) ON CONFLICT DO NOTHING", id, queueID, v.ID, v.PatientID, priority)
	if e != nil {
		return e
	}
	if tag.RowsAffected() > 0 {
		_, e = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,to_status,to_queue_id,automated,reason) VALUES($1,$2,'waiting',$3,true,'Automatic visit routing')", hist, id, queueID)
		if e != nil {
			return e
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
	rows, e := r.exec(c).Query(c, "SELECT id,name,event_type,visit_type::text,encounter_type::text,order_kind,service_category,target_queue_id,priority,active FROM queue_routing_rules ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.RoutingRule{}
	for rows.Next() {
		var x clinical.RoutingRule
		if e = rows.Scan(&x.ID, &x.Name, &x.EventType, &x.VisitType, &x.EncounterType, &x.OrderKind, &x.ServiceCategory, &x.TargetQueueID, &x.Priority, &x.Active); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) CreateRoutingRule(c context.Context, x clinical.RoutingRule) (*clinical.RoutingRule, error) {
	e := r.exec(c).QueryRow(c, "INSERT INTO queue_routing_rules(id,name,event_type,visit_type,encounter_type,order_kind,service_category,target_queue_id,priority) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,name,event_type,visit_type::text,encounter_type::text,order_kind,service_category,target_queue_id,priority,active", x.ID, x.Name, x.EventType, x.VisitType, x.EncounterType, x.OrderKind, x.ServiceCategory, x.TargetQueueID, x.Priority).Scan(&x.ID, &x.Name, &x.EventType, &x.VisitType, &x.EncounterType, &x.OrderKind, &x.ServiceCategory, &x.TargetQueueID, &x.Priority, &x.Active)
	return &x, normalizeResourceError(e)
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
