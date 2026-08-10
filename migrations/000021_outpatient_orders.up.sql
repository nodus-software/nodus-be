ALTER TABLE clinical_visits ADD COLUMN service_point_id UUID REFERENCES service_points(id);

ALTER TABLE queue_routing_rules
  ADD COLUMN encounter_type clinical_encounter_type,
  ADD COLUMN order_kind TEXT CHECK (order_kind IS NULL OR order_kind IN ('service','medication')),
  ADD COLUMN service_category TEXT;

CREATE TABLE clinical_orders (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    patient_id UUID NOT NULL REFERENCES patients(id),
    visit_id UUID NOT NULL REFERENCES clinical_visits(id),
    encounter_id UUID NOT NULL REFERENCES clinical_encounters(id),
    kind TEXT NOT NULL CHECK (kind IN ('service','medication')),
    category TEXT NOT NULL,
    priority SMALLINT NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100),
    status TEXT NOT NULL DEFAULT 'ordered' CHECK (status IN ('ordered','accepted','in_progress','completed','rejected','cancelled')),
    review_required BOOLEAN NOT NULL DEFAULT true,
    ordered_by UUID NOT NULL REFERENCES users(id),
    ordered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    transition_reason TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clinical_orders_visit ON clinical_orders (visit_id, ordered_at DESC);
CREATE INDEX idx_clinical_orders_worklist ON clinical_orders (tenant_id, category, status, priority DESC, ordered_at);

CREATE TABLE clinical_service_order_details (
    order_id UUID PRIMARY KEY REFERENCES clinical_orders(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    service_id UUID NOT NULL REFERENCES clinical_services(id),
    service_code TEXT NOT NULL,
    service_name TEXT NOT NULL
);

CREATE TABLE clinical_medication_order_details (
    order_id UUID PRIMARY KEY REFERENCES clinical_orders(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    medication_id UUID NOT NULL REFERENCES medication_catalogue(id),
    medication_code TEXT NOT NULL,
    medication_name TEXT NOT NULL,
    dose NUMERIC NOT NULL CHECK (dose > 0),
    dose_unit TEXT NOT NULL,
    route TEXT NOT NULL,
    frequency TEXT NOT NULL,
    duration_days INTEGER CHECK (duration_days IS NULL OR duration_days > 0),
    quantity NUMERIC CHECK (quantity IS NULL OR quantity > 0),
    instructions TEXT,
    allergy_override_reason TEXT,
    allergy_acknowledged_at TIMESTAMPTZ
);

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['clinical_orders','clinical_service_order_details','clinical_medication_order_details']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;

CREATE TRIGGER clinical_orders_set_updated_at BEFORE UPDATE ON clinical_orders
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (id, code, description) VALUES
 ('00000000-0000-4000-8000-000000000030','outpatient:read','View outpatient records'),
 ('00000000-0000-4000-8000-000000000031','outpatient:complete','Complete outpatient visits'),
 ('00000000-0000-4000-8000-000000000032','outpatient:workflow-override','Override the configured outpatient workflow'),
 ('00000000-0000-4000-8000-000000000033','orders:read','View clinical orders'),
 ('00000000-0000-4000-8000-000000000034','orders:place','Place clinical service orders'),
 ('00000000-0000-4000-8000-000000000035','orders:fulfill','Progress clinical orders'),
 ('00000000-0000-4000-8000-000000000036','orders:cancel','Cancel clinical orders'),
 ('00000000-0000-4000-8000-000000000037','prescriptions:place','Place medication prescriptions'),
 ('00000000-0000-4000-8000-000000000038','queues:view','View operational queues'),
 ('00000000-0000-4000-8000-000000000039','queues:operate','Operate and move queue entries'),
 ('00000000-0000-4000-8000-000000000040','queues:configure','Configure queues and routing')
ON CONFLICT (code) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO role_permissions(role_id,permission_id,tenant_id)
SELECT DISTINCT rp.role_id,newp.id,rp.tenant_id
FROM role_permissions rp
JOIN permissions oldp ON oldp.id=rp.permission_id
JOIN permissions newp ON
  (oldp.code='clinical:read' AND newp.code IN ('outpatient:read','orders:read','queues:view')) OR
  (oldp.code='clinical:write' AND newp.code IN ('outpatient:triage','outpatient:consult','outpatient:complete','orders:place','prescriptions:place')) OR
  (oldp.code='queues:manage' AND newp.code IN ('queues:operate','queues:configure'))
ON CONFLICT DO NOTHING;
