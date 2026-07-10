-- Durable social visibility, Reply continuity, and immutable quote snapshots.

CREATE TABLE hidden_targets (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type text NOT NULL CHECK (target_type IN ('echo', 'reply')),
    target_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_type, target_id)
);

CREATE INDEX hidden_targets_target_idx ON hidden_targets(target_type, target_id);

ALTER TABLE echo_links
    ADD COLUMN source_reply_revision integer,
    ADD COLUMN snapshot_status text NOT NULL DEFAULT 'not_requested'
        CHECK (snapshot_status IN ('not_requested', 'queued', 'rendering', 'ready', 'suppressed', 'failed')),
    ADD COLUMN snapshot_suppressed_at timestamptz,
    ADD COLUMN snapshot_suppression_reason text;

CREATE INDEX replies_parent_created_idx ON replies(parent_reply_id, created_at, id);
CREATE INDEX notifications_expires_idx ON notifications(expires_at) WHERE expires_at IS NOT NULL;

-- Backfill revision 1 for any Replies created before revision tracking landed.
INSERT INTO reply_revisions (id, reply_id, revision_number, body_text, created_at)
SELECT id, id, 1, body_text, created_at FROM replies
ON CONFLICT (reply_id, revision_number) DO NOTHING;
