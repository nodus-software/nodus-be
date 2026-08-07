package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"nodus-health/internal/clinical"
)

const templateCols = "id,code,name,description,encounter_type::text,is_default,archived_at,created_at,updated_at"

func scanTemplate(row pgx.Row) (*clinical.ClinicalTemplate, error) {
	var x clinical.ClinicalTemplate
	err := row.Scan(&x.ID, &x.Code, &x.Name, &x.Description, &x.EncounterType, &x.IsDefault, &x.ArchivedAt, &x.CreatedAt, &x.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, err
}

func scanVersion(row pgx.Row) (*clinical.ClinicalTemplateVersion, error) {
	var x clinical.ClinicalTemplateVersion
	var raw []byte
	err := row.Scan(&x.ID, &x.TemplateID, &x.Version, &x.Status, &raw, &x.CreatedAt, &x.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &x.Definition)
	}
	return &x, err
}

func (r *Repository) hydrateTemplate(c context.Context, x *clinical.ClinicalTemplate) error {
	rows, err := r.exec(c).Query(c, "SELECT id,template_id,version,status,definition,created_at,published_at FROM clinical_template_versions WHERE template_id=$1 AND status IN ('draft','published') ORDER BY version", x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanVersion(rows)
		if e != nil {
			return e
		}
		if v.Status == "draft" {
			x.Draft = v
		} else {
			x.Published = v
		}
	}
	return rows.Err()
}

func (r *Repository) GetTemplate(c context.Context, id string) (*clinical.ClinicalTemplate, error) {
	x, e := scanTemplate(r.exec(c).QueryRow(c, "SELECT "+templateCols+" FROM clinical_templates WHERE id=$1", id))
	if e != nil {
		return nil, e
	}
	e = r.hydrateTemplate(c, x)
	return x, e
}

func (r *Repository) ListTemplates(c context.Context, kind, status string) ([]clinical.ClinicalTemplate, error) {
	q := "SELECT " + templateCols + " FROM clinical_templates WHERE ($1='' OR encounter_type::text=$1) AND ($2='' OR ($2='archived' AND archived_at IS NOT NULL) OR ($2='active' AND archived_at IS NULL)) ORDER BY encounter_type,name"
	rows, e := r.exec(c).Query(c, q, kind, status)
	if e != nil {
		return nil, e
	}
	out := []clinical.ClinicalTemplate{}
	for rows.Next() {
		x, e := scanTemplate(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		if e = r.hydrateTemplate(c, &out[i]); e != nil {
			return nil, e
		}
	}
	return out, nil
}

func (r *Repository) CreateTemplate(c context.Context, x clinical.ClinicalTemplate, v clinical.ClinicalTemplateVersion) (*clinical.ClinicalTemplate, error) {
	exec := r.exec(c)
	if _, e := exec.Exec(c, "SAVEPOINT clinical_template_create"); e != nil {
		return nil, e
	}
	rollback := func(err error) (*clinical.ClinicalTemplate, error) {
		_, _ = exec.Exec(c, "ROLLBACK TO SAVEPOINT clinical_template_create")
		_, _ = exec.Exec(c, "RELEASE SAVEPOINT clinical_template_create")
		return nil, err
	}
	_, e := exec.Exec(c, "INSERT INTO clinical_templates(id,code,name,description,encounter_type,created_by) VALUES($1,$2,$3,$4,$5,$6)", x.ID, x.Code, x.Name, x.Description, x.EncounterType, v.CreatedBy)
	if e != nil {
		return rollback(normalizeResourceError(e))
	}
	raw, _ := json.Marshal(v.Definition)
	_, e = exec.Exec(c, "INSERT INTO clinical_template_versions(id,template_id,version,status,definition,created_by) VALUES($1,$2,1,'draft',$3,$4)", v.ID, x.ID, raw, v.CreatedBy)
	if e != nil {
		return rollback(normalizeResourceError(e))
	}
	if _, e = exec.Exec(c, "RELEASE SAVEPOINT clinical_template_create"); e != nil {
		return nil, e
	}
	return r.GetTemplate(c, x.ID)
}

func (r *Repository) CreateTemplateDraft(c context.Context, id, versionID, actor string) (*clinical.ClinicalTemplate, error) {
	tag, e := r.exec(c).Exec(c, `INSERT INTO clinical_template_versions(id,template_id,version,status,definition,created_by)
		SELECT $2,id,COALESCE((SELECT max(version)+1 FROM clinical_template_versions WHERE template_id=$1),1),'draft',
		COALESCE((SELECT definition FROM clinical_template_versions WHERE template_id=$1 AND status='published'),'{}'::jsonb),$3
		FROM clinical_templates WHERE id=$1 AND archived_at IS NULL ON CONFLICT DO NOTHING`, id, versionID, actor)
	if e != nil {
		return nil, normalizeResourceError(e)
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	return r.GetTemplate(c, id)
}

func (r *Repository) UpdateTemplateDraft(c context.Context, id, actor string, q clinical.UpdateTemplateDraftRequest) (*clinical.ClinicalTemplate, error) {
	raw, _ := json.Marshal(q.Definition)
	tag, e := r.exec(c).Exec(c, `WITH updated_template AS (UPDATE clinical_templates SET name=COALESCE($2,name),description=COALESCE($3,description) WHERE id=$1 AND archived_at IS NULL RETURNING id)
		UPDATE clinical_template_versions SET definition=$4,created_by=$5 WHERE template_id IN (SELECT id FROM updated_template) AND status='draft'`, id, q.Name, q.Description, raw, actor)
	if e != nil {
		return nil, normalizeResourceError(e)
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	return r.GetTemplate(c, id)
}

func (r *Repository) PublishTemplate(c context.Context, id, actor string) (*clinical.ClinicalTemplate, error) {
	exec := r.exec(c)
	if _, e := exec.Exec(c, "SAVEPOINT clinical_template_publish"); e != nil {
		return nil, e
	}
	rollback := func(err error) (*clinical.ClinicalTemplate, error) {
		_, _ = exec.Exec(c, "ROLLBACK TO SAVEPOINT clinical_template_publish")
		_, _ = exec.Exec(c, "RELEASE SAVEPOINT clinical_template_publish")
		return nil, err
	}
	var draftID string
	e := exec.QueryRow(c, "SELECT id FROM clinical_template_versions WHERE template_id=$1 AND status='draft' FOR UPDATE", id).Scan(&draftID)
	if errors.Is(e, pgx.ErrNoRows) {
		return rollback(clinical.ErrConflict)
	}
	if e != nil {
		return rollback(e)
	}
	if _, e = exec.Exec(c, "UPDATE clinical_template_versions SET status='superseded' WHERE template_id=$1 AND status='published'", id); e != nil {
		return rollback(e)
	}
	tag, e := exec.Exec(c, "UPDATE clinical_template_versions SET status='published',published_by=$2,published_at=now() WHERE id=$1 AND status='draft'", draftID, actor)
	if e != nil {
		return rollback(e)
	}
	if tag.RowsAffected() != 1 {
		return rollback(clinical.ErrConflict)
	}
	if _, e = exec.Exec(c, "RELEASE SAVEPOINT clinical_template_publish"); e != nil {
		return nil, e
	}
	return r.GetTemplate(c, id)
}

func (r *Repository) SetDefaultTemplate(c context.Context, id string) (*clinical.ClinicalTemplate, error) {
	var kind string
	e := r.exec(c).QueryRow(c, `SELECT encounter_type::text FROM clinical_templates t WHERE id=$1 AND archived_at IS NULL AND EXISTS(SELECT 1 FROM clinical_template_versions v WHERE v.template_id=t.id AND v.status='published')`, id).Scan(&kind)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrConflict
	}
	if e != nil {
		return nil, e
	}
	if _, e = r.exec(c).Exec(c, "UPDATE clinical_templates SET is_default=false WHERE encounter_type::text=$1 AND is_default", kind); e != nil {
		return nil, e
	}
	if _, e = r.exec(c).Exec(c, "UPDATE clinical_templates SET is_default=true WHERE id=$1", id); e != nil {
		return nil, e
	}
	return r.GetTemplate(c, id)
}

func (r *Repository) ArchiveTemplate(c context.Context, id string) (*clinical.ClinicalTemplate, error) {
	tag, e := r.exec(c).Exec(c, "UPDATE clinical_templates SET archived_at=now() WHERE id=$1 AND archived_at IS NULL AND NOT is_default", id)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	return r.GetTemplate(c, id)
}

func scanForm(row pgx.Row) (*clinical.EncounterForm, error) {
	var x clinical.EncounterForm
	var answers []byte
	err := row.Scan(&x.ID, &x.EncounterID, &x.TemplateVersion.ID, &x.Status, &answers, &x.Revision, &x.SavedBy, &x.SubmittedBy, &x.CreatedAt, &x.UpdatedAt, &x.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(answers, &x.Answers)
	}
	return &x, err
}

func (r *Repository) hydrateForm(c context.Context, x *clinical.EncounterForm) error {
	v, e := scanVersion(r.exec(c).QueryRow(c, "SELECT id,template_id,version,status,definition,created_at,published_at FROM clinical_template_versions WHERE id=$1", x.TemplateVersion.ID))
	if e != nil {
		return e
	}
	x.TemplateVersion = *v
	t, e := scanTemplate(r.exec(c).QueryRow(c, "SELECT "+templateCols+" FROM clinical_templates WHERE id=$1", v.TemplateID))
	if e != nil {
		return e
	}
	x.Template = *t
	return nil
}

func (r *Repository) CreateEncounterForm(c context.Context, encounterID, formID, actor string) (*clinical.EncounterForm, error) {
	tag, e := r.exec(c).Exec(c, `INSERT INTO clinical_encounter_forms(id,encounter_id,template_version_id,saved_by)
		SELECT $2,$1,v.id,$3 FROM clinical_encounters e JOIN clinical_templates t ON t.encounter_type=e.encounter_type AND t.is_default AND t.archived_at IS NULL JOIN clinical_template_versions v ON v.template_id=t.id AND v.status='published' WHERE e.id=$1`, encounterID, formID, actor)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	return r.GetEncounterForm(c, encounterID)
}

func (r *Repository) GetEncounterForm(c context.Context, encounterID string) (*clinical.EncounterForm, error) {
	x, e := scanForm(r.exec(c).QueryRow(c, "SELECT id,encounter_id,template_version_id,status,answers,revision,saved_by,submitted_by,created_at,updated_at,submitted_at FROM clinical_encounter_forms WHERE encounter_id=$1", encounterID))
	if e != nil {
		return nil, e
	}
	e = r.hydrateForm(c, x)
	return x, e
}

func (r *Repository) ListEncounterForms(c context.Context, visitID string) ([]clinical.EncounterForm, error) {
	rows, e := r.exec(c).Query(c, `SELECT f.id,f.encounter_id,f.template_version_id,f.status,f.answers,f.revision,f.saved_by,f.submitted_by,f.created_at,f.updated_at,f.submitted_at
		FROM clinical_encounter_forms f JOIN clinical_encounters e ON e.id=f.encounter_id WHERE e.visit_id=$1 ORDER BY f.created_at`, visitID)
	if e != nil {
		return nil, e
	}
	out := []clinical.EncounterForm{}
	for rows.Next() {
		x, err := scanForm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		if e = r.hydrateForm(c, &out[i]); e != nil {
			return nil, e
		}
	}
	return out, nil
}

func (r *Repository) SaveEncounterForm(c context.Context, encounterID, actor string, q clinical.SaveEncounterFormRequest) (*clinical.EncounterForm, error) {
	raw, _ := json.Marshal(q.Answers)
	tag, e := r.exec(c).Exec(c, "UPDATE clinical_encounter_forms SET answers=$3,revision=revision+1,saved_by=$4 WHERE encounter_id=$1 AND revision=$2 AND status='draft'", encounterID, q.Revision, raw, actor)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, clinical.ErrConflict
	}
	return r.GetEncounterForm(c, encounterID)
}

func (r *Repository) SubmitEncounterForm(c context.Context, form clinical.EncounterForm, actor string, observations []clinical.Observation) (*clinical.EncounterForm, error) {
	exec := r.exec(c)
	if _, e := exec.Exec(c, "SAVEPOINT clinical_form_submit"); e != nil {
		return nil, e
	}
	rollback := func(err error) (*clinical.EncounterForm, error) {
		_, _ = exec.Exec(c, "ROLLBACK TO SAVEPOINT clinical_form_submit")
		_, _ = exec.Exec(c, "RELEASE SAVEPOINT clinical_form_submit")
		return nil, err
	}
	tag, e := exec.Exec(c, "UPDATE clinical_encounter_forms SET status='submitted',submitted_by=$3,submitted_at=now(),revision=revision+1 WHERE id=$1 AND revision=$2 AND status='draft'", form.ID, form.Revision, actor)
	if e != nil {
		return rollback(e)
	}
	if tag.RowsAffected() != 1 {
		return rollback(clinical.ErrConflict)
	}
	for _, x := range observations {
		_, e = exec.Exec(c, "INSERT INTO clinical_observations(id,patient_id,visit_id,encounter_id,code,value_numeric,value_text,unit,observed_at,recorded_by,source_form_id,source_form_field_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)", x.ID, x.PatientID, x.VisitID, x.EncounterID, x.Code, x.ValueNumeric, x.ValueText, x.Unit, x.ObservedAt, x.RecordedBy, form.ID, x.SourceFormFieldKey)
		if e != nil {
			return rollback(e)
		}
	}
	if _, e = exec.Exec(c, "RELEASE SAVEPOINT clinical_form_submit"); e != nil {
		return nil, e
	}
	return r.GetEncounterForm(c, form.EncounterID)
}
