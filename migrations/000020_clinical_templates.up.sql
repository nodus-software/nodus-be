CREATE TABLE clinical_templates (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    encounter_type clinical_encounter_type NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    archived_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code)
);
CREATE UNIQUE INDEX uq_default_clinical_template
ON clinical_templates (tenant_id, encounter_type)
WHERE is_default AND archived_at IS NULL;

CREATE TABLE clinical_template_versions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES clinical_templates(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('draft','published','superseded')),
    definition JSONB NOT NULL,
    created_by UUID REFERENCES users(id),
    published_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    UNIQUE (template_id, version)
);
CREATE UNIQUE INDEX uq_clinical_template_draft
ON clinical_template_versions (template_id) WHERE status='draft';
CREATE UNIQUE INDEX uq_clinical_template_published
ON clinical_template_versions (template_id) WHERE status='published';

CREATE TABLE clinical_encounter_forms (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    encounter_id UUID NOT NULL UNIQUE REFERENCES clinical_encounters(id) ON DELETE CASCADE,
    template_version_id UUID NOT NULL REFERENCES clinical_template_versions(id),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','submitted')),
    answers JSONB NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    saved_by UUID REFERENCES users(id),
    submitted_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    submitted_at TIMESTAMPTZ
);

ALTER TABLE clinical_observations
    ADD COLUMN source_form_id UUID REFERENCES clinical_encounter_forms(id),
    ADD COLUMN source_form_field_key TEXT;
CREATE UNIQUE INDEX uq_observation_form_field
ON clinical_observations (source_form_id, source_form_field_key)
WHERE source_form_id IS NOT NULL;

INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000025', 'templates:manage', 'Configure clinical encounter templates')
ON CONFLICT (code) DO UPDATE SET description=EXCLUDED.description;

-- System starter definitions for tenants that already exist. Registration uses
-- the same definitions in Go for tenants created after this migration.
WITH tenant_templates AS (
  INSERT INTO clinical_templates(id,tenant_id,code,name,description,encounter_type,is_default)
  SELECT gen_random_uuid(),id,'outpatient-triage','Outpatient Triage','Core outpatient triage observations','triage'::clinical_encounter_type,true FROM organizations
  UNION ALL
  SELECT gen_random_uuid(),id,'outpatient-consultation','Outpatient Consultation','Practical outpatient consultation note','consultation'::clinical_encounter_type,true FROM organizations
  RETURNING id,tenant_id,code
)
INSERT INTO clinical_template_versions(id,tenant_id,template_id,version,status,definition,published_at)
SELECT gen_random_uuid(),tenant_id,id,1,'published',
  CASE code
    WHEN 'outpatient-triage' THEN '{"schema_version":1,"sections":[{"key":"vitals","title":"Vital signs","fields":[{"key":"temperature","label":"Temperature","type":"number","required":true,"binding":{"kind":"observation","code":"temperature","unit":"Cel"}},{"key":"pulse","label":"Pulse","type":"number","required":true,"binding":{"kind":"observation","code":"pulse","unit":"/min"}},{"key":"respiratory_rate","label":"Respiratory rate","type":"number","required":true,"binding":{"kind":"observation","code":"respiratory-rate","unit":"/min"}},{"key":"systolic_bp","label":"Systolic blood pressure","type":"number","required":true,"binding":{"kind":"observation","code":"blood-pressure-systolic","unit":"mm[Hg]"}},{"key":"diastolic_bp","label":"Diastolic blood pressure","type":"number","required":true,"binding":{"kind":"observation","code":"blood-pressure-diastolic","unit":"mm[Hg]"}},{"key":"oxygen_saturation","label":"Oxygen saturation","type":"number","required":false,"validation":{"min":0,"max":100},"binding":{"kind":"observation","code":"oxygen-saturation","unit":"%"}},{"key":"weight","label":"Weight","type":"number","required":false,"binding":{"kind":"observation","code":"body-weight","unit":"kg"}},{"key":"height","label":"Height","type":"number","required":false,"binding":{"kind":"observation","code":"body-height","unit":"cm"}}]},{"key":"notes","title":"Triage notes","fields":[{"key":"triage_notes","label":"Triage notes","type":"long_text","required":false}]}]}'::jsonb
    ELSE '{"schema_version":1,"sections":[{"key":"history","title":"History","fields":[{"key":"presenting_complaint","label":"Presenting complaint and history","type":"long_text","required":true},{"key":"medical_history","label":"Relevant medical history","type":"long_text","required":false}]},{"key":"examination","title":"Examination","fields":[{"key":"examination_findings","label":"Examination findings","type":"long_text","required":false}]},{"key":"assessment_plan","title":"Assessment and plan","fields":[{"key":"clinical_assessment","label":"Clinical assessment","type":"long_text","required":true},{"key":"management_plan","label":"Management plan","type":"long_text","required":true}]}]}'::jsonb
  END,
  now()
FROM tenant_templates;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['clinical_templates','clinical_template_versions','clinical_encounter_forms']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;
CREATE TRIGGER clinical_templates_set_updated_at BEFORE UPDATE ON clinical_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER clinical_encounter_forms_set_updated_at BEFORE UPDATE ON clinical_encounter_forms FOR EACH ROW EXECUTE FUNCTION set_updated_at();
