ALTER TABLE users
    DROP CONSTRAINT users_deactivation_metadata_status,
    DROP CONSTRAINT users_suspension_metadata_status,
    DROP COLUMN deactivation_reason,
    DROP COLUMN deactivated_by,
    DROP COLUMN suspension_reason,
    DROP COLUMN suspended_by,
    DROP COLUMN suspended_at;
