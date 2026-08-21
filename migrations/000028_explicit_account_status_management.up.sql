ALTER TABLE users
    ADD COLUMN suspended_at TIMESTAMPTZ,
    ADD COLUMN suspended_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN suspension_reason TEXT,
    ADD COLUMN deactivated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN deactivation_reason TEXT;

UPDATE users SET suspended_at = updated_at, suspension_reason = 'migration_existing_status'
WHERE status = 'suspended';
UPDATE users SET deactivation_reason = 'migration_existing_status'
WHERE status = 'deactivated';

ALTER TABLE users ADD CONSTRAINT users_suspension_metadata_status CHECK (
    status = 'suspended' OR (suspended_at IS NULL AND suspended_by IS NULL AND suspension_reason IS NULL)
);
ALTER TABLE users ADD CONSTRAINT users_deactivation_metadata_status CHECK (
    status = 'deactivated' OR (deactivated_at IS NULL AND deactivated_by IS NULL AND deactivation_reason IS NULL)
);
