-- Bed identifiers only need to be unique inside their room. This permits
-- Room 1 / Bed 1 and Room 2 / Bed 1, including rooms in the same ward.
ALTER TABLE beds DROP CONSTRAINT beds_tenant_id_ward_id_code_key;

ALTER TABLE beds
    ADD CONSTRAINT beds_tenant_id_room_id_code_key UNIQUE (tenant_id, room_id, code);
