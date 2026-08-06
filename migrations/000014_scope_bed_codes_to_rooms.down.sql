ALTER TABLE beds DROP CONSTRAINT IF EXISTS beds_tenant_id_room_id_code_key;

ALTER TABLE beds
    ADD CONSTRAINT beds_tenant_id_ward_id_code_key UNIQUE (tenant_id, ward_id, code);
