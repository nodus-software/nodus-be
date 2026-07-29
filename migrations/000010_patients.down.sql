DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'patients:merge');
DELETE FROM permissions WHERE code = 'patients:merge';

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['patients','patient_mrn_sequences','patient_identifiers',
    'patient_consents','patient_corrections','patient_activity_log']
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS stamp_tenant_id ON %I', t);
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
    EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
  END LOOP;
END $$;

DROP TABLE IF EXISTS patient_activity_log;
DROP TABLE IF EXISTS patient_corrections;
DROP TABLE IF EXISTS patient_consents;
DROP TABLE IF EXISTS patient_identifiers;
DROP TABLE IF EXISTS patient_mrn_sequences;
DROP TABLE IF EXISTS patients;

DROP TYPE IF EXISTS patient_correction_status;
DROP TYPE IF EXISTS patient_gender;
DROP TYPE IF EXISTS patient_status;

DROP EXTENSION IF EXISTS pg_trgm;
