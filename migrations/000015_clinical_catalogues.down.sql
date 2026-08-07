DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'catalogues:manage');
DELETE FROM permissions WHERE code = 'catalogues:manage';

DROP TABLE IF EXISTS catalogue_imports;
DROP TABLE IF EXISTS medication_catalogue;
DROP TABLE IF EXISTS clinical_services;
DROP TABLE IF EXISTS medication_reference_items;
DROP TABLE IF EXISTS service_reference_items;
DROP INDEX IF EXISTS uq_active_catalogue_reference_release;
DROP TABLE IF EXISTS catalogue_reference_releases;
DROP TYPE IF EXISTS catalogue_import_status;
DROP TYPE IF EXISTS catalogue_import_mode;
DROP TYPE IF EXISTS clinical_catalogue_kind;
