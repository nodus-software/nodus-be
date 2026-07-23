DROP TABLE IF EXISTS enrollment_tokens;
DROP TABLE IF EXISTS invitations;

ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
