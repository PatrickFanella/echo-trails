-- Production media processing and lifecycle states.

ALTER TABLE media_assets DROP CONSTRAINT media_assets_status_check;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_status_check
    CHECK (status IN ('pending_upload', 'uploaded', 'processing', 'pending_review', 'approved', 'rejected', 'failed', 'deleted'));

ALTER TABLE media_assets ADD COLUMN verified_content_type text;
ALTER TABLE media_assets ADD COLUMN malware_scan_status text NOT NULL DEFAULT 'pending'
    CHECK (malware_scan_status IN ('pending', 'clean', 'infected', 'failed'));
ALTER TABLE media_assets ADD COLUMN processed_at timestamptz;

CREATE INDEX media_assets_original_expiry_idx ON media_assets(original_expires_at)
    WHERE original_expires_at IS NOT NULL AND deleted_at IS NULL;
