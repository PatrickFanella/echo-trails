DROP INDEX IF EXISTS media_assets_echo_sort_unique_idx;

ALTER TABLE media_assets DROP CONSTRAINT media_assets_status_check;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_status_check
    CHECK (status IN ('pending_upload', 'uploaded', 'processing', 'approved', 'rejected', 'failed', 'deleted'));

ALTER TABLE echoes ADD COLUMN private_creation_location geography(Point, 4326);
ALTER TABLE echoes DROP CONSTRAINT IF EXISTS echoes_creation_token_id_fkey;

ALTER TABLE user_profiles DROP COLUMN IF EXISTS handle_changed_at;

ALTER TABLE sessions
    DROP COLUMN IF EXISTS device_name,
    DROP COLUMN IF EXISTS access_expires_at;

DROP TABLE IF EXISTS email_auth_requests;
