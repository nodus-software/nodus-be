CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TYPE patient_status AS ENUM ('active', 'deceased', 'merged');
CREATE TYPE patient_gender AS ENUM ('male', 'female', 'unknown');
CREATE TYPE patient_correction_status AS ENUM ('pending', 'actioned', 'rejected');

CREATE TABLE patients (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL REFERENCES organizations(id),
    mrn               TEXT NOT NULL,
    full_name         TEXT NOT NULL,
    dob               DATE,
    dob_estimated     BOOLEAN NOT NULL DEFAULT false,
    approx_age_years  SMALLINT,
    gender            patient_gender NOT NULL DEFAULT 'unknown',
    phone             TEXT,
    address           TEXT,
    national_id       TEXT,
    status            patient_status NOT NULL DEFAULT 'active',
    date_of_death     TIMESTAMPTZ,
    insured           BOOLEAN NOT NULL DEFAULT false,
    guardian_id       UUID REFERENCES patients(id),
    merged_into_id    UUID REFERENCES patients(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT patients_tenant_mrn_key UNIQUE (tenant_id, mrn)
);
-- Partial: unlimited unidentified/minor patients with national_id = NULL may coexist.
CREATE UNIQUE INDEX patients_tenant_national_id_key ON patients (tenant_id, national_id) WHERE national_id IS NOT NULL;
CREATE INDEX idx_patients_tenant_id ON patients (tenant_id);
CREATE INDEX idx_patients_tenant_status ON patients (tenant_id, status);
CREATE INDEX idx_patients_tenant_phone ON patients (tenant_id, phone) WHERE phone IS NOT NULL;
CREATE INDEX idx_patients_guardian_id ON patients (guardian_id) WHERE guardian_id IS NOT NULL;
CREATE INDEX idx_patients_merged_into_id ON patients (merged_into_id) WHERE merged_into_id IS NOT NULL;
CREATE INDEX idx_patients_full_name_trgm ON patients USING gin (full_name gin_trgm_ops);
CREATE TRIGGER patients_set_updated_at BEFORE UPDATE ON patients FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Per-tenant, atomically-incrementing MRN counter. The UPDATE ... RETURNING
-- used to issue an MRN row-locks only within the issuing tenant.
CREATE TABLE patient_mrn_sequences (
    tenant_id  UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_value BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE patient_identifiers (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations(id),
    patient_id  UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    id_type     TEXT NOT NULL,
    id_value    TEXT NOT NULL,
    verified_at TIMESTAMPTZ, -- NULL => "Pending verification"; no verification flow yet
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_patient_identifiers_patient_id ON patient_identifiers (patient_id);

CREATE TABLE patient_consents (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations(id),
    patient_id  UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    scope       TEXT NOT NULL,
    granted     BOOLEAN NOT NULL DEFAULT false,
    granted_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT patient_consents_patient_scope_key UNIQUE (patient_id, scope)
);
CREATE TRIGGER patient_consents_set_updated_at BEFORE UPDATE ON patient_consents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE patient_corrections (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES organizations(id),
    patient_id      UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    field           TEXT NOT NULL, -- full_name | dob | gender | national_id
    current_value   TEXT,
    requested_value TEXT NOT NULL,
    evidence_note   TEXT,
    status          patient_correction_status NOT NULL DEFAULT 'pending',
    submitted_by    UUID REFERENCES users(id),
    decided_by      UUID REFERENCES users(id),
    decided_at      TIMESTAMPTZ,
    decision_note   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_patient_corrections_patient_id ON patient_corrections (patient_id);
CREATE INDEX idx_patient_corrections_tenant_status ON patient_corrections (tenant_id, status);
CREATE TRIGGER patient_corrections_set_updated_at BEFORE UPDATE ON patient_corrections FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Single source of truth for "who did what to this patient, when" - both
-- free-text staff notes (kind='note') and system-generated entries
-- (kind='system': consent toggles, identifier add/remove, mark-deceased,
-- correction decisions, merges) land here, so the child tables above need
-- no created_by column of their own to show attribution in the Activity tab.
CREATE TABLE patient_activity_log (
    id         UUID PRIMARY KEY,
    tenant_id  UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    user_id    UUID REFERENCES users(id),
    kind       TEXT NOT NULL DEFAULT 'note',
    text       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_patient_activity_log_patient_id ON patient_activity_log (patient_id, created_at DESC);

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['patients','patient_mrn_sequences','patient_identifiers',
    'patient_consents','patient_corrections','patient_activity_log']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)',
      t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;

INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000015', 'patients:merge', 'Merge duplicate patient records (destructive, hard to reverse)')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;
