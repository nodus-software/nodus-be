-- Make tenant-owned clinical vocabularies case-insensitively unique and add
-- the remaining prescription/laboratory reference lists.

-- Keep one row for case-only duplicates. Prefer active and then the oldest row.
-- Medication catalogue values are codes, so normalize them before removing an
-- alternate spelling.
ALTER TABLE medication_catalogue NO FORCE ROW LEVEL SECURITY;
UPDATE medication_catalogue SET dosage_form = lower(trim(dosage_form)) WHERE dosage_form IS NOT NULL;
UPDATE medication_catalogue SET route = lower(trim(route)) WHERE route IS NOT NULL;
UPDATE medication_catalogue SET unit_of_measure = lower(trim(unit_of_measure)) WHERE unit_of_measure IS NOT NULL;
ALTER TABLE medication_catalogue FORCE ROW LEVEL SECURITY;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['medication_dosage_forms','administration_routes','units_of_measure']
  LOOP
    EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
    EXECUTE format($sql$
      DELETE FROM %I victim USING %I keeper
      WHERE victim.tenant_id = keeper.tenant_id
        AND victim.id <> keeper.id
        AND (lower(trim(victim.code)) = lower(trim(keeper.code))
          OR lower(trim(victim.name)) = lower(trim(keeper.name)))
        AND (victim.active, victim.created_at, victim.id) < (keeper.active, keeper.created_at, keeper.id)
    $sql$, t, t);
    EXECUTE format('UPDATE %I SET code=lower(trim(code)), name=trim(name)', t);
    EXECUTE format('CREATE UNIQUE INDEX %I_tenant_code_ci_key ON %I (tenant_id, lower(trim(code)))', t, t);
    EXECUTE format('CREATE UNIQUE INDEX %I_tenant_name_ci_key ON %I (tenant_id, lower(trim(name)))', t, t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
  END LOOP;
END $$;

CREATE TABLE prescription_frequencies (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX prescription_frequencies_tenant_code_ci_key ON prescription_frequencies (tenant_id, lower(trim(code)));
CREATE UNIQUE INDEX prescription_frequencies_tenant_name_ci_key ON prescription_frequencies (tenant_id, lower(trim(name)));

CREATE TABLE specimen_types (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX specimen_types_tenant_code_ci_key ON specimen_types (tenant_id, lower(trim(code)));
CREATE UNIQUE INDEX specimen_types_tenant_name_ci_key ON specimen_types (tenant_id, lower(trim(name)));

INSERT INTO prescription_frequencies (id, tenant_id, code, name)
SELECT gen_random_uuid(), o.id, v.code, v.name FROM organizations o CROSS JOIN (VALUES
 ('OD','Once daily'), ('TD','Twice daily'),
 ('TDS','Three times daily'), ('QDS','Four times daily'),
 ('Q4H','Every 4 hours'), ('Q6H','Every 6 hours'),
 ('Q8H','Every 8 hours'), ('Q12H','Every 12 hours'),
 ('NOCTE','At night'), ('PRN','As needed'), ('STAT','Immediately')
) v(code,name);

INSERT INTO specimen_types (id, tenant_id, code, name)
SELECT gen_random_uuid(), o.id, v.code, v.name FROM organizations o CROSS JOIN (VALUES
 ('whole_blood','Whole blood'), ('serum','Serum'), ('plasma','Plasma'),
 ('urine','Urine'), ('stool','Stool'), ('sputum','Sputum'), ('swab','Swab'),
 ('cerebrospinal_fluid','Cerebrospinal fluid'), ('tissue','Tissue')
) v(code,name);

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['prescription_frequencies','specimen_types']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
    EXECUTE format('CREATE TRIGGER %I_set_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
  END LOOP;
END $$;
