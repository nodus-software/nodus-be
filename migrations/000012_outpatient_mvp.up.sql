CREATE TABLE clinical_allergies (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id),
    allergen TEXT NOT NULL,
    reaction TEXT,
    severity TEXT CHECK (severity IS NULL OR severity IN ('mild','moderate','severe')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','entered_in_error')),
    recorded_by UUID NOT NULL REFERENCES users(id),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clinical_allergies_patient ON clinical_allergies (patient_id, status, recorded_at DESC);
CREATE INDEX idx_active_outpatient_visits ON clinical_visits (tenant_id, patient_id, started_at DESC)
WHERE visit_type = 'outpatient' AND status = 'active';
CREATE UNIQUE INDEX uq_active_encounter_kind_per_visit ON clinical_encounters (visit_id, encounter_type)
WHERE status IN ('planned','in_progress');

ALTER TABLE clinical_allergies ENABLE ROW LEVEL SECURITY;
ALTER TABLE clinical_allergies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON clinical_allergies
USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON clinical_allergies
FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id();

INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000020', 'outpatient:check-in', 'Check patients into outpatient care'),
  ('00000000-0000-4000-8000-000000000021', 'outpatient:triage', 'Record and complete outpatient triage'),
  ('00000000-0000-4000-8000-000000000022', 'outpatient:consult', 'Document outpatient consultations and complete visits'),
  ('00000000-0000-4000-8000-000000000023', 'outpatient:duplicate-override', 'Override the active outpatient visit safeguard')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;
