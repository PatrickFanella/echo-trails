CREATE TABLE session_refresh_replays (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    request_refresh_hash bytea NOT NULL,
    idempotency_key text NOT NULL,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (request_refresh_hash, idempotency_key)
);
CREATE INDEX session_refresh_replays_expiry_idx ON session_refresh_replays(expires_at);
