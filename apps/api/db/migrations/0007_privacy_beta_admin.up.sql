-- Invite-gated beta access, privacy preferences, and admin browser sessions.

ALTER TABLE users ADD COLUMN beta_access_at timestamptz;

CREATE TABLE user_privacy_settings (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    analytics_opt_out boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token_hash bytea NOT NULL UNIQUE,
    csrf_token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz
);

CREATE INDEX admin_sessions_user_expires_idx ON admin_sessions(user_id, expires_at);

ALTER TABLE appeals ADD COLUMN body_revisions jsonb NOT NULL DEFAULT '[]'::jsonb;
