-- Full-feature beta persistence, search, ranking, notification, and demo-place foundation.

CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE sessions
    ADD COLUMN access_token_hash bytea,
    ADD COLUMN refresh_token_hash bytea,
    ADD COLUMN device_id uuid REFERENCES devices(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX sessions_access_token_hash_unique_idx ON sessions(access_token_hash) WHERE access_token_hash IS NOT NULL;
CREATE UNIQUE INDEX sessions_refresh_token_hash_unique_idx ON sessions(refresh_token_hash) WHERE refresh_token_hash IS NOT NULL;

ALTER TABLE devices
    ADD COLUMN push_token_ciphertext bytea,
    ADD COLUMN push_environment text CHECK (push_environment IN ('sandbox', 'production')),
    ADD COLUMN notifications_authorized_at timestamptz;

CREATE TABLE creation_authorizations (
    id uuid PRIMARY KEY,
    token_hash bytea NOT NULL UNIQUE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    exact_location_ciphertext bytea NOT NULL,
    proof_captured_at timestamptz NOT NULL,
    accuracy_meters numeric(8,2) NOT NULL CHECK (accuracy_meters > 0 AND accuracy_meters <= 100),
    expires_at timestamptz NOT NULL,
    consumed_idempotency_key text,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX creation_authorizations_user_expires_idx ON creation_authorizations(user_id, expires_at);

CREATE TABLE place_imports (
    id uuid PRIMARY KEY,
    provider text NOT NULL,
    source_version text NOT NULL,
    source_url text NOT NULL,
    attribution text NOT NULL,
    bounds jsonb NOT NULL DEFAULT '{}'::jsonb,
    imported_at timestamptz NOT NULL DEFAULT now(),
    imported_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE reply_revisions (
    id uuid PRIMARY KEY,
    reply_id uuid NOT NULL REFERENCES replies(id) ON DELETE CASCADE,
    revision_number integer NOT NULL CHECK (revision_number > 0),
    body_text text NOT NULL CHECK (char_length(body_text) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (reply_id, revision_number)
);

CREATE TABLE reply_mentions (
    reply_id uuid NOT NULL REFERENCES replies(id) ON DELETE CASCADE,
    mentioned_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (reply_id, mentioned_user_id)
);

CREATE TABLE user_acknowledgements (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    acknowledgement_type text NOT NULL CHECK (acknowledgement_type IN ('first_reply_quote_warning', 'stitch_reply_snapshot_warning')),
    acknowledged_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, acknowledgement_type)
);

CREATE TABLE notification_preferences (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    direct_activity_enabled boolean NOT NULL DEFAULT true,
    place_alerts_enabled boolean NOT NULL DEFAULT true,
    moderation_enabled boolean NOT NULL DEFAULT true,
    upload_status_enabled boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE notification_mutes (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    place_id uuid REFERENCES places(id) ON DELETE CASCADE,
    echo_id uuid REFERENCES echoes(id) ON DELETE CASCADE,
    muted_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((place_id IS NOT NULL)::integer + (echo_id IS NOT NULL)::integer = 1),
    UNIQUE NULLS NOT DISTINCT (user_id, place_id, echo_id)
);

CREATE TABLE echo_engagement_events (
    id uuid PRIMARY KEY,
    echo_id uuid NOT NULL REFERENCES echoes(id) ON DELETE CASCADE,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    event_type text NOT NULL CHECK (event_type IN ('detail_view', 'reaction', 'reply', 'stitch')),
    event_weight numeric(6,3) NOT NULL CHECK (event_weight >= 0),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    dedupe_key text
);

CREATE UNIQUE INDEX echo_engagement_events_dedupe_idx ON echo_engagement_events(dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX echo_engagement_events_echo_time_idx ON echo_engagement_events(echo_id, occurred_at DESC);

CREATE TABLE user_trust_scores (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    trust_tier text NOT NULL DEFAULT 'new' CHECK (trust_tier IN ('established', 'new', 'restricted', 'banned')),
    weight numeric(4,3) NOT NULL DEFAULT 0.500 CHECK (weight >= 0 AND weight <= 1),
    reason text NOT NULL DEFAULT 'new_account',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE echo_rank_snapshots (
    echo_id uuid PRIMARY KEY REFERENCES echoes(id) ON DELETE CASCADE,
    algorithm_version text NOT NULL,
    recency_score numeric(7,6) NOT NULL,
    engagement_score numeric(7,6) NOT NULL,
    locality_score numeric(7,6) NOT NULL,
    quality_score numeric(7,6) NOT NULL,
    subscription_score numeric(7,6) NOT NULL,
    risk_penalty numeric(7,6) NOT NULL,
    notability_score numeric(7,6) NOT NULL,
    ranking_reason text NOT NULL CHECK (ranking_reason IN ('recent', 'popular_nearby', 'subscribed_place', 'notable')),
    computed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX echo_rank_snapshots_score_idx ON echo_rank_snapshots(notability_score DESC, computed_at DESC);

CREATE TABLE place_alert_deliveries (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    echo_id uuid NOT NULL REFERENCES echoes(id) ON DELETE CASCADE,
    notification_id uuid REFERENCES notifications(id) ON DELETE SET NULL,
    delivered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, place_id, echo_id)
);

CREATE INDEX place_alert_deliveries_cooldown_idx ON place_alert_deliveries(user_id, place_id, delivered_at DESC);

CREATE TABLE privacy_jobs (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_type text NOT NULL CHECK (job_type IN ('export', 'delete')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'ready', 'completed', 'failed')),
    result_object_key text,
    failure_reason text,
    requested_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    expires_at timestamptz
);

CREATE INDEX privacy_jobs_user_requested_idx ON privacy_jobs(user_id, requested_at DESC);

ALTER TABLE media_assets
    ADD COLUMN sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order >= 0 AND sort_order < 5),
    ADD COLUMN capture_source text CHECK (capture_source IN ('camera', 'photo_library', 'microphone', 'quote_renderer')),
    ADD COLUMN original_expires_at timestamptz,
    ADD COLUMN moderation_hold boolean NOT NULL DEFAULT false;

ALTER TABLE places ADD COLUMN search_document tsvector NOT NULL DEFAULT ''::tsvector;
ALTER TABLE echoes ADD COLUMN search_document tsvector NOT NULL DEFAULT ''::tsvector;
ALTER TABLE media_transcripts ADD COLUMN search_document tsvector NOT NULL DEFAULT ''::tsvector;

CREATE INDEX places_search_document_gin_idx ON places USING gin(search_document);
CREATE INDEX places_name_trgm_idx ON places USING gin(normalized_name gin_trgm_ops);
CREATE INDEX place_aliases_name_trgm_idx ON place_aliases USING gin(normalized_alias gin_trgm_ops);
CREATE INDEX echoes_search_document_gin_idx ON echoes USING gin(search_document);
CREATE INDEX media_transcripts_search_document_gin_idx ON media_transcripts USING gin(search_document);

CREATE FUNCTION echotrails_places_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := setweight(to_tsvector('simple', coalesce(NEW.name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.description, '')), 'C');
    RETURN NEW;
END;
$$;

CREATE FUNCTION echotrails_echoes_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.body_text, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.caption_text, '')), 'B');
    RETURN NEW;
END;
$$;

CREATE FUNCTION echotrails_transcripts_search_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.search_document := to_tsvector('simple', coalesce(NEW.transcript_text, ''));
    RETURN NEW;
END;
$$;

CREATE TRIGGER places_search_update BEFORE INSERT OR UPDATE OF name, description ON places FOR EACH ROW EXECUTE FUNCTION echotrails_places_search_update();
CREATE TRIGGER echoes_search_update BEFORE INSERT OR UPDATE OF title, body_text, caption_text ON echoes FOR EACH ROW EXECUTE FUNCTION echotrails_echoes_search_update();
CREATE TRIGGER transcripts_search_update BEFORE INSERT OR UPDATE OF transcript_text ON media_transcripts FOR EACH ROW EXECUTE FUNCTION echotrails_transcripts_search_update();

UPDATE places SET name = name;
UPDATE echoes SET title = title;
UPDATE media_transcripts SET transcript_text = transcript_text;

INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, category, sensitive) VALUES
    ('019f486a-0001-7000-8000-000000000001', 'Prince Pond', 'prince pond', 'active', ST_SetSRID(ST_MakePoint(-88.0153297, 41.7975912), 4326)::geography, 100, 'landmark', false),
    ('019f486a-0002-7000-8000-000000000002', 'Downers Grove Main Street Metra', 'downers grove main street metra', 'active', ST_SetSRID(ST_MakePoint(-88.0086302, 41.7955440), 4326)::geography, 100, 'transit_station', false),
    ('019f486a-0003-7000-8000-000000000003', 'Downers Grove Public Library', 'downers grove public library', 'active', ST_SetSRID(ST_MakePoint(-88.0114616, 41.7940943), 4326)::geography, 75, 'library', false),
    ('019f486a-0004-7000-8000-000000000004', 'Tivoli Theatre', 'tivoli theatre', 'active', ST_SetSRID(ST_MakePoint(-88.0088001, 41.7960844), 4326)::geography, 75, 'theatre', false),
    ('019f486a-0005-7000-8000-000000000005', 'Fishel Park', 'fishel park', 'active', ST_SetSRID(ST_MakePoint(-88.0116448, 41.7928667), 4326)::geography, 150, 'park', false),
    ('019f486a-0006-7000-8000-000000000006', 'Washington Park', 'washington park', 'active', ST_SetSRID(ST_MakePoint(-88.0057961, 41.7992530), 4326)::geography, 200, 'park', false),
    ('019f486a-0007-7000-8000-000000000007', 'Constitution Park and Lincoln Center', 'constitution park and lincoln center', 'active', ST_SetSRID(ST_MakePoint(-88.0089976, 41.7907635), 4326)::geography, 200, 'park', false),
    ('019f486a-0008-7000-8000-000000000008', 'Downers Grove Museum', 'downers grove museum', 'active', ST_SetSRID(ST_MakePoint(-88.0051168, 41.7925048), 4326)::geography, 100, 'museum', false),
    ('019f486a-0009-7000-8000-000000000009', 'Downers Grove North High School', 'downers grove north high school', 'restricted', ST_SetSRID(ST_MakePoint(-88.0122581, 41.8065417), 4326)::geography, 250, 'school', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO place_provider_mappings (id, place_id, provider, provider_place_id, provider_payload) VALUES
    ('019f486b-0001-7000-8000-000000000001', '019f486a-0001-7000-8000-000000000001', 'openstreetmap', 'way/35362323', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0002-7000-8000-000000000002', '019f486a-0002-7000-8000-000000000002', 'openstreetmap', 'node/25779937', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0003-7000-8000-000000000003', '019f486a-0003-7000-8000-000000000003', 'openstreetmap', 'way/200192088', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0004-7000-8000-000000000004', '019f486a-0004-7000-8000-000000000004', 'openstreetmap', 'way/200218663', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0005-7000-8000-000000000005', '019f486a-0005-7000-8000-000000000005', 'openstreetmap', 'way/200054182', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0006-7000-8000-000000000006', '019f486a-0006-7000-8000-000000000006', 'openstreetmap', 'way/552662784', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0007-7000-8000-000000000007', '019f486a-0007-7000-8000-000000000007', 'openstreetmap', 'way/567113043', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0008-7000-8000-000000000008', '019f486a-0008-7000-8000-000000000008', 'openstreetmap', 'way/684600471', '{"attribution":"OpenStreetMap contributors"}'),
    ('019f486b-0009-7000-8000-000000000009', '019f486a-0009-7000-8000-000000000009', 'openstreetmap', 'way/548571172', '{"attribution":"OpenStreetMap contributors"}')
ON CONFLICT (provider, provider_place_id) DO NOTHING;

INSERT INTO place_aliases (id, place_id, alias, normalized_alias) VALUES
    ('019f486c-0001-7000-8000-000000000001', '019f486a-0001-7000-8000-000000000001', 'Prince Pond Park', 'prince pond park'),
    ('019f486c-0002-7000-8000-000000000002', '019f486a-0002-7000-8000-000000000002', 'Main Street Station', 'main street station'),
    ('019f486c-0003-7000-8000-000000000003', '019f486a-0003-7000-8000-000000000003', 'DGPL', 'dgpl'),
    ('019f486c-0004-7000-8000-000000000004', '019f486a-0007-7000-8000-000000000007', 'Constitution Park', 'constitution park'),
    ('019f486c-0005-7000-8000-000000000005', '019f486a-0007-7000-8000-000000000007', 'Lincoln Center', 'lincoln center'),
    ('019f486c-0006-7000-8000-000000000006', '019f486a-0008-7000-8000-000000000008', 'Wandschneider Park', 'wandschneider park')
ON CONFLICT (place_id, normalized_alias) DO NOTHING;
