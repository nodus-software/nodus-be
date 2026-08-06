ALTER TABLE beds DROP CONSTRAINT IF EXISTS beds_room_ward_fk;
ALTER TABLE beds DROP CONSTRAINT IF EXISTS beds_room_id_required;
ALTER TABLE rooms DROP CONSTRAINT IF EXISTS rooms_id_ward_id_unique;
