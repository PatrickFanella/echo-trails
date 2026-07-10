-- Self-hosted text moderation for text Echoes and captions.

CREATE TABLE echo_moderation_reviews (
    id uuid PRIMARY KEY,
    echo_id uuid NOT NULL REFERENCES echoes(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('approved', 'needs_review', 'rejected', 'failed')),
    risk_level text NOT NULL CHECK (risk_level IN ('unknown', 'low', 'medium', 'high')),
    categories jsonb NOT NULL DEFAULT '{}'::jsonb,
    scanner_name text NOT NULL,
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (echo_id, scanner_name)
);

CREATE INDEX echo_moderation_reviews_status_idx ON echo_moderation_reviews(status, created_at);
