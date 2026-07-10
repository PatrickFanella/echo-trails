DELETE FROM place_provider_mappings WHERE provider = 'openstreetmap' AND provider_place_id IN (
    'way/35362323', 'node/25779937', 'way/200192088', 'way/200218663', 'way/200054182',
    'way/552662784', 'way/567113043', 'way/684600471', 'way/548571172'
);
DELETE FROM places WHERE id IN (
    '019f486a-0001-7000-8000-000000000001', '019f486a-0002-7000-8000-000000000002',
    '019f486a-0003-7000-8000-000000000003', '019f486a-0004-7000-8000-000000000004',
    '019f486a-0005-7000-8000-000000000005', '019f486a-0006-7000-8000-000000000006',
    '019f486a-0007-7000-8000-000000000007', '019f486a-0008-7000-8000-000000000008',
    '019f486a-0009-7000-8000-000000000009'
);

DROP TRIGGER IF EXISTS transcripts_search_update ON media_transcripts;
DROP TRIGGER IF EXISTS echoes_search_update ON echoes;
DROP TRIGGER IF EXISTS places_search_update ON places;
DROP FUNCTION IF EXISTS echotrails_transcripts_search_update();
DROP FUNCTION IF EXISTS echotrails_echoes_search_update();
DROP FUNCTION IF EXISTS echotrails_places_search_update();

DROP INDEX IF EXISTS media_transcripts_search_document_gin_idx;
DROP INDEX IF EXISTS echoes_search_document_gin_idx;
DROP INDEX IF EXISTS place_aliases_name_trgm_idx;
DROP INDEX IF EXISTS places_name_trgm_idx;
DROP INDEX IF EXISTS places_search_document_gin_idx;

ALTER TABLE media_transcripts DROP COLUMN IF EXISTS search_document;
ALTER TABLE echoes DROP COLUMN IF EXISTS search_document;
ALTER TABLE places DROP COLUMN IF EXISTS search_document;

ALTER TABLE media_assets
    DROP COLUMN IF EXISTS moderation_hold,
    DROP COLUMN IF EXISTS original_expires_at,
    DROP COLUMN IF EXISTS capture_source,
    DROP COLUMN IF EXISTS sort_order;

DROP TABLE IF EXISTS privacy_jobs;
DROP TABLE IF EXISTS place_alert_deliveries;
DROP TABLE IF EXISTS echo_rank_snapshots;
DROP TABLE IF EXISTS user_trust_scores;
DROP TABLE IF EXISTS echo_engagement_events;
DROP TABLE IF EXISTS notification_mutes;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS user_acknowledgements;
DROP TABLE IF EXISTS reply_mentions;
DROP TABLE IF EXISTS reply_revisions;
DROP TABLE IF EXISTS place_imports;
DROP TABLE IF EXISTS creation_authorizations;

ALTER TABLE devices
    DROP COLUMN IF EXISTS notifications_authorized_at,
    DROP COLUMN IF EXISTS push_environment,
    DROP COLUMN IF EXISTS push_token_ciphertext;

DROP INDEX IF EXISTS sessions_refresh_token_hash_unique_idx;
DROP INDEX IF EXISTS sessions_access_token_hash_unique_idx;
ALTER TABLE sessions
    DROP COLUMN IF EXISTS device_id,
    DROP COLUMN IF EXISTS refresh_token_hash,
    DROP COLUMN IF EXISTS access_token_hash;
