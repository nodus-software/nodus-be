CREATE TYPE clinical_catalogue_kind AS ENUM ('services', 'medications');
CREATE TYPE catalogue_import_mode AS ENUM ('create_only', 'upsert');
CREATE TYPE catalogue_import_status AS ENUM ('validated', 'committed', 'expired');

CREATE TABLE catalogue_reference_releases (
    id UUID PRIMARY KEY,
    catalogue clinical_catalogue_kind NOT NULL,
    source_system TEXT NOT NULL,
    version TEXT NOT NULL,
    title TEXT NOT NULL,
    released_on DATE,
    provenance_url TEXT,
    checksum TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT false,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (catalogue, source_system, version)
);

CREATE UNIQUE INDEX uq_active_catalogue_reference_release
    ON catalogue_reference_releases (catalogue, source_system) WHERE active;

CREATE TABLE service_reference_items (
    id UUID PRIMARY KEY,
    release_id UUID NOT NULL REFERENCES catalogue_reference_releases(id) ON DELETE CASCADE,
    source_code TEXT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('laboratory','imaging','procedure','nursing_service','consultation','therapy','other')),
    standard_system TEXT,
    standard_code TEXT,
    searchable_text TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (release_id, source_code)
);

CREATE INDEX idx_service_reference_search ON service_reference_items
    USING gin (to_tsvector('simple', searchable_text));

CREATE TABLE medication_reference_items (
    id UUID PRIMARY KEY,
    release_id UUID NOT NULL REFERENCES catalogue_reference_releases(id) ON DELETE CASCADE,
    source_code TEXT NOT NULL,
    generic_name TEXT NOT NULL,
    strength TEXT,
    dosage_form TEXT,
    route TEXT,
    standard_system TEXT,
    standard_code TEXT,
    searchable_text TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (release_id, source_code)
);

CREATE INDEX idx_medication_reference_search ON medication_reference_items
    USING gin (to_tsvector('simple', searchable_text));

CREATE TABLE clinical_services (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    reference_item_id UUID REFERENCES service_reference_items(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('laboratory','imaging','procedure','nursing_service','consultation','therapy','other')),
    department_id UUID REFERENCES departments(id),
    service_point_id UUID REFERENCES service_points(id),
    price_minor BIGINT CHECK (price_minor IS NULL OR price_minor >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'KES',
    requires_order BOOLEAN NOT NULL DEFAULT true,
    requires_result BOOLEAN NOT NULL DEFAULT false,
    estimated_duration_minutes INTEGER CHECK (estimated_duration_minutes IS NULL OR estimated_duration_minutes > 0),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_clinical_service_code ON clinical_services (tenant_id, lower(code));
CREATE INDEX idx_clinical_services_list ON clinical_services (tenant_id, active, category, name);

CREATE TABLE medication_catalogue (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    reference_item_id UUID REFERENCES medication_reference_items(id),
    code TEXT NOT NULL,
    generic_name TEXT NOT NULL,
    brand_name TEXT,
    strength TEXT,
    dosage_form TEXT,
    route TEXT,
    pack_size TEXT,
    unit_of_measure TEXT,
    prescription_required BOOLEAN NOT NULL DEFAULT true,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_medication_catalogue_code ON medication_catalogue (tenant_id, lower(code));
CREATE INDEX idx_medication_catalogue_list ON medication_catalogue (tenant_id, active, generic_name);

CREATE TABLE catalogue_imports (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    catalogue clinical_catalogue_kind NOT NULL,
    mode catalogue_import_mode NOT NULL,
    status catalogue_import_status NOT NULL DEFAULT 'validated',
    file_name TEXT NOT NULL,
    file_checksum TEXT NOT NULL,
    rows JSONB NOT NULL,
    summary JSONB NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    committed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_catalogue_import_expiry ON catalogue_imports (tenant_id, expires_at);

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['clinical_services','medication_catalogue','catalogue_imports']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;

CREATE TRIGGER clinical_services_set_updated_at BEFORE UPDATE ON clinical_services
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER medication_catalogue_set_updated_at BEFORE UPDATE ON medication_catalogue
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000024', 'catalogues:manage', 'Configure clinical service and medication catalogues')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;
