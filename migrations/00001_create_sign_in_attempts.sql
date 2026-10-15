-- +goose Up
CREATE TABLE sign_in_attempts (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text        NOT NULL,
    fingerprint bytea       NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sign_in_attempts_lookup_idx ON sign_in_attempts (kind, fingerprint, created_at);
CREATE INDEX sign_in_attempts_created_at_idx ON sign_in_attempts (created_at);

COMMENT ON TABLE sign_in_attempts IS 'Sign-in code requests for rate limiting. fingerprint is an HMAC of the email, phone or IP, never the raw value.';

-- +goose Down
DROP TABLE sign_in_attempts;
