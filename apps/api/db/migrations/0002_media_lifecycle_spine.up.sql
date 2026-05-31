-- Media upload lifecycle, transcription, and moderation spine.
-- Defines durable contracts only; processors/scanners are later integrations.

CREATE TABLE media_uploads (
    id uuid PRIMARY KEY,
    media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
    upload_url text NOT NULL,
    method text NOT NULL DEFAULT 'PUT' CHECK (method IN ('PUT')),
    status text NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'uploaded', 'expired', 'failed')),
    expires_at timestamptz NOT NULL,
    idempotency_key text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key)
);

CREATE INDEX media_uploads_media_asset_id_idx ON media_uploads(media_asset_id);
CREATE INDEX media_uploads_status_expires_at_idx ON media_uploads(status, expires_at);

CREATE TABLE media_transcripts (
    id uuid PRIMARY KEY,
    media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'skipped')),
    locale text NOT NULL DEFAULT 'und',
    transcript_text text NOT NULL DEFAULT '',
    confidence numeric(5,4),
    model_name text NOT NULL DEFAULT '',
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (media_asset_id, locale)
);

CREATE INDEX media_transcripts_status_idx ON media_transcripts(status);

CREATE TABLE media_moderation_reviews (
    id uuid PRIMARY KEY,
    media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'scanning', 'approved', 'needs_review', 'rejected', 'failed')),
    risk_level text NOT NULL DEFAULT 'unknown' CHECK (risk_level IN ('unknown', 'low', 'medium', 'high')),
    categories jsonb NOT NULL DEFAULT '{}'::jsonb,
    scanner_name text NOT NULL DEFAULT '',
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (media_asset_id, scanner_name)
);

CREATE INDEX media_moderation_reviews_status_idx ON media_moderation_reviews(status);
