DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code IN ('outpatient:check-in','outpatient:triage','outpatient:consult','outpatient:duplicate-override'));
DELETE FROM permissions WHERE code IN ('outpatient:check-in','outpatient:triage','outpatient:consult','outpatient:duplicate-override');
DROP INDEX IF EXISTS uq_active_encounter_kind_per_visit;
DROP INDEX IF EXISTS idx_active_outpatient_visits;
DROP TABLE IF EXISTS clinical_allergies;
