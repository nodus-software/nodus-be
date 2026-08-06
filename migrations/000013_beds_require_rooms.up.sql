-- Preserve existing legacy rows while enforcing the ward -> room -> bed
-- hierarchy for every new or updated bed.
ALTER TABLE rooms
    ADD CONSTRAINT rooms_id_ward_id_unique UNIQUE (id, ward_id);

ALTER TABLE beds
    ADD CONSTRAINT beds_room_id_required
    CHECK (room_id IS NOT NULL) NOT VALID;

ALTER TABLE beds
    ADD CONSTRAINT beds_room_ward_fk
    FOREIGN KEY (room_id, ward_id) REFERENCES rooms (id, ward_id) NOT VALID;
