-- +migrate Up
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT,
    oauth_provider TEXT NOT NULL,
    oauth_id TEXT NOT NULL,
    phone TEXT UNIQUE,
    role TEXT NOT NULL DEFAULT 'user',
    admin_granted_by BIGINT,
    admin_granted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);
-- GetByOAuth runs on every sign-in. Unique, not just an index: it also stops two
-- concurrent first-logins for one Google account from both inserting a row.
CREATE UNIQUE INDEX idx_users_oauth ON users(oauth_provider, oauth_id);

CREATE TABLE refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    token_id TEXT NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    family_id TEXT NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens(family_id);

-- +migrate Down
DROP TABLE refresh_tokens;
DROP TABLE users;
