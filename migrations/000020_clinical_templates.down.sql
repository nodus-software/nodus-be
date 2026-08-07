DROP INDEX IF EXISTS uq_observation_form_field;
ALTER TABLE clinical_observations DROP COLUMN IF EXISTS source_form_field_key, DROP COLUMN IF EXISTS source_form_id;
DROP TABLE IF EXISTS clinical_encounter_forms, clinical_template_versions, clinical_templates;
DELETE FROM permissions WHERE code='templates:manage';
