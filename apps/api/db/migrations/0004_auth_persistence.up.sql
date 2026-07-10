-- Durable passwordless authentication and bearer-session support.

CREATE TABLE email_auth_requests (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    code_hash bytea NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0 AND attempts <= 5),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_auth_requests_email_created_idx ON email_auth_requests(email, created_at DESC);
CREATE INDEX email_auth_requests_expires_idx ON email_auth_requests(expires_at) WHERE used_at IS NULL;

ALTER TABLE sessions
    ADD COLUMN access_expires_at timestamptz,
    ADD COLUMN device_name text NOT NULL DEFAULT 'unknown device';

ALTER TABLE user_profiles ADD COLUMN handle_changed_at timestamptz;

ALTER TABLE echoes
    ADD CONSTRAINT echoes_creation_token_id_fkey
    FOREIGN KEY (creation_token_id) REFERENCES creation_authorizations(id) ON DELETE SET NULL;

-- Exact coordinates live only in application-encrypted creation_authorizations.
ALTER TABLE echoes DROP COLUMN private_creation_location;

ALTER TABLE media_assets DROP CONSTRAINT media_assets_status_check;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_status_check
    CHECK (status IN ('pending_upload', 'uploaded', 'processing', 'pending_review', 'approved', 'rejected', 'failed', 'deleted'));

CREATE UNIQUE INDEX media_assets_echo_sort_unique_idx
    ON media_assets(echo_id, sort_order) WHERE echo_id IS NOT NULL AND media_type <> 'quote_snapshot';
