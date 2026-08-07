DROP INDEX IF EXISTS clinical_allergies_active_catalogue_key;
ALTER TABLE clinical_allergies DROP COLUMN IF EXISTS allergen_id;
DROP TABLE IF EXISTS allergen_catalogue;
DROP TABLE IF EXISTS tenant_terminology_overrides;
DROP TABLE IF EXISTS terminology_import_runs;
DROP INDEX IF EXISTS terminology_concepts_active_search;
DROP INDEX IF EXISTS terminology_concepts_linearization_uri_key;
ALTER TABLE terminology_concepts DROP COLUMN IF EXISTS primary_tabulation, DROP COLUMN IF EXISTS is_residual, DROP COLUMN IF EXISTS is_leaf, DROP COLUMN IF EXISTS class_kind, DROP COLUMN IF EXISTS parent_uri, DROP COLUMN IF EXISTS chapter_no, DROP COLUMN IF EXISTS source_title, DROP COLUMN IF EXISTS linearization_uri, DROP COLUMN IF EXISTS foundation_uri;
ALTER TABLE terminology_releases DROP COLUMN IF EXISTS attribution, DROP COLUMN IF EXISTS source_file, DROP COLUMN IF EXISTS source_checksum, DROP COLUMN IF EXISTS linearization, DROP COLUMN IF EXISTS language;
