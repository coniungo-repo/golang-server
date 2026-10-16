-- +goose Up
CREATE TABLE users (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    status     text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_identities (
    id                  bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id             uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind                text        NOT NULL CHECK (kind IN ('phone', 'email', 'oidc')),
    subject             text        NOT NULL,
    email               text,
    supertokens_user_id text        NOT NULL UNIQUE,
    verified_at         timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, subject)
);

CREATE INDEX user_identities_user_id_idx ON user_identities (user_id);

COMMENT ON COLUMN user_identities.subject IS 'E.164 phone, lowercased email, or issuer|sub for OpenID Connect. Never an email for oidc: emails can be renamed or reissued.';

CREATE TABLE user_roles (
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       text        NOT NULL CHECK (role IN ('customer', 'provider', 'admin')),
    granted_by uuid        REFERENCES users (id) ON DELETE SET NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role)
);

CREATE TABLE admin_invites (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email       text        NOT NULL,
    invited_by  uuid        REFERENCES users (id) ON DELETE SET NULL,
    invited_at  timestamptz NOT NULL DEFAULT now(),
    accepted_by uuid        REFERENCES users (id) ON DELETE SET NULL,
    accepted_at timestamptz,
    revoked_at  timestamptz
);

CREATE UNIQUE INDEX admin_invites_open_email_idx ON admin_invites (email) WHERE revoked_at IS NULL;

CREATE TABLE admin_audit_log (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    action     text        NOT NULL,
    target     text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_log_created_at_idx ON admin_audit_log (created_at);

-- +goose Down
DROP TABLE admin_audit_log;
DROP TABLE admin_invites;
DROP TABLE user_roles;
DROP TABLE user_identities;
DROP TABLE users;
