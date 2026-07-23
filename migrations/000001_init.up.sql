-- Core auth/RBAC/audit schema.
-- UUIDs are always generated and supplied by the application (pkg/utility),
-- never by the database, so no column below has a UUID default.

CREATE TYPE user_status AS ENUM ('invited', 'active', 'suspended', 'pending_review');
CREATE TYPE mfa_factor_type AS ENUM ('totp', 'biometric');
CREATE TYPE audit_result AS ENUM ('success', 'failure');

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE users (
    id                      UUID PRIMARY KEY,
    full_name               TEXT NOT NULL,
    username                TEXT NOT NULL UNIQUE,
    email                   TEXT NOT NULL UNIQUE,
    password_hash           TEXT NOT NULL,
    provider_identifier     TEXT,
    status                  user_status NOT NULL DEFAULT 'invited',
    failed_login_attempts   INTEGER NOT NULL DEFAULT 0,
    locked_until            TIMESTAMPTZ,
    password_changed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_access_review_at   TIMESTAMPTZ,
    next_access_review_due  TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_status ON users (status);

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE roles (
    id                             UUID PRIMARY KEY,
    name                           TEXT NOT NULL UNIQUE,
    description                    TEXT NOT NULL DEFAULT '',
    is_superuser_role              BOOLEAN NOT NULL DEFAULT false,
    requires_provider_identifier   BOOLEAN NOT NULL DEFAULT false,
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER roles_set_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE permissions (
    id           UUID PRIMARY KEY,
    code         TEXT NOT NULL UNIQUE,
    description  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE role_permissions (
    role_id        UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id  UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id      UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);

CREATE TABLE mfa_factors (
    id                 UUID PRIMARY KEY,
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type               mfa_factor_type NOT NULL,
    label              TEXT NOT NULL DEFAULT '',
    secret_encrypted   TEXT,
    public_key         TEXT,
    confirmed_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mfa_factor_secret_or_key CHECK (
        (type = 'totp' AND secret_encrypted IS NOT NULL) OR
        (type = 'biometric' AND public_key IS NOT NULL)
    )
);
CREATE INDEX idx_mfa_factors_user_id ON mfa_factors (user_id);

CREATE TABLE mfa_backup_codes (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash   TEXT NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mfa_backup_codes_user_id ON mfa_backup_codes (user_id);

CREATE TABLE login_challenges (
    id                     UUID PRIMARY KEY,
    user_id                UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    challenge_token_hash   TEXT NOT NULL UNIQUE,
    expires_at             TIMESTAMPTZ NOT NULL,
    consumed_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_login_challenges_user_id ON login_challenges (user_id);

CREATE TABLE sessions (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_label    TEXT NOT NULL DEFAULT '',
    ip_address      TEXT NOT NULL DEFAULT '',
    user_agent      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_active_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);
CREATE INDEX idx_sessions_user_id ON sessions (user_id);

CREATE TABLE refresh_tokens (
    id           UUID PRIMARY KEY,
    session_id   UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_session_id ON refresh_tokens (session_id);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

CREATE TABLE password_reset_tokens (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens (user_id);

CREATE TABLE password_reset_attempts (
    id                   UUID PRIMARY KEY,
    username_attempted   TEXT NOT NULL,
    ip_address           TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_password_reset_attempts_username ON password_reset_attempts (username_attempted, created_at);
CREATE INDEX idx_password_reset_attempts_ip ON password_reset_attempts (ip_address, created_at);

-- Append-only: no code path ever issues UPDATE/DELETE against this table.
CREATE TABLE audit_logs (
    id                UUID PRIMARY KEY,
    "timestamp"       TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    action            TEXT NOT NULL,
    target_resource   TEXT NOT NULL DEFAULT '',
    ip_address        TEXT NOT NULL DEFAULT '',
    result            audit_result NOT NULL,
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_user_id ON audit_logs (user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs (action);
CREATE INDEX idx_audit_logs_timestamp ON audit_logs ("timestamp");
