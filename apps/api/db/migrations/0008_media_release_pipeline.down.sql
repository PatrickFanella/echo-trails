DROP INDEX IF EXISTS media_assets_original_expiry_idx;
ALTER TABLE media_assets DROP COLUMN IF EXISTS processed_at;
ALTER TABLE media_assets DROP COLUMN IF EXISTS malware_scan_status;
ALTER TABLE media_assets DROP COLUMN IF EXISTS verified_content_type;
ALTER TABLE media_assets DROP CONSTRAINT media_assets_status_check;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_status_check
    CHECK (status IN ('pending_upload', 'uploaded', 'processing', 'approved', 'rejected', 'failed', 'deleted'));
