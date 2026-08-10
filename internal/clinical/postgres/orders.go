package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"nodus-health/internal/clinical"
	"nodus-health/pkg/utility"
)

const orderCols = `o.id,o.patient_id,o.visit_id,o.encounter_id,o.kind,o.category,o.priority,o.status,o.review_required,o.ordered_by,o.ordered_at,o.completed_at,o.cancelled_at,o.transition_reason,o.version,o.updated_at`

func scanOrder(row pgx.Row) (*clinical.ClinicalOrder, error) {
	var x clinical.ClinicalOrder
	err := row.Scan(&x.ID, &x.PatientID, &x.VisitID, &x.EncounterID, &x.Kind, &x.Category, &x.Priority, &x.Status, &x.ReviewRequired, &x.OrderedBy, &x.OrderedAt, &x.CompletedAt, &x.CancelledAt, &x.TransitionReason, &x.Version, &x.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, err
}

func (r *Repository) hydrateOrder(c context.Context, x *clinical.ClinicalOrder) error {
	if x.Kind == "service" {
		var d clinical.ServiceOrderDetail
		err := r.exec(c).QueryRow(c, `SELECT service_id,service_code,service_name FROM clinical_service_order_details WHERE order_id=$1`, x.ID).Scan(&d.ServiceID, &d.ServiceCode, &d.ServiceName)
		x.Service = &d
		return err
	}
	var d clinical.MedicationOrderDetail
	err := r.exec(c).QueryRow(c, `SELECT medication_id,medication_code,medication_name,dose,dose_unit,route,frequency,duration_days,quantity,instructions,allergy_override_reason,allergy_acknowledged_at FROM clinical_medication_order_details WHERE order_id=$1`, x.ID).Scan(&d.MedicationID, &d.MedicationCode, &d.MedicationName, &d.Dose, &d.DoseUnit, &d.Route, &d.Frequency, &d.DurationDays, &d.Quantity, &d.Instructions, &d.AllergyOverrideReason, &d.AllergyAcknowledgedAt)
	x.Medication = &d
	return err
}

func (r *Repository) CreateOrder(c context.Context, x clinical.ClinicalOrder) (*clinical.ClinicalOrder, error) {
	var err error
	if x.Kind == "service" {
		var d clinical.ServiceOrderDetail
		err = r.exec(c).QueryRow(c, `SELECT id,code,name,category FROM clinical_services WHERE id=$1 AND active`, x.Service.ServiceID).Scan(&d.ServiceID, &d.ServiceCode, &d.ServiceName, &x.Category)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, clinical.ErrInvalidInput
		}
		if err != nil {
			return nil, err
		}
		x.Service = &d
	} else {
		var d = *x.Medication
		err = r.exec(c).QueryRow(c, `SELECT id,code,COALESCE(brand_name,generic_name) FROM medication_catalogue WHERE id=$1 AND active`, d.MedicationID).Scan(&d.MedicationID, &d.MedicationCode, &d.MedicationName)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, clinical.ErrInvalidInput
		}
		if err != nil {
			return nil, err
		}
		x.Category = "pharmacy"
		x.Medication = &d
	}
	created, err := scanOrder(r.exec(c).QueryRow(c, `INSERT INTO clinical_orders(id,patient_id,visit_id,encounter_id,kind,category,priority,status,review_required,ordered_by) VALUES($1,$2,$3,$4,$5,$6,$7,'ordered',$8,$9) RETURNING `+strings.ReplaceAll(orderCols, "o.", ""), x.ID, x.PatientID, x.VisitID, x.EncounterID, x.Kind, x.Category, x.Priority, x.ReviewRequired, x.OrderedBy))
	if err != nil {
		return nil, err
	}
	if x.Kind == "service" {
		_, err = r.exec(c).Exec(c, `INSERT INTO clinical_service_order_details(order_id,service_id,service_code,service_name) VALUES($1,$2,$3,$4)`, x.ID, x.Service.ServiceID, x.Service.ServiceCode, x.Service.ServiceName)
	} else {
		d := x.Medication
		_, err = r.exec(c).Exec(c, `INSERT INTO clinical_medication_order_details(order_id,medication_id,medication_code,medication_name,dose,dose_unit,route,frequency,duration_days,quantity,instructions,allergy_override_reason,allergy_acknowledged_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,CASE WHEN $12 IS NULL THEN NULL ELSE now() END)`, x.ID, d.MedicationID, d.MedicationCode, d.MedicationName, d.Dose, d.DoseUnit, d.Route, d.Frequency, d.DurationDays, d.Quantity, d.Instructions, d.AllergyOverrideReason)
	}
	if err != nil {
		return nil, err
	}
	created.Service = x.Service
	created.Medication = x.Medication
	if x.ReviewRequired {
		_, err = r.exec(c).Exec(c, `UPDATE queue_entries SET status='paused' WHERE subject_type='visit' AND subject_id=$1 AND status IN ('waiting','called','in_service')`, x.VisitID)
	}
	return created, err
}

func (r *Repository) GetOrder(c context.Context, id string) (*clinical.ClinicalOrder, error) {
	x, e := scanOrder(r.exec(c).QueryRow(c, `SELECT `+orderCols+` FROM clinical_orders o WHERE o.id=$1`, id))
	if e != nil {
		return nil, e
	}
	e = r.hydrateOrder(c, x)
	return x, e
}

func (r *Repository) ListOrders(c context.Context, f clinical.OrderFilters) ([]clinical.ClinicalOrder, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.VisitID != "" {
		add("o.visit_id=$%d", f.VisitID)
	}
	if f.Category != "" {
		add("o.category=$%d", f.Category)
	}
	if f.Status != "" {
		add("o.status=$%d", f.Status)
	}
	if f.Kind != "" {
		add("o.kind=$%d", f.Kind)
	}
	var total int
	if e := r.exec(c).QueryRow(c, `SELECT count(*) FROM clinical_orders o WHERE `+strings.Join(where, " AND "), args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	page, per := f.Page, f.PerPage
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 25
	}
	args = append(args, per, (page-1)*per)
	rows, e := r.exec(c).Query(c, `SELECT `+orderCols+` FROM clinical_orders o WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(" ORDER BY o.priority DESC,o.ordered_at LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []clinical.ClinicalOrder{}
	for rows.Next() {
		x, e := scanOrder(rows)
		if e != nil {
			return nil, 0, e
		}
		if e = r.hydrateOrder(c, x); e != nil {
			return nil, 0, e
		}
		out = append(out, *x)
	}
	return out, total, rows.Err()
}

func (r *Repository) TransitionOrder(c context.Context, x clinical.ClinicalOrder, status, reason string) (*clinical.ClinicalOrder, error) {
	tag, e := r.exec(c).Exec(c, `UPDATE clinical_orders SET status=$2,transition_reason=NULLIF($3,''),version=version+1,completed_at=CASE WHEN $2='completed' THEN now() ELSE completed_at END,cancelled_at=CASE WHEN $2 IN ('cancelled','rejected') THEN now() ELSE cancelled_at END WHERE id=$1 AND version=$4`, x.ID, status, reason, x.Version)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	return r.GetOrder(c, x.ID)
}

func (r *Repository) HasActiveMedicationAllergy(c context.Context, patient string) (bool, error) {
	var v bool
	e := r.exec(c).QueryRow(c, `SELECT EXISTS(SELECT 1 FROM clinical_allergies a JOIN allergen_catalogue c ON c.id=a.allergen_id WHERE a.patient_id=$1 AND a.status='active' AND c.category='medication')`, patient).Scan(&v)
	return v, e
}
func (r *Repository) HasUnresolvedReviewOrders(c context.Context, visit string) (bool, error) {
	var v bool
	e := r.exec(c).QueryRow(c, `SELECT EXISTS(SELECT 1 FROM clinical_orders WHERE visit_id=$1 AND review_required AND status NOT IN ('completed','rejected','cancelled'))`, visit).Scan(&v)
	return v, e
}

func (r *Repository) GetCurrentVisitQueueEntry(c context.Context, visit string) (*clinical.QueueEntry, error) {
	return scanEntry(r.exec(c).QueryRow(c, entrySelect+` WHERE e.subject_type='visit' AND e.subject_id=$1 AND e.status IN ('waiting','called','in_service','paused') ORDER BY e.updated_at DESC LIMIT 1`, visit))
}

func (r *Repository) ApplyEventRouting(c context.Context, event, subjectType, subjectID, patientID, visitType, encounterType string, actor *string) error {
	var kind, category string
	if subjectType == "order" {
		_ = r.exec(c).QueryRow(c, `SELECT kind,category FROM clinical_orders WHERE id=$1`, subjectID).Scan(&kind, &category)
	}
	var queueID string
	var priority int16
	e := r.exec(c).QueryRow(c, `SELECT target_queue_id,priority FROM queue_routing_rules
		WHERE active AND event_type=$1
		  AND (visit_type IS NULL OR visit_type::text=$2)
		  AND (encounter_type IS NULL OR encounter_type::text=$3)
		  AND (order_kind IS NULL OR order_kind=$4)
		  AND (service_category IS NULL OR service_category=$5)
		ORDER BY ((visit_type IS NOT NULL)::int + (encounter_type IS NOT NULL)::int +
		          (order_kind IS NOT NULL)::int + (service_category IS NOT NULL)::int) DESC,
		         priority DESC, id
		LIMIT 1`, event, visitType, encounterType, kind, category).Scan(&queueID, &priority)
	if errors.Is(e, pgx.ErrNoRows) {
		return clinical.ErrRoutingMissing
	}
	if e != nil {
		return e
	}
	if event == "encounter.completed" || event == "order.review_ready" {
		_, e = r.exec(c).Exec(c, `UPDATE queue_entries SET status='completed',completed_at=now() WHERE subject_type='visit' AND subject_id=$1 AND status IN ('waiting','called','in_service','paused')`, subjectID)
		if e != nil {
			return e
		}
	}
	id, _ := utility.GenerateUUID()
	h, _ := utility.GenerateUUID()
	tag, e := r.exec(c).Exec(c, `INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,priority) VALUES($1,$2,$3,$4,$5,'waiting',$6) ON CONFLICT DO NOTHING`, id, queueID, subjectType, subjectID, patientID, priority)
	if e != nil {
		return e
	}
	if tag.RowsAffected() > 0 {
		_, e = r.exec(c).Exec(c, `INSERT INTO queue_entry_history(id,queue_entry_id,to_status,to_queue_id,actor_id,automated,reason) VALUES($1,$2,'waiting',$3,$4,true,$5)`, h, id, queueID, actor, "Automatic "+event+" routing")
		if e != nil {
			return e
		}
	}
	return nil
}
