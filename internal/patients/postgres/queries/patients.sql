-- name: ListPatients :many
SELECT * FROM patients
WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND (sqlc.narg(q)::text IS NULL OR
       full_name ILIKE '%' || sqlc.narg(q)::text || '%' OR
       mrn ILIKE '%' || sqlc.narg(q)::text || '%' OR
       national_id ILIKE '%' || sqlc.narg(q)::text || '%' OR
       phone ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(statuses)::patient_status[] IS NULL OR status = ANY(sqlc.narg(statuses)::patient_status[]))
  AND (sqlc.narg(genders)::patient_gender[] IS NULL OR gender = ANY(sqlc.narg(genders)::patient_gender[]))
  AND (sqlc.narg(insured)::boolean IS NULL OR insured = sqlc.narg(insured)::boolean)
  AND (sqlc.narg(reg_from)::date IS NULL OR created_at::date >= sqlc.narg(reg_from)::date)
  AND (sqlc.narg(reg_to)::date IS NULL OR created_at::date <= sqlc.narg(reg_to)::date)
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_val) OFFSET sqlc.arg(offset_val);

-- name: CountPatients :one
SELECT count(*) FROM patients
WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND (sqlc.narg(q)::text IS NULL OR
       full_name ILIKE '%' || sqlc.narg(q)::text || '%' OR
       mrn ILIKE '%' || sqlc.narg(q)::text || '%' OR
       national_id ILIKE '%' || sqlc.narg(q)::text || '%' OR
       phone ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(statuses)::patient_status[] IS NULL OR status = ANY(sqlc.narg(statuses)::patient_status[]))
  AND (sqlc.narg(genders)::patient_gender[] IS NULL OR gender = ANY(sqlc.narg(genders)::patient_gender[]))
  AND (sqlc.narg(insured)::boolean IS NULL OR insured = sqlc.narg(insured)::boolean)
  AND (sqlc.narg(reg_from)::date IS NULL OR created_at::date >= sqlc.narg(reg_from)::date)
  AND (sqlc.narg(reg_to)::date IS NULL OR created_at::date <= sqlc.narg(reg_to)::date);

-- name: GetPatientByID :one
SELECT * FROM patients
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- IssuePatientMRNNumber atomically issues the next sequential MRN number
-- for the current tenant. First call for a tenant inserts the counter row
-- (seeded so the *next* value is 2) and returns 1; every call after that
-- increments and returns the pre-increment value - so numbers issued are
-- 1, 2, 3, ... per tenant, with no separate provisioning step required.
-- name: IssuePatientMRNNumber :one
INSERT INTO patient_mrn_sequences (tenant_id, next_value)
VALUES (NULLIF(current_setting('app.tenant_id', true), '')::uuid, 2)
ON CONFLICT (tenant_id) DO UPDATE SET next_value = patient_mrn_sequences.next_value + 1
RETURNING next_value - 1 AS issued;

-- name: InsertPatient :exec
INSERT INTO patients (
  id, tenant_id, mrn, full_name, dob, dob_estimated, approx_age_years, gender,
  phone, address, national_id, status, insured, guardian_id
) VALUES (
  sqlc.arg(id), NULLIF(current_setting('app.tenant_id', true), '')::uuid, sqlc.arg(mrn), sqlc.arg(full_name),
  sqlc.narg(dob)::date, sqlc.arg(dob_estimated), sqlc.narg(approx_age_years)::smallint, sqlc.arg(gender)::patient_gender,
  sqlc.narg(phone), sqlc.narg(address), sqlc.narg(national_id), 'active', sqlc.arg(insured), sqlc.narg(guardian_id)::uuid
);

-- name: UpdatePatientContact :exec
UPDATE patients SET phone = $2, address = $3
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: MarkPatientDeceased :exec
UPDATE patients SET status = 'deceased', date_of_death = $2
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: SetPatientMergedInto :exec
UPDATE patients SET status = 'merged', merged_into_id = $2
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: UpdatePatientFullName :exec
UPDATE patients SET full_name = $2
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: UpdatePatientDOB :exec
UPDATE patients SET dob = $2::date
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: UpdatePatientGender :exec
UPDATE patients SET gender = $2::patient_gender
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: UpdatePatientNationalID :exec
UPDATE patients SET national_id = $2
WHERE id = $1 AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- FindDuplicateCandidates ranks existing (non-merged) patients in the
-- tenant by a combined trigram-name-similarity + exact-field score:
-- name: FindDuplicateCandidates :many
SELECT p.*,
  similarity(p.full_name, sqlc.arg(full_name)::text) AS name_score,
  (sqlc.narg(dob)::date IS NOT NULL AND p.dob = sqlc.narg(dob)::date) AS dob_exact,
  (sqlc.narg(national_id)::text IS NOT NULL AND p.national_id = sqlc.narg(national_id)::text) AS national_id_exact,
  (sqlc.narg(phone)::text IS NOT NULL AND p.phone = sqlc.narg(phone)::text) AS phone_exact
FROM patients p
WHERE p.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND p.status != 'merged'
  AND (p.full_name % sqlc.arg(full_name)::text
       OR (sqlc.narg(national_id)::text IS NOT NULL AND p.national_id = sqlc.narg(national_id)::text)
       OR (sqlc.narg(phone)::text IS NOT NULL AND p.phone = sqlc.narg(phone)::text))
ORDER BY (similarity(p.full_name, sqlc.arg(full_name)::text)
  + (CASE WHEN sqlc.narg(dob)::date IS NOT NULL AND p.dob = sqlc.narg(dob)::date THEN 0.4 ELSE 0 END)
  + (CASE WHEN sqlc.narg(national_id)::text IS NOT NULL AND p.national_id = sqlc.narg(national_id)::text THEN 0.5 ELSE 0 END)
  + (CASE WHEN sqlc.narg(phone)::text IS NOT NULL AND p.phone = sqlc.narg(phone)::text THEN 0.2 ELSE 0 END)) DESC
LIMIT 10;

-- name: InsertCorrection :exec
INSERT INTO patient_corrections (id, patient_id, field, current_value, requested_value, evidence_note, submitted_by)
VALUES (sqlc.arg(id), sqlc.arg(patient_id), sqlc.arg(field), sqlc.narg(current_value), sqlc.arg(requested_value), sqlc.narg(evidence_note), sqlc.narg(submitted_by)::uuid);

-- name: ListCorrections :many
SELECT * FROM patient_corrections WHERE patient_id = $1 ORDER BY created_at DESC;

-- name: GetCorrectionByID :one
SELECT * FROM patient_corrections WHERE id = $1;

-- name: DecideCorrection :exec
UPDATE patient_corrections SET status = $2::patient_correction_status, decided_by = $3::uuid, decided_at = $4, decision_note = $5
WHERE id = $1;

-- name: InsertIdentifier :exec
INSERT INTO patient_identifiers (id, patient_id, id_type, id_value)
VALUES ($1, $2, $3, $4);

-- name: DeleteIdentifier :exec
DELETE FROM patient_identifiers WHERE id = $1 AND patient_id = $2;

-- name: ListIdentifiers :many
SELECT * FROM patient_identifiers WHERE patient_id = $1 ORDER BY created_at;

-- name: CountIdentifiers :one
SELECT count(*) FROM patient_identifiers WHERE patient_id = $1;

-- name: ListConsents :many
SELECT * FROM patient_consents WHERE patient_id = $1 ORDER BY scope;

-- name: UpsertConsent :one
INSERT INTO patient_consents (id, patient_id, scope, granted, granted_at, revoked_at)
VALUES (sqlc.arg(id), sqlc.arg(patient_id), sqlc.arg(scope), sqlc.arg(granted), sqlc.narg(granted_at), sqlc.narg(revoked_at))
ON CONFLICT (patient_id, scope) DO UPDATE SET granted = EXCLUDED.granted, granted_at = EXCLUDED.granted_at, revoked_at = EXCLUDED.revoked_at
RETURNING *;

-- name: InsertActivity :exec
INSERT INTO patient_activity_log (id, patient_id, user_id, kind, text)
VALUES ($1, $2, $3, $4, $5);

-- name: ListActivity :many
SELECT * FROM patient_activity_log WHERE patient_id = $1 ORDER BY created_at DESC;

-- name: ReassignIdentifiers :exec
UPDATE patient_identifiers SET patient_id = $2 WHERE patient_id = $1;

-- Consents carry a UNIQUE(patient_id, scope) constraint, so any away-side
-- scope the keep side already has is left attached to the (now-merged,
-- never-deleted) away record rather than conflicting.
-- name: ReassignConsents :exec
UPDATE patient_consents pc SET patient_id = $2
WHERE pc.patient_id = $1
  AND pc.scope NOT IN (SELECT scope FROM patient_consents WHERE patient_id = $2);

-- name: ReassignCorrections :exec
UPDATE patient_corrections SET patient_id = $2 WHERE patient_id = $1;

-- name: ReassignActivity :exec
UPDATE patient_activity_log SET patient_id = $2 WHERE patient_id = $1;
