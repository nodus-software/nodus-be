ALTER TABLE terminology_releases ADD COLUMN language TEXT NOT NULL DEFAULT 'en', ADD COLUMN linearization TEXT, ADD COLUMN source_checksum TEXT, ADD COLUMN source_file TEXT, ADD COLUMN attribution TEXT;
ALTER TABLE terminology_concepts ADD COLUMN foundation_uri TEXT, ADD COLUMN linearization_uri TEXT, ADD COLUMN source_title TEXT, ADD COLUMN chapter_no TEXT, ADD COLUMN parent_uri TEXT, ADD COLUMN class_kind TEXT, ADD COLUMN is_leaf BOOLEAN NOT NULL DEFAULT true, ADD COLUMN is_residual BOOLEAN NOT NULL DEFAULT false, ADD COLUMN primary_tabulation BOOLEAN NOT NULL DEFAULT true;
CREATE UNIQUE INDEX terminology_concepts_linearization_uri_key ON terminology_concepts (linearization_uri) WHERE linearization_uri IS NOT NULL;
CREATE INDEX terminology_concepts_active_search ON terminology_concepts (release_id, primary_tabulation, chapter_no, code);

CREATE TABLE terminology_import_runs (id UUID PRIMARY KEY, system TEXT NOT NULL, version TEXT, source_file TEXT NOT NULL, source_checksum TEXT NOT NULL, status TEXT NOT NULL CHECK (status IN ('validated','committed','failed')), total_rows INTEGER NOT NULL DEFAULT 0, imported_rows INTEGER NOT NULL DEFAULT 0, error_message TEXT, started_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ);
CREATE TABLE tenant_terminology_overrides (id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, concept_id UUID NOT NULL REFERENCES terminology_concepts(id) ON DELETE CASCADE, enabled BOOLEAN NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id, concept_id));
CREATE TABLE allergen_catalogue (id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, code TEXT NOT NULL, name TEXT NOT NULL, category TEXT NOT NULL CHECK (category IN ('medication','food','contrast','contact','insect','environmental','other')), aliases TEXT[] NOT NULL DEFAULT '{}', active BOOLEAN NOT NULL DEFAULT true, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE UNIQUE INDEX allergen_catalogue_tenant_code_ci_key ON allergen_catalogue (tenant_id, lower(trim(code)));
CREATE UNIQUE INDEX allergen_catalogue_tenant_name_ci_key ON allergen_catalogue (tenant_id, lower(trim(name)));
CREATE INDEX allergen_catalogue_search_idx ON allergen_catalogue USING gin (to_tsvector('simple', name));
ALTER TABLE clinical_allergies ADD COLUMN allergen_id UUID REFERENCES allergen_catalogue(id);
CREATE UNIQUE INDEX clinical_allergies_active_catalogue_key ON clinical_allergies (tenant_id, patient_id, allergen_id) WHERE status = 'active' AND allergen_id IS NOT NULL;

INSERT INTO allergen_catalogue (id,tenant_id,code,name,category,aliases)
SELECT gen_random_uuid(),o.id,v.code,v.name,v.category,v.aliases FROM organizations o CROSS JOIN (VALUES
 ('penicillins','Penicillins','medication',ARRAY['penicillin','penicillin antibiotics']), ('cephalosporins','Cephalosporins','medication',ARRAY['cephalosporin antibiotics']),
 ('sulfonamide_antibiotics','Sulfonamide antibiotics','medication',ARRAY['sulfa','sulphonamide','sulphur']), ('nsaids','Non-steroidal anti-inflammatory drugs','medication',ARRAY['NSAIDs','anti-inflammatory medicines']),
 ('amoxicillin','Amoxicillin','medication',ARRAY['amoxycillin']), ('amoxicillin_clavulanate','Amoxicillin/clavulanate','medication',ARRAY['co-amoxiclav','amox-clav','augmentin']),
 ('cotrimoxazole','Co-trimoxazole','medication',ARRAY['cotrimoxazole','trimethoprim-sulfamethoxazole','septrin']), ('aspirin','Aspirin','medication',ARRAY['acetylsalicylic acid']),
 ('ibuprofen','Ibuprofen','medication',ARRAY[]::text[]), ('diclofenac','Diclofenac','medication',ARRAY[]::text[]), ('paracetamol','Paracetamol','medication',ARRAY['acetaminophen']),
 ('azithromycin','Azithromycin','medication',ARRAY[]::text[]), ('erythromycin','Erythromycin','medication',ARRAY[]::text[]), ('ciprofloxacin','Ciprofloxacin','medication',ARRAY[]::text[]),
 ('metronidazole','Metronidazole','medication',ARRAY[]::text[]), ('nevirapine','Nevirapine','medication',ARRAY[]::text[]), ('abacavir','Abacavir','medication',ARRAY[]::text[]),
 ('zidovudine','Zidovudine','medication',ARRAY['AZT']), ('carbamazepine','Carbamazepine','medication',ARRAY[]::text[]), ('phenytoin','Phenytoin','medication',ARRAY[]::text[]),
 ('cows_milk','Cow''s milk','food',ARRAY['milk','dairy']), ('egg','Egg','food',ARRAY['eggs']), ('peanut','Peanut','food',ARRAY['groundnut']), ('tree_nuts','Tree nuts','food',ARRAY['nuts']),
 ('fish','Fish','food',ARRAY[]::text[]), ('shellfish','Shellfish','food',ARRAY['crustaceans','molluscs']), ('wheat','Wheat','food',ARRAY['gluten']), ('soy','Soy','food',ARRAY['soya']), ('sesame','Sesame','food',ARRAY[]::text[]),
 ('iodinated_contrast','Iodinated contrast media','contrast',ARRAY['iodine contrast','radiographic contrast']), ('gadolinium_contrast','Gadolinium contrast media','contrast',ARRAY['MRI contrast']),
 ('natural_rubber_latex','Natural rubber latex','contact',ARRAY['latex']), ('chlorhexidine','Chlorhexidine','contact',ARRAY[]::text[]), ('bee_venom','Bee venom','insect',ARRAY['bee sting']), ('wasp_venom','Wasp venom','insect',ARRAY['wasp sting']),
 ('house_dust_mite','House dust mite','environmental',ARRAY['dust mite']), ('pollen','Pollen','environmental',ARRAY[]::text[]), ('mould','Mould','environmental',ARRAY['mold']), ('animal_dander','Animal dander','environmental',ARRAY['pet dander'])
) v(code,name,category,aliases);

DO $$ DECLARE t text; BEGIN FOREACH t IN ARRAY ARRAY['tenant_terminology_overrides','allergen_catalogue'] LOOP EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t); EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t); EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t); EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t); EXECUTE format('CREATE TRIGGER %I_set_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t); END LOOP; END $$;
