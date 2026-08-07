package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"nodus-health/internal/clinical"
	"nodus-health/pkg/utility"
)

func (r *Repository) FindActiveOutpatientVisit(c context.Context, patient string) (*clinical.Visit, error) {
	var locked string
	if e := r.exec(c).QueryRow(c, "SELECT id FROM patients WHERE id=$1 FOR UPDATE", patient).Scan(&locked); errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	} else if e != nil {
		return nil, e
	}
	x, e := scanVisit(r.exec(c).QueryRow(c, "SELECT "+visitCols+" FROM clinical_visits WHERE patient_id=$1 AND visit_type='outpatient' AND status='active' ORDER BY started_at DESC LIMIT 1", patient))
	if errors.Is(e, clinical.ErrNotFound) {
		return nil, nil
	}
	return x, e
}
func (r *Repository) ListVisits(c context.Context, kind, status string) ([]clinical.Visit, error) {
	q := "SELECT " + visitCols + " FROM clinical_visits WHERE visit_type=$1::clinical_visit_type"
	args := []any{kind}
	if status != "" {
		q += " AND status=$2::clinical_visit_status"
		args = append(args, status)
	}
	q += " AND started_at >= date_trunc('day',now()) ORDER BY started_at DESC"
	rows, e := r.exec(c).Query(c, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Visit{}
	for rows.Next() {
		x, e := scanVisit(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

const encounterCols = "id,visit_id,encounter_type::text,status::text,service_point_id,clinician_id,started_at,ended_at,created_at"

func scanEncounter(row pgx.Row) (*clinical.Encounter, error) {
	var x clinical.Encounter
	e := row.Scan(&x.ID, &x.VisitID, &x.EncounterType, &x.Status, &x.ServicePointID, &x.ClinicianID, &x.StartedAt, &x.EndedAt, &x.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, e
}
func (r *Repository) CreateEncounter(c context.Context, x clinical.Encounter) (*clinical.Encounter, error) {
	return scanEncounter(r.exec(c).QueryRow(c, "INSERT INTO clinical_encounters(id,visit_id,service_point_id,encounter_type,status,clinician_id,started_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING "+encounterCols, x.ID, x.VisitID, x.ServicePointID, x.EncounterType, x.Status, x.ClinicianID, x.StartedAt))
}
func (r *Repository) CreateEncounterWithForm(c context.Context, x clinical.Encounter, formID, actor string) (*clinical.Encounter, error) {
	row := r.exec(c).QueryRow(c, `WITH selected AS (
		SELECT v.id AS version_id FROM clinical_templates t JOIN clinical_template_versions v ON v.template_id=t.id AND v.status='published'
		WHERE t.encounter_type=$4 AND t.is_default AND t.archived_at IS NULL
	), inserted_encounter AS (
		INSERT INTO clinical_encounters(id,visit_id,service_point_id,encounter_type,status,clinician_id,started_at)
		SELECT $1,$2,$3,$4,$5,$6,$7 FROM selected RETURNING id,visit_id,encounter_type,status,service_point_id,clinician_id,started_at,ended_at,created_at
	), inserted_form AS (
		INSERT INTO clinical_encounter_forms(id,encounter_id,template_version_id,saved_by)
		SELECT $8,e.id,s.version_id,$9 FROM inserted_encounter e CROSS JOIN selected s RETURNING encounter_id
	) SELECT e.id,e.visit_id,e.encounter_type::text,e.status::text,e.service_point_id,e.clinician_id,e.started_at,e.ended_at,e.created_at
	FROM inserted_encounter e JOIN inserted_form f ON f.encounter_id=e.id`, x.ID, x.VisitID, x.ServicePointID, x.EncounterType, x.Status, x.ClinicianID, x.StartedAt, formID, actor)
	created, err := scanEncounter(row)
	if errors.Is(err, clinical.ErrNotFound) {
		return nil, clinical.ErrConflict
	}
	return created, err
}
func (r *Repository) GetEncounter(c context.Context, id string) (*clinical.Encounter, error) {
	return scanEncounter(r.exec(c).QueryRow(c, "SELECT "+encounterCols+" FROM clinical_encounters WHERE id=$1", id))
}
func (r *Repository) ListEncounters(c context.Context, visit string) ([]clinical.Encounter, error) {
	rows, e := r.exec(c).Query(c, "SELECT "+encounterCols+" FROM clinical_encounters WHERE visit_id=$1 ORDER BY created_at", visit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Encounter{}
	for rows.Next() {
		x, e := scanEncounter(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}
func (r *Repository) CompleteEncounter(c context.Context, id, actor string, targetQueue *string) (*clinical.Encounter, error) {
	e, err := r.GetEncounter(c, id)
	if err != nil {
		return nil, err
	}
	tag, err := r.exec(c).Exec(c, "UPDATE clinical_encounters SET status='completed',ended_at=now() WHERE id=$1 AND status='in_progress'", id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	if e.EncounterType == "triage" && targetQueue != nil {
		var patient string
		if err = r.exec(c).QueryRow(c, "SELECT patient_id FROM clinical_visits WHERE id=$1", e.VisitID).Scan(&patient); err != nil {
			return nil, err
		}
		rows, err := r.exec(c).Query(c, "SELECT id,queue_id,status::text FROM queue_entries WHERE subject_type='visit' AND subject_id=$1 AND status IN ('waiting','called','in_service','paused') FOR UPDATE", e.VisitID)
		if err != nil {
			return nil, err
		}
		type old struct{ id, queue, status string }
		var olds []old
		for rows.Next() {
			var x old
			if err = rows.Scan(&x.id, &x.queue, &x.status); err != nil {
				rows.Close()
				return nil, err
			}
			olds = append(olds, x)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		for _, x := range olds {
			_, err = r.exec(c).Exec(c, "UPDATE queue_entries SET status='completed',completed_at=now() WHERE id=$1", x.id)
			if err != nil {
				return nil, err
			}
			h, _ := utility.GenerateUUID()
			_, err = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,from_status,to_status,from_queue_id,to_queue_id,actor_id,reason) VALUES($1,$2,$3,'completed',$4,$4,$5,'Triage completed')", h, x.id, x.status, x.queue, actor)
			if err != nil {
				return nil, err
			}
		}
		qid, _ := utility.GenerateUUID()
		h, _ := utility.GenerateUUID()
		tag, err = r.exec(c).Exec(c, "INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,priority) VALUES($1,$2,'visit',$3,$4,'waiting',0) ON CONFLICT DO NOTHING", qid, *targetQueue, e.VisitID, patient)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 1 {
			_, err = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,to_status,to_queue_id,actor_id,reason,automated) VALUES($1,$2,'waiting',$3,$4,'Triage completed',true)", h, qid, *targetQueue, actor)
			if err != nil {
				return nil, err
			}
		}
	}
	return r.GetEncounter(c, id)
}

const observationCols = "id,patient_id,visit_id,encounter_id,code,value_numeric,value_text,unit,observed_at,recorded_by,created_at,source_form_id,source_form_field_key"

func scanObservation(row pgx.Row) (clinical.Observation, error) {
	var x clinical.Observation
	e := row.Scan(&x.ID, &x.PatientID, &x.VisitID, &x.EncounterID, &x.Code, &x.ValueNumeric, &x.ValueText, &x.Unit, &x.ObservedAt, &x.RecordedBy, &x.CreatedAt, &x.SourceFormID, &x.SourceFormFieldKey)
	return x, e
}
func (r *Repository) CreateObservations(c context.Context, in []clinical.Observation) ([]clinical.Observation, error) {
	for _, x := range in {
		_, e := r.exec(c).Exec(c, "INSERT INTO clinical_observations(id,patient_id,visit_id,encounter_id,code,value_numeric,value_text,unit,observed_at,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", x.ID, x.PatientID, x.VisitID, x.EncounterID, x.Code, x.ValueNumeric, x.ValueText, x.Unit, x.ObservedAt, x.RecordedBy)
		if e != nil {
			return nil, e
		}
	}
	return in, nil
}
func (r *Repository) ListObservations(c context.Context, visit string) ([]clinical.Observation, error) {
	rows, e := r.exec(c).Query(c, "SELECT "+observationCols+" FROM clinical_observations WHERE visit_id=$1 ORDER BY observed_at", visit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Observation{}
	for rows.Next() {
		x, e := scanObservation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) CreateNote(c context.Context, x clinical.ClinicalNote) (*clinical.ClinicalNote, error) {
	e := r.exec(c).QueryRow(c, "INSERT INTO clinical_notes(id,patient_id,visit_id,encounter_id,note_type,body,authored_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING authored_at", x.ID, x.PatientID, x.VisitID, x.EncounterID, x.NoteType, x.Body, x.AuthoredBy).Scan(&x.AuthoredAt)
	return &x, e
}
func (r *Repository) ListNotes(c context.Context, visit string) ([]clinical.ClinicalNote, error) {
	rows, e := r.exec(c).Query(c, "SELECT id,patient_id,visit_id,encounter_id,note_type,body,authored_by,authored_at,amended_from_id FROM clinical_notes WHERE visit_id=$1 ORDER BY authored_at", visit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.ClinicalNote{}
	for rows.Next() {
		var x clinical.ClinicalNote
		if e = rows.Scan(&x.ID, &x.PatientID, &x.VisitID, &x.EncounterID, &x.NoteType, &x.Body, &x.AuthoredBy, &x.AuthoredAt, &x.AmendedFromID); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) CreateDiagnosis(c context.Context, x clinical.Diagnosis) (*clinical.Diagnosis, error) {
	e := r.exec(c).QueryRow(c, "INSERT INTO clinical_diagnoses(id,patient_id,visit_id,encounter_id,concept_id,kind,note,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING concept_id,recorded_at", x.ID, x.PatientID, x.VisitID, x.EncounterID, x.ConceptID, x.Kind, x.Note, x.RecordedBy).Scan(&x.ConceptID, &x.RecordedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrInvalidInput
	}
	if e == nil {
		_ = r.exec(c).QueryRow(c, "SELECT display FROM terminology_concepts WHERE id=$1", x.ConceptID).Scan(&x.Display)
	}
	return &x, e
}
func (r *Repository) ListDiagnoses(c context.Context, visit string) ([]clinical.Diagnosis, error) {
	rows, e := r.exec(c).Query(c, "SELECT d.id,d.patient_id,d.visit_id,d.encounter_id,d.concept_id,c.code,c.display,d.kind::text,d.note,d.recorded_by,d.recorded_at FROM clinical_diagnoses d JOIN terminology_concepts c ON c.id=d.concept_id WHERE d.visit_id=$1 ORDER BY d.recorded_at", visit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Diagnosis{}
	for rows.Next() {
		var x clinical.Diagnosis
		if e = rows.Scan(&x.ID, &x.PatientID, &x.VisitID, &x.EncounterID, &x.ConceptID, &x.Code, &x.Display, &x.Kind, &x.Note, &x.RecordedBy, &x.RecordedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) CreateAllergy(c context.Context, x clinical.Allergy) (*clinical.Allergy, error) {
	e := r.exec(c).QueryRow(c, "INSERT INTO clinical_allergies(id,patient_id,allergen,allergen_id,reaction,severity,status,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING recorded_at", x.ID, x.PatientID, x.Allergen, x.AllergenID, x.Reaction, x.Severity, x.Status, x.RecordedBy).Scan(&x.RecordedAt)
	return &x, normalizeResourceError(e)
}
func (r *Repository) ListAllergies(c context.Context, patient string) ([]clinical.Allergy, error) {
	rows, e := r.exec(c).Query(c, "SELECT a.id,a.patient_id,a.allergen,a.reaction,a.severity,a.status,a.recorded_by,a.recorded_at,a.allergen_id,c.code,c.category,(a.allergen_id IS NULL) FROM clinical_allergies a LEFT JOIN allergen_catalogue c ON c.id=a.allergen_id WHERE a.patient_id=$1 ORDER BY a.recorded_at DESC", patient)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Allergy{}
	for rows.Next() {
		var x clinical.Allergy
		if e = rows.Scan(&x.ID, &x.PatientID, &x.Allergen, &x.Reaction, &x.Severity, &x.Status, &x.RecordedBy, &x.RecordedAt, &x.AllergenID, &x.AllergenCode, &x.Category, &x.IsCustom); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) CompleteVisit(c context.Context, id string) (*clinical.Visit, error) {
	tag, e := r.exec(c).Exec(c, "UPDATE clinical_visits SET status='completed',ended_at=now() WHERE id=$1 AND status='active'", id)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	rows, e := r.exec(c).Query(c, "SELECT id,queue_id,status::text FROM queue_entries WHERE subject_type='visit' AND subject_id=$1 AND status IN ('waiting','called','in_service','paused') FOR UPDATE", id)
	if e != nil {
		return nil, e
	}
	type activeEntry struct{ id, queue, status string }
	var entries []activeEntry
	for rows.Next() {
		var x activeEntry
		if e = rows.Scan(&x.id, &x.queue, &x.status); e != nil {
			rows.Close()
			return nil, e
		}
		entries = append(entries, x)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, e
	}
	for _, x := range entries {
		_, e = r.exec(c).Exec(c, "UPDATE queue_entries SET status='completed',completed_at=now() WHERE id=$1", x.id)
		if e != nil {
			return nil, e
		}
		h, _ := utility.GenerateUUID()
		_, e = r.exec(c).Exec(c, "INSERT INTO queue_entry_history(id,queue_entry_id,from_status,to_status,from_queue_id,to_queue_id,reason,automated) VALUES($1,$2,$3,'completed',$4,$4,'Visit completed',true)", h, x.id, x.status, x.queue)
		if e != nil {
			return nil, e
		}
	}
	outbox, _ := utility.GenerateUUID()
	_, e = r.exec(c).Exec(c, "INSERT INTO clinical_outbox(id,event_type,aggregate_type,aggregate_id,payload,status,processed_at) VALUES($1,'visit.completed','visit',$2,'{}','processed',now())", outbox, id)
	if e != nil {
		return nil, e
	}
	return r.GetVisit(c, id)
}
