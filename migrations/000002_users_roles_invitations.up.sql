-- Invited users have no password until they accept their invitation and set
-- one themselves, so the column can no longer be NOT NULL.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

CREATE TABLE invitations (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invited_by   UUID NOT NULL REFERENCES users(id),
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_invitations_user_id ON invitations (user_id);

-- Short-lived, single-use token scoped only to completing MFA enrollment
-- right after an invite is accepted — deliberately separate from a real
-- session/refresh token so it can never be used to call any other endpoint.
CREATE TABLE enrollment_tokens (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    consumed_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_enrollment_tokens_user_id ON enrollment_tokens (user_id);
