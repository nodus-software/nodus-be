CREATE TYPE clinical_visit_type AS ENUM ('test', 'outpatient', 'emergency', 'specialty');
CREATE TYPE clinical_visit_status AS ENUM ('planned', 'active', 'completed', 'cancelled');
CREATE TYPE clinical_encounter_type AS ENUM ('triage', 'consultation', 'ward_round', 'nursing', 'other');
CREATE TYPE clinical_encounter_status AS ENUM ('planned', 'in_progress', 'completed', 'cancelled');
CREATE TYPE queue_entry_status AS ENUM ('waiting', 'called', 'in_service', 'paused', 'transferred', 'completed', 'cancelled', 'no_show');
CREATE TYPE queue_subject_type AS ENUM ('visit', 'admission', 'order');
CREATE TYPE diagnosis_kind AS ENUM ('provisional', 'final', 'secondary');
CREATE TYPE outbox_status AS ENUM ('pending', 'processing', 'processed', 'failed');

CREATE TABLE departments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code),
    UNIQUE (tenant_id, name)
);

CREATE TABLE service_points (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    department_id UUID NOT NULL REFERENCES departments(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'other',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE wards (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    department_id UUID NOT NULL REFERENCES departments(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE rooms (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    ward_id UUID NOT NULL REFERENCES wards(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, ward_id, code)
);

CREATE TABLE beds (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    ward_id UUID NOT NULL REFERENCES wards(id),
    room_id UUID REFERENCES rooms(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'available' CHECK (status IN ('available', 'occupied', 'reserved', 'maintenance', 'inactive')),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, ward_id, code)
);

CREATE TABLE queues (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    service_point_id UUID NOT NULL REFERENCES service_points(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE clinical_visits (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id),
    visit_type clinical_visit_type NOT NULL,
    status clinical_visit_status NOT NULL DEFAULT 'active',
    reason TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clinical_visits_patient ON clinical_visits (patient_id, started_at DESC);
CREATE INDEX idx_clinical_visits_status ON clinical_visits (tenant_id, status);

CREATE TABLE clinical_encounters (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    visit_id UUID NOT NULL REFERENCES clinical_visits(id) ON DELETE CASCADE,
    service_point_id UUID REFERENCES service_points(id),
    encounter_type clinical_encounter_type NOT NULL,
    status clinical_encounter_status NOT NULL DEFAULT 'planned',
    clinician_id UUID REFERENCES users(id),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clinical_observations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id),
    visit_id UUID NOT NULL REFERENCES clinical_visits(id),
    encounter_id UUID REFERENCES clinical_encounters(id),
    code TEXT NOT NULL,
    value_numeric NUMERIC,
    value_text TEXT,
    unit TEXT,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    recorded_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((value_numeric IS NOT NULL) <> (value_text IS NOT NULL))
);

CREATE TABLE terminology_releases (
    id UUID PRIMARY KEY,
    system TEXT NOT NULL,
    version TEXT NOT NULL,
    title TEXT NOT NULL,
    released_on DATE,
    active BOOLEAN NOT NULL DEFAULT false,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (system, version)
);

CREATE TABLE terminology_concepts (
    id UUID PRIMARY KEY,
    release_id UUID NOT NULL REFERENCES terminology_releases(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    display TEXT NOT NULL,
    searchable_text TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (release_id, code)
);
CREATE INDEX idx_terminology_concepts_search ON terminology_concepts USING gin (to_tsvector('simple', searchable_text));

CREATE TABLE clinical_diagnoses (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id),
    visit_id UUID NOT NULL REFERENCES clinical_visits(id),
    encounter_id UUID REFERENCES clinical_encounters(id),
    concept_id UUID NOT NULL REFERENCES terminology_concepts(id),
    kind diagnosis_kind NOT NULL,
    note TEXT,
    recorded_by UUID NOT NULL REFERENCES users(id),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clinical_notes (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id),
    visit_id UUID NOT NULL REFERENCES clinical_visits(id),
    encounter_id UUID REFERENCES clinical_encounters(id),
    note_type TEXT NOT NULL,
    body TEXT NOT NULL,
    authored_by UUID NOT NULL REFERENCES users(id),
    authored_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    amended_from_id UUID REFERENCES clinical_notes(id)
);

CREATE TABLE queue_entries (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    queue_id UUID NOT NULL REFERENCES queues(id),
    subject_type queue_subject_type NOT NULL,
    subject_id UUID NOT NULL,
    patient_id UUID NOT NULL REFERENCES patients(id),
    status queue_entry_status NOT NULL DEFAULT 'waiting',
    priority SMALLINT NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100),
    acuity SMALLINT CHECK (acuity BETWEEN 1 AND 5),
    position_override INTEGER,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    called_at TIMESTAMPTZ,
    service_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_queue_entries_board ON queue_entries (queue_id, status, priority DESC, position_override, joined_at);
CREATE UNIQUE INDEX uq_active_queue_subject ON queue_entries (queue_id, subject_type, subject_id)
WHERE status IN ('waiting', 'called', 'in_service', 'paused');

CREATE TABLE queue_entry_history (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    queue_entry_id UUID NOT NULL REFERENCES queue_entries(id) ON DELETE CASCADE,
    from_status queue_entry_status,
    to_status queue_entry_status NOT NULL,
    from_queue_id UUID REFERENCES queues(id),
    to_queue_id UUID NOT NULL REFERENCES queues(id),
    actor_id UUID REFERENCES users(id),
    reason TEXT,
    automated BOOLEAN NOT NULL DEFAULT false,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE queue_routing_rules (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    visit_type clinical_visit_type,
    target_queue_id UUID NOT NULL REFERENCES queues(id),
    priority SMALLINT NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE clinical_outbox (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status outbox_status NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clinical_outbox_pending ON clinical_outbox (status, available_at) WHERE status IN ('pending', 'failed');

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['departments','service_points','wards','rooms','beds','queues',
    'clinical_visits','clinical_encounters','clinical_observations','clinical_diagnoses','clinical_notes',
    'queue_entries','queue_entry_history','queue_routing_rules','clinical_outbox']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['departments','service_points','wards','rooms','beds','queues','clinical_visits','clinical_encounters','queue_entries']
  LOOP
    EXECUTE format('CREATE TRIGGER %I_set_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
  END LOOP;
END $$;

INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000016', 'clinical:read', 'View visits, clinical records and queues'),
  ('00000000-0000-4000-8000-000000000017', 'clinical:write', 'Create and update visits and clinical records'),
  ('00000000-0000-4000-8000-000000000018', 'queues:manage', 'Configure queues and move patients'),
  ('00000000-0000-4000-8000-000000000019', 'facilities:manage', 'Configure departments, service points, wards and beds')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;

