DROP INDEX IF EXISTS notifications_expires_idx;
DROP INDEX IF EXISTS replies_parent_created_idx;

ALTER TABLE echo_links
    DROP COLUMN IF EXISTS snapshot_suppression_reason,
    DROP COLUMN IF EXISTS snapshot_suppressed_at,
    DROP COLUMN IF EXISTS snapshot_status,
    DROP COLUMN IF EXISTS source_reply_revision;

DROP TABLE IF EXISTS hidden_targets;
