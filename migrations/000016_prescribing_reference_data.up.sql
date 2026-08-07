-- Prescribing reference data: the code lists the medication catalogue picks from.
--
-- Dosage form, route and unit of measure were free text on medication_catalogue,
-- so "Tab", "tablet" and "TABLET" were three different values. They become
-- tenant-owned vocabularies served by the generic /clinical/config/{kind}
-- resource API, seeded here and editable from Clinical configuration ->
-- Reference data.
--
-- Seed values mirror internal/clinical/prescribing_defaults.go, which seeds the
-- same lists for tenants registered after this migration. Keep the two in step.

CREATE TABLE medication_dosage_forms (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code),
    UNIQUE (tenant_id, name)
);

CREATE TABLE administration_routes (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code),
    UNIQUE (tenant_id, name)
);

CREATE TABLE units_of_measure (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code),
    UNIQUE (tenant_id, name)
);

-- Seed every existing tenant. This runs before row-level security is enabled
-- below: the tenant_isolation policy and the stamp_tenant_id trigger both key
-- off app.tenant_id, which a migration has no single value for.
INSERT INTO medication_dosage_forms (id, tenant_id, code, name)
SELECT gen_random_uuid(), o.id, v.code, v.name
FROM organizations o
CROSS JOIN (VALUES
    ('tablet', 'Tablet'),
    ('capsule', 'Capsule'),
    ('oral_solution', 'Oral solution'),
    ('oral_suspension', 'Oral suspension'),
    ('syrup', 'Syrup'),
    ('powder_for_suspension', 'Powder for suspension'),
    ('injection', 'Injection'),
    ('infusion', 'Infusion'),
    ('cream', 'Cream'),
    ('ointment', 'Ointment'),
    ('gel', 'Gel'),
    ('lotion', 'Lotion'),
    ('eye_drops', 'Eye drops'),
    ('ear_drops', 'Ear drops'),
    ('nasal_spray', 'Nasal spray'),
    ('inhaler', 'Inhaler'),
    ('nebuliser_solution', 'Nebuliser solution'),
    ('suppository', 'Suppository'),
    ('pessary', 'Pessary'),
    ('transdermal_patch', 'Transdermal patch'),
    ('granules', 'Granules'),
    ('implant', 'Implant')
) AS v(code, name);

INSERT INTO administration_routes (id, tenant_id, code, name)
SELECT gen_random_uuid(), o.id, v.code, v.name
FROM organizations o
CROSS JOIN (VALUES
    ('oral', 'Oral'),
    ('sublingual', 'Sublingual'),
    ('buccal', 'Buccal'),
    ('intravenous', 'Intravenous (IV)'),
    ('intramuscular', 'Intramuscular (IM)'),
    ('subcutaneous', 'Subcutaneous (SC)'),
    ('intradermal', 'Intradermal'),
    ('intrathecal', 'Intrathecal'),
    ('topical', 'Topical'),
    ('transdermal', 'Transdermal'),
    ('rectal', 'Rectal'),
    ('vaginal', 'Vaginal'),
    ('ophthalmic', 'Ophthalmic (eye)'),
    ('otic', 'Otic (ear)'),
    ('nasal', 'Nasal'),
    ('inhalation', 'Inhalation')
) AS v(code, name);

INSERT INTO units_of_measure (id, tenant_id, code, name)
SELECT gen_random_uuid(), o.id, v.code, v.name
FROM organizations o
CROSS JOIN (VALUES
    ('tablet', 'Tablet(s)'),
    ('capsule', 'Capsule(s)'),
    ('bottle', 'Bottle'),
    ('vial', 'Vial'),
    ('ampoule', 'Ampoule'),
    ('sachet', 'Sachet'),
    ('tube', 'Tube'),
    ('blister', 'Blister pack'),
    ('pre_filled_syringe', 'Pre-filled syringe'),
    ('suppository', 'Suppository'),
    ('patch', 'Patch'),
    ('inhaler', 'Inhaler'),
    ('ml', 'Millilitre (mL)'),
    ('g', 'Gram (g)'),
    ('piece', 'Piece')
) AS v(code, name);

-- Normalise the free text already on medication_catalogue onto the seeded codes.
-- medication_catalogue forces row-level security, which applies to the table
-- owner too, so it has to be relaxed for the length of these statements.
ALTER TABLE medication_catalogue NO FORCE ROW LEVEL SECURITY;

UPDATE medication_catalogue m
SET dosage_form = alias.code
FROM (VALUES
    ('tab', 'tablet'), ('tabs', 'tablet'), ('tablets', 'tablet'),
    ('cap', 'capsule'), ('caps', 'capsule'), ('capsules', 'capsule'),
    ('susp', 'oral_suspension'), ('suspension', 'oral_suspension'),
    ('soln', 'oral_solution'), ('solution', 'oral_solution'),
    ('inj', 'injection'), ('neb', 'nebuliser_solution'),
    ('patch', 'transdermal_patch')
) AS alias(raw, code)
WHERE lower(trim(m.dosage_form)) = alias.raw;

UPDATE medication_catalogue m
SET route = alias.code
FROM (VALUES
    ('po', 'oral'), ('iv', 'intravenous'), ('im', 'intramuscular'),
    ('sc', 'subcutaneous'), ('sq', 'subcutaneous'), ('subcut', 'subcutaneous'),
    ('pr', 'rectal'), ('pv', 'vaginal'), ('top', 'topical'),
    ('inhaled', 'inhalation'), ('neb', 'inhalation')
) AS alias(raw, code)
WHERE lower(trim(m.route)) = alias.raw;

UPDATE medication_catalogue m
SET unit_of_measure = alias.code
FROM (VALUES
    ('mls', 'ml'), ('millilitre', 'ml'), ('milliliter', 'ml'), ('millilitres', 'ml'),
    ('gram', 'g'), ('grams', 'g'), ('gm', 'g'),
    ('amp', 'ampoule'), ('amps', 'ampoule'),
    ('pcs', 'piece'), ('each', 'piece'), ('unit', 'piece'), ('units', 'piece'),
    ('tabs', 'tablet'), ('tablets', 'tablet'),
    ('caps', 'capsule'), ('capsules', 'capsule'),
    ('syringe', 'pre_filled_syringe')
) AS alias(raw, code)
WHERE lower(trim(m.unit_of_measure)) = alias.raw;

-- Case-only differences ("TABLET") match a seeded code once lowered.
UPDATE medication_catalogue m SET dosage_form = lower(trim(m.dosage_form))
WHERE m.dosage_form IS NOT NULL AND EXISTS (
    SELECT 1 FROM medication_dosage_forms d
    WHERE d.tenant_id = m.tenant_id AND d.code = lower(trim(m.dosage_form)));
UPDATE medication_catalogue m SET route = lower(trim(m.route))
WHERE m.route IS NOT NULL AND EXISTS (
    SELECT 1 FROM administration_routes r
    WHERE r.tenant_id = m.tenant_id AND r.code = lower(trim(m.route)));
UPDATE medication_catalogue m SET unit_of_measure = lower(trim(m.unit_of_measure))
WHERE m.unit_of_measure IS NOT NULL AND EXISTS (
    SELECT 1 FROM units_of_measure u
    WHERE u.tenant_id = m.tenant_id AND u.code = lower(trim(m.unit_of_measure)));

-- Whatever is left has no home in the vocabulary. Clearing it is deliberate:
-- an admin re-picks the value from the dropdown rather than the catalogue
-- carrying values nothing can interpret.
UPDATE medication_catalogue m SET dosage_form = NULL
WHERE m.dosage_form IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM medication_dosage_forms d WHERE d.tenant_id = m.tenant_id AND d.code = m.dosage_form);
UPDATE medication_catalogue m SET route = NULL
WHERE m.route IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM administration_routes r WHERE r.tenant_id = m.tenant_id AND r.code = m.route);
UPDATE medication_catalogue m SET unit_of_measure = NULL
WHERE m.unit_of_measure IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM units_of_measure u WHERE u.tenant_id = m.tenant_id AND u.code = m.unit_of_measure);

ALTER TABLE medication_catalogue FORCE ROW LEVEL SECURITY;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['medication_dosage_forms','administration_routes','units_of_measure']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
    EXECUTE format('CREATE TRIGGER %I_set_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
  END LOOP;
END $$;
